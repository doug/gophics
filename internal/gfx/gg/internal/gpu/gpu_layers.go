//go:build !nogpu

package gpu

import (
	"github.com/doug/gossamer/internal/gfx/gg"
	"github.com/doug/gossamer/internal/gfx/gpucontext"
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
	s.mu.Lock()
	if len(s.childCtxPool) < maxChildCtxPool {
		s.childCtxPool = append(s.childCtxPool, c)
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	c.Close()
}

// hasLayerMarkers reports whether pendingDraws contains any opacity/blend group.
func (rc *GPURenderContext) hasLayerMarkers() bool {
	for i := range rc.pendingDraws {
		if rc.pendingDraws[i].kind == drawCmdPushLayer {
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
	if w == 0 || h == 0 {
		// No usable dimensions — flatten markers away (draw at full opacity)
		// rather than leaving them for the group builder to choke on.
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

// stripLayerMarkers drops PushLayer/PopLayer markers, leaving the group's draws
// inline (full opacity). Fallback when no offscreen target is possible.
func stripLayerMarkers(src []drawCommand) []drawCommand {
	out := src[:0]
	for i := range src {
		if src[i].kind == drawCmdPushLayer || src[i].kind == drawCmdPopLayer {
			continue
		}
		out = append(out, src[i])
	}
	return out
}
