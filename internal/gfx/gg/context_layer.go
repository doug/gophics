package gg

import (
	intImage "github.com/doug/gophics/internal/gfx/gg/internal/image"
)

// layerCtxOps is the optional per-context GPU interface for offscreen layer
// (saveLayer) compositing, implemented by *gpu.GPURenderContext. It is detected
// on c.gpuCtxOps() the same way the shared-encoder ops are (see context.go), so
// PushLayer/PopLayer route to the accelerator on the GPU path instead of the
// CPU-pixmap swap below.
type layerCtxOps interface {
	PushLayer(opacity float64, blend BlendMode)
	PopLayer()
}

// Layer represents a drawing layer with blend mode and opacity.
// Layers allow isolating drawing operations and compositing them with
// different blend modes and opacity values, similar to layers in Photoshop
// or SVG group opacity.
type Layer struct {
	pixmap    *Pixmap
	blendMode BlendMode
	opacity   float64
	mask      *Mask // optional alpha mask, applied on PopLayer (nil = no mask)
}

// layerStack manages the layer hierarchy for the context.
type layerStack struct {
	layers []*Layer
	pool   *intImage.Pool
}

// newLayerStack creates a new layer stack with a pool for memory reuse.
func newLayerStack() *layerStack {
	return &layerStack{
		layers: make([]*Layer, 0, 4),
		pool:   intImage.NewPool(8),
	}
}

// PushLayer creates a new layer and makes it the active drawing target.
// All subsequent drawing operations will render to this layer until PopLayer is called.
//
// The layer will be composited onto the parent layer/canvas when PopLayer is called,
// using the specified blend mode and opacity.
//
// Parameters:
//   - blendMode: How to composite this layer onto the parent (e.g., BlendMultiply, BlendScreen)
//   - opacity: Layer opacity in range [0.0, 1.0] where 0 is fully transparent and 1 is fully opaque
//
// Example:
//
//	dc.PushLayer(gg.BlendMultiply, 0.5)
//	dc.SetRGB(1, 0, 0)
//	dc.DrawCircle(100, 100, 50)
//	dc.Fill()
//	dc.PopLayer() // Composite circle onto canvas with multiply blend at 50% opacity
func (c *Context) PushLayer(blendMode BlendMode, opacity float64) {
	// Clamp opacity to valid range
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}

	// GPU path: record a layer marker on the per-context render context so the
	// accelerator composites an offscreen target (Skia saveLayer). Skip the
	// CPU-pixmap swap below — accelerated fills bypass c.pixmap, so the swap
	// loses prior GPU content and cannot apply the group alpha. See
	// design/gpu-opacity-layers.md.
	if rc := c.gpuCtxOps(); rc != nil {
		if la, ok := rc.(layerCtxOps); ok {
			la.PushLayer(opacity, blendMode)
			c.gpuLayerDepth++
			return
		}
	}

	// Initialize layer stack if needed
	if c.layerStack == nil {
		c.layerStack = newLayerStack()
	}

	// Save base pixmap on first push
	if len(c.layerStack.layers) == 0 && c.basePixmap == nil {
		c.basePixmap = c.pixmap
	}

	// Create new pixmap for the layer at the current target's PHYSICAL size.
	// c.width/c.height are logical; the backing pixmap is width*deviceScale.
	// Sizing the layer to c.width/c.height clipped all drawing past the
	// top-left logical region on HiDPI (deviceScale > 1) — matching c.pixmap
	// keeps the layer full-resolution.
	layerPixmap := NewPixmap(c.pixmap.Width(), c.pixmap.Height())
	layerPixmap.Clear(Transparent)

	// Create layer
	layer := &Layer{
		pixmap:    layerPixmap,
		blendMode: blendMode,
		opacity:   opacity,
	}

	// Save current pixmap and switch to layer pixmap
	c.layerStack.layers = append(c.layerStack.layers, layer)
	c.pixmap = layerPixmap
}

// PopLayer composites the current layer onto the parent layer/canvas.
// Uses the blend mode and opacity specified in the corresponding PushLayer call.
//
// The layer is composited using the specified blend mode and opacity.
// After compositing, the layer's memory is returned to the pool for reuse.
//
// If there are no layers to pop, this function does nothing.
//
// Example:
//
//	dc.PushLayer(gg.BlendScreen, 1.0)
//	// ... draw operations ...
//	dc.PopLayer() // Composite layer onto parent
func (c *Context) PopLayer() {
	// GPU path: balance a PushLayer that was routed to the accelerator.
	if c.gpuLayerDepth > 0 {
		if rc := c.gpuCtxOps(); rc != nil {
			if la, ok := rc.(layerCtxOps); ok {
				la.PopLayer()
				c.gpuLayerDepth--
				return
			}
		}
		// GPU became unavailable mid-frame: drop the stale count and fall
		// through to the CPU path (no-op if the CPU stack is empty).
		c.gpuLayerDepth = 0
	}

	if c.layerStack == nil || len(c.layerStack.layers) == 0 {
		return
	}

	// Pop the current layer
	layers := c.layerStack.layers
	layer := layers[len(layers)-1]
	c.layerStack.layers = layers[:len(layers)-1]

	// Get parent pixmap (either previous layer or base)
	var parentPixmap *Pixmap
	if len(c.layerStack.layers) > 0 {
		parentPixmap = c.layerStack.layers[len(c.layerStack.layers)-1].pixmap
	} else {
		// Restore base pixmap
		parentPixmap = c.basePixmap
		c.basePixmap = nil
	}

	// Apply mask to layer content before compositing (PushMaskLayer).
	if layer.mask != nil {
		c.applyMaskToPixmap(layer.pixmap, layer.mask)
	}

	// Composite layer onto parent
	c.compositeLayer(layer, parentPixmap)

	// Restore parent pixmap as current drawing target
	c.pixmap = parentPixmap
}

