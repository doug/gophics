//go:build !nogpu

package gpu

import (
	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gpucontext"
)

// Opacity/blend group compositing on the GPU (Skia saveLayer / Flutter
// OpacityLayer). PushLayer/PopLayer record markers into pendingDraws
// (gpu_render_context.go); at Flush time resolveLayers rewrites the queue so
// each balanced [PushLayer..PopLayer] span becomes a single textured-quad
// composite of an offscreen target the span was rendered into.
//
// Why offscreen: the accelerated frame is ONE render pass with per-type tier
// ordering, so a group's alpha cannot be applied per-fill. Rendering the group
// to its own target (LoadOpClear, TBDR-safe) and compositing the resolved
// texture with the group alpha is the standard model (Skia/Impeller). A group
// is effectively a RepaintBoundary with an opacity composite.
//
// Buffer-sharing hazard: the render session writes into session-level GPU
// buffers, so a group must render through its OWN GPURenderContext/session (the
// multi-context pattern the shared encoder already supports), not by re-running
// the parent session mid-frame. Each child self-submits to its offscreen target;
// queue-ordered submits give the read-after-write dependency the parent's
// composite needs.

// maxChildCtxPool bounds the reusable child-context pool. Layer groups render
// sequentially (resolveDraws recurses innermost-first, one child active at a
// time), so this only needs to cover a little nesting depth.
const maxChildCtxPool = 4

// acquireChildContext returns a reusable child render context for rendering an
// opacity/blend group to an offscreen target, creating one on a pool miss.
func (s *GPUShared) acquireChildContext() *GPURenderContext {
	s.mu.Lock()
	if n := len(s.childCtxPool); n > 0 {
		c := s.childCtxPool[n-1]
		s.childCtxPool = s.childCtxPool[:n-1]
		s.mu.Unlock()
		return c
	}
	s.mu.Unlock()
	return s.NewRenderContext()
}

// releaseChildContext returns a child context to the pool for reuse, keeping its
// session/buffers alive. Transient per-frame state is dropped. When the pool is
// full the context is closed (its GPU resources freed) instead.
func (s *GPUShared) releaseChildContext(c *GPURenderContext) {
	c.pendingDraws = c.pendingDraws[:0]
	c.hasPendingTarget = false
	c.clipRect = nil
	c.clipRRect = nil
	c.clipPath = nil
	// Frame state must go too, and this is load-bearing rather than tidiness.
	//
	// The surface pass picks LoadOpClear or LoadOpLoad from frameRendered, and
	// resets it only when the target view changes. That was sound while every
	// layer got a freshly created texture: a new view each frame, so the reset
	// always fired. Once offscreen textures were pooled, a same-size layer gets
	// the same view back — and a pooled context that still remembered it would
	// compare equal, skip the reset, and load the recycled texture's previous
	// contents instead of clearing them.
	//
	// A layer's backdrop is transparent by definition, which is what makes
	// group opacity mean anything, so a child always begins its target cleared.
	// Left in, this showed up as a drag preview leaving a copy of itself at
	// every position the pointer passed through.
	c.frameRendered = false
	c.lastView = nil
	if c.session != nil {
		c.session.SetFrameState(false, nil)
	}
	s.mu.Lock()
	if len(s.childCtxPool) < maxChildCtxPool {
		s.childCtxPool = append(s.childCtxPool, c)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	c.Close()
}

// hasLayerMarkers reports whether pendingDraws needs the offscreen-resolve pass:
// an opacity/blend group (drawCmdPushLayer) or a backdrop blur.
func (rc *GPURenderContext) hasLayerMarkers() bool {
	for i := range rc.pendingDraws {
		if k := rc.pendingDraws[i].kind; k == drawCmdPushLayer || k == drawCmdBackdropBlur {
			return true
		}
	}
	return false
}

// resolveLayers rewrites rc.pendingDraws in place, replacing every top-level
// opacity/blend group with a composite of an offscreen render of its contents.
// Nested groups are resolved first (innermost-first) by recursion in
// resolveDraws. The offscreen textures are freed one frame later (the GPU has
// finished sampling them by then — avoids a use-after-free on the async submit).
func (rc *GPURenderContext) resolveLayers(target gg.GPURenderTarget) {
	// Free the previous frame's layer textures now that their submit is done.
	for _, r := range rc.prevLayerReleases {
		if r != nil {
			r()
		}
	}
	rc.prevLayerReleases = rc.prevLayerReleases[:0]

	w, h := rc.layerDims(target)

	// rasterAtlas cannot render into a layer texture at all. On that strategy
	// shapes are rasterized into a CPU pixmap and the pixmap is uploaded to the
	// target — but a layer target is a bare texture with no pixmap behind it,
	// so uploadPixmapToView finds nothing to upload and returns success having
	// written nothing. The caller then gets a freshly created, still-black
	// texture and composites it as though it were the group's contents or the
	// backdrop, which is how a frosted card becomes an opaque slab: a
	// translucent white surface over black is flat grey.
	//
	// Better to have no frost and no group isolation than a black rectangle
	// where the UI should be.
	if w == 0 || h == 0 || rc.shared.strategy == strategyRasterAtlas {
		// No usable layer target — flatten markers away (draw at full opacity)
		// and drop backdrop blurs rather than leaving them for the group
		// builder to choke on.
		rc.pendingDraws = stripLayerMarkers(rc.pendingDraws)
		return
	}

	var releases []func()
	rc.pendingDraws = rc.resolveDraws(rc.pendingDraws, w, h, &releases)
	rc.prevLayerReleases = releases
}

// resolveDraws returns a flat, marker-free copy of src in which each balanced
// top-level [PushLayer..PopLayer] span is replaced by a single GPU-texture
// composite command. Offscreen release funcs are appended to *releases.
func (rc *GPURenderContext) resolveDraws(src []drawCommand, w, h uint32, releases *[]func()) []drawCommand {
	out := make([]drawCommand, 0, len(src))
	i := 0
	for i < len(src) {
		if src[i].kind == drawCmdBackdropBlur {
			// Frost the backdrop resolved so far, within this command's clip.
			out = rc.appendBackdropBlur(out, src[i], w, h, releases)
			i++
			continue
		}
		if src[i].kind != drawCmdPushLayer {
			out = append(out, src[i])
			i++
			continue
		}

		// Find the PopLayer matching this PushLayer (balanced nesting).
		opacity := src[i].layerOpacity
		depth := 1
		j := i + 1
		for j < len(src) {
			if src[j].kind == drawCmdPushLayer {
				depth++
			} else if src[j].kind == drawCmdPopLayer {
				depth--
				if depth == 0 {
					break
				}
			}
			j++
		}
		// inner = the group's draws; may itself contain nested groups.
		inner := src[i+1 : j]
		flatInner := rc.resolveDraws(inner, w, h, releases) // innermost-first

		view, release := rc.renderLayerToTexture(flatInner, w, h)
		if view.IsNil() {
			// Offscreen render unavailable — inline the group's draws so its
			// content still appears (at full opacity, base preserved) rather
			// than vanishing.
			out = append(out, flatInner...)
		} else {
			*releases = append(*releases, release)
			out = append(out, layerCompositeCommand(view, opacity, w, h))
		}

		if j < len(src) {
			i = j + 1 // skip past the matching PopLayer
		} else {
			i = j // unbalanced (no PopLayer) — stop
		}
	}
	return out
}

// renderLayerToTexture renders a flat (marker-free) draw list to a fresh
// offscreen target via a child render context and returns the sampleable view
// plus a release func. Returns a nil view if the GPU target can't be created.
func (rc *GPURenderContext) renderLayerToTexture(flat []drawCommand, w, h uint32) (gpucontext.TextureView, func()) {
	if len(flat) == 0 {
		return gpucontext.TextureView{}, nil
	}
	view, release := rc.CreateOffscreenTexture(int(w), int(h))
	if view.IsNil() {
		return gpucontext.TextureView{}, nil
	}

	// Render the group through its own context/session so the parent's shared
	// GPU buffers are not clobbered mid-frame. flat is already marker-free, so
	// the child never re-enters the layer path. The child is pooled — sessions
	// are reused across groups/frames rather than created and destroyed each time.
	child := rc.shared.acquireChildContext()
	child.antiAlias = rc.antiAlias
	child.SetPipelineMode(rc.pipelineMode)
	child.pendingDraws = append(child.pendingDraws, flat...)

	lt := gg.GPURenderTarget{View: view, ViewWidth: w, ViewHeight: h}
	err := child.Flush(lt)
	rc.shared.releaseChildContext(child)
	if err != nil {
		slogger().Warn("opacity layer render failed", "err", err, "w", w, "h", h)
		if release != nil {
			release()
		}
		return gpucontext.TextureView{}, nil
	}
	return view, release
}

// layerCompositeCommand builds the full-surface textured-quad draw that
// composites a rendered layer target onto the parent with the group opacity.
func layerCompositeCommand(view gpucontext.TextureView, opacity float32, w, h uint32) drawCommand {
	return drawCommand{
		kind: drawCmdGPUTexture,
		gpuTexCmd: GPUTextureDrawCommand{
			View:           view,
			DstX:           0,
			DstY:           0,
			DstW:           float32(w),
			DstH:           float32(h),
			Opacity:        opacity,
			ViewportWidth:  w,
			ViewportHeight: h,
		},
	}
}

// blurComposite is layerCompositeCommand (opacity 1) carrying the frosted-glass
// extras: a per-tap Gaussian step (one axis at a time) and/or a saturation
// boost. FROST-BLUR (VULKAN-VERIFY) — only appendBackdropBlur uses it; every
// other composite goes through layerCompositeCommand with these left at 0.
func blurComposite(view gpucontext.TextureView, w, h uint32, stepX, stepY, saturation float32) drawCommand {
	dc := layerCompositeCommand(view, 1, w, h)
	tc := dc.gpuTexCmd.(GPUTextureDrawCommand)
	tc.BlurStepX = stepX
	tc.BlurStepY = stepY
	tc.Saturation = saturation
	dc.gpuTexCmd = tc
	return dc
}

// layerDims returns the physical pixel dimensions for full-surface layer
// targets, matching how the session sizes the parent frame
// (effectiveDimensions): the per-pass view size when rendering to a view,
// otherwise the CPU-readback target size.
func (rc *GPURenderContext) layerDims(target gg.GPURenderTarget) (uint32, uint32) {
	if !target.View.IsNil() && target.ViewWidth > 0 && target.ViewHeight > 0 {
		return target.ViewWidth, target.ViewHeight
	}
	if target.Width > 0 && target.Height > 0 {
		return uint32(target.Width), uint32(target.Height) //nolint:gosec // dimensions fit uint32
	}
	return 0, 0
}

// backdropSaturation is the saturation multiplier applied to the frosted
// backdrop as it composites back. A wide blur averages colors toward grey; Apple
// (and every "vibrancy" material since) pushes saturation back up so the color
// behind the glass still glows through. ~1.5 is the sweet spot — enough to read
// as vivid, short of the neon look >1.8 gives. See textured_quad.wgsl.
const backdropSaturation = 1.5

// appendBackdropBlur frosts the backdrop drawn so far (the commands already in
// out) within cmd's clip. It renders that backdrop to a full-size offscreen,
// downsamples it (cheaper + keeps the fixed-tap kernel dense), runs a real
// separable Gaussian — a horizontal then a vertical weighted-tap pass — and
// composites the result back at full size with a saturation boost, clipped to
// the panel's rounded rect. It reuses the opacity-layer offscreen machinery, so
// everything composites in the main pass — no mid-pass swapchain readback. If an
// offscreen can't be made it degrades to no frost (the panel's tint stands in).
//
// This replaced an earlier mip-pyramid (downsample then bilinear upscale): a
// pyramid only APPROXIMATES a blur and its power-of-two reconstruction never
// matched the CPU path's true box blur — visibly softer/faceted. A Gaussian
// kernel at the actual radius matches it. The kernel lives in textured_quad.wgsl
// (FROST-BLUR, VULKAN-VERIFY); this function just drives the passes.
func (rc *GPURenderContext) appendBackdropBlur(out []drawCommand, cmd drawCommand, w, h uint32, releases *[]func()) []drawCommand {
	if len(out) == 0 || w < 8 || h < 8 {
		return out // nothing behind, or too small to reduce
	}
	src, release := rc.renderLayerToTexture(out, w, h)
	if src.IsNil() {
		return out
	}
	*releases = append(*releases, release)

	// Downsample first: the shader uses a fixed 13 taps spaced radius/2 device px
	// apart (see below), so shrink the backdrop until that step is only ~1–2
	// reduced texels — dense enough that the wide kernel reconstructs smoothly
	// instead of rippling. The UV blur step below is invariant to how far we
	// downsample (taps and texels shrink together), so this only trades quality
	// for speed, never the amount of blur. The Gaussian-smoothed source also
	// upsamples cleanly in the final composite, so heavy reduction is safe here.
	cw, ch := w, h
	for r := cmd.backdropRadius; r > 4 && cw >= 32 && ch >= 32; r /= 2 {
		cw, ch = cw/2, ch/2
		down, rel := rc.resampleTexture(src, cw, ch)
		if down.IsNil() {
			break
		}
		*releases = append(*releases, rel)
		src = down
	}

	// Separable Gaussian sized to MATCH the CPU path's 3-pass box blur (backdrop.go
	// blurPixmapRegion), whose effective sigma ≈ radius (sqrt(r²+r)). The kernel is
	// 13 taps at sigma=2 in tap space, so sigma_device = 2·stepDevice; a step of
	// radius/2 gives sigma_device = radius and lands the ±6 taps at ±3·sigma. In UV
	// that per-tap step is (radius/2)/fullDim = radius/(2·fullDim) — invariant to
	// the downsample level. (An earlier radius/(6·fullDim) put the taps at only
	// ±radius, so sigma came out ~radius/3 and the GPU frost read visibly sharper
	// than the CPU one.)
	stepX := cmd.backdropRadius / (2 * float32(w))
	stepY := cmd.backdropRadius / (2 * float32(h))
	hp := blurComposite(src, cw, ch, stepX, 0, 0)
	if hv, rel := rc.renderLayerToTexture([]drawCommand{hp}, cw, ch); !hv.IsNil() {
		*releases = append(*releases, rel)
		src = hv
	}
	vp := blurComposite(src, cw, ch, 0, stepY, 0)
	if vv, rel := rc.renderLayerToTexture([]drawCommand{vp}, cw, ch); !vv.IsNil() {
		*releases = append(*releases, rel)
		src = vv
	}

	// Composite back at full size with the vibrancy boost, clipped to the panel's
	// rounded rect so only it frosts.
	comp := blurComposite(src, w, h, 0, 0, backdropSaturation)
	comp.clipRect = cmd.clipRect
	comp.clipRRect = cmd.clipRRect
	comp.clipPath = cmd.clipPath
	return append(out, comp)
}

// resampleTexture renders src into a fresh w×h offscreen with bilinear
// sampling — the downsample step feeding the Gaussian passes in
// appendBackdropBlur.
func (rc *GPURenderContext) resampleTexture(src gpucontext.TextureView, w, h uint32) (gpucontext.TextureView, func()) {
	shrink := layerCompositeCommand(src, 1, w, h)
	return rc.renderLayerToTexture([]drawCommand{shrink}, w, h)
}

// stripLayerMarkers drops PushLayer/PopLayer markers, leaving the group's draws
// inline (full opacity), and drops backdrop blurs. Fallback when no offscreen
// target is possible.
//
// A backdrop blur has to go too, not just the group markers: it is resolved by
// rendering the backdrop into an offscreen and compositing it back, so with no
// offscreen available the command has nothing to draw and would otherwise be
// handed to the group builder unresolved.
func stripLayerMarkers(src []drawCommand) []drawCommand {
	out := src[:0]
	for i := range src {
		switch src[i].kind {
		case drawCmdPushLayer, drawCmdPopLayer, drawCmdBackdropBlur:
			continue
		}
		out = append(out, src[i])
	}
	return out
}