// PushMaskLayer creates an isolated layer with an associated alpha mask.
// All subsequent drawing operations render to this layer normally (without masking).
// When PopLayer is called, the ENTIRE layer is masked by the mask and then
// composited onto the parent using source-over blending with full opacity.
//
// This produces different results from SetMask: PushMaskLayer masks the
// composited group, while SetMask masks each shape individually.
//
// Matches Vello push_mask_layer() semantics (research §4).
//
// Example:
//
//	mask := gg.NewMaskFromAlpha(maskImage)
//	dc.PushMaskLayer(mask)
//	dc.DrawCircle(100, 100, 50)
//	dc.Fill()
//	dc.DrawRect(80, 80, 40, 40)
//	dc.Fill()
//	dc.PopLayer() // entire layer content masked, then composited
func (c *Context) PushMaskLayer(mask *Mask) {
	// Clamp: nil mask means no masking (equivalent to PushLayer).
	if mask == nil {
		c.PushLayer(BlendNormal, 1.0)
		return
	}

	// Initialize layer stack if needed.
	if c.layerStack == nil {
		c.layerStack = newLayerStack()
	}

	// Save base pixmap on first push.
	if len(c.layerStack.layers) == 0 && c.basePixmap == nil {
		c.basePixmap = c.pixmap
	}

	// Create new pixmap for the layer at the current target's PHYSICAL size
	// (see PushLayer — logical size clips HiDPI content).
	layerPixmap := NewPixmap(c.pixmap.Width(), c.pixmap.Height())
	layerPixmap.Clear(Transparent)

	// Create layer with mask.
	layer := &Layer{
		pixmap:    layerPixmap,
		blendMode: BlendNormal,
		opacity:   1.0,
		mask:      mask,
	}

	// Switch to layer pixmap.
	c.layerStack.layers = append(c.layerStack.layers, layer)
	c.pixmap = layerPixmap
}

// applyMaskToPixmap applies a DestinationIn mask to a pixmap's pixel data.
// For each pixel: all channels are scaled by mask.At(x,y) / 255.
func (c *Context) applyMaskToPixmap(pm *Pixmap, mask *Mask) {
	applyMaskToPixmapData(pm, mask)
}

// SetBlendMode sets the blend mode for subsequent fill and stroke operations.
// The blend mode controls how source pixels are composited onto the destination.
//
// All 29 blend modes from the W3C Compositing and Blending specification are
// supported: 14 Porter-Duff operators, 11 separable modes (Multiply, Screen,
// Overlay, etc.), and 4 non-separable HSL modes (Hue, Saturation, Color, Luminosity).
//
// The default mode is BlendNormal (source-over alpha compositing).
// For SourceOver (the default), rendering uses the existing optimized float64
// inline path with zero additional overhead.
//
// Example:
//
//	dc.SetBlendMode(gg.BlendMultiply)
//	dc.SetRGB(1, 0, 0)
//	dc.DrawRectangle(0, 0, 100, 100)
//	dc.Fill()
//	dc.SetBlendMode(gg.BlendNormal) // restore default
func (c *Context) SetBlendMode(mode BlendMode) {
	c.paint.blendMode = mapPublicBlendMode(mode)
}

// GetBlendMode returns the current blend mode for fill and stroke operations.
func (c *Context) GetBlendMode() BlendMode {
	return BlendMode(c.paint.blendMode)
}

// compositeLayer composites a layer onto a parent pixmap using the layer's
// blend mode and opacity.
func (c *Context) compositeLayer(layer *Layer, parent *Pixmap) {
	// Convert pixmaps to ImageBuf for blending
	srcImg := c.pixmapToImageBuf(layer.pixmap)
	dstImg := c.pixmapToImageBuf(parent)

	// Use DrawImage to composite with blend mode and opacity
	srcW, srcH := srcImg.Bounds()

	params := intImage.DrawParams{
		DstRect: intImage.Rect{
			X:      0,
			Y:      0,
			Width:  srcW,
			Height: srcH,
		},
		Interp:    intImage.InterpNearest, // No scaling, so nearest is fine
		Opacity:   layer.opacity,
		BlendMode: layer.blendMode,
	}

	intImage.DrawImage(dstImg, srcImg, params)
}
