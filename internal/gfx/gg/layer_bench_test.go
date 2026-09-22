package gg

import "testing"

// phoneLayerContext builds a 1080x2400 context (a 411x914 logical phone
// surface at scale 2.625) with an opaque background, as the mobile CPU
// present path sees it.
func phoneLayerContext() *Context {
	dc := NewContext(1080, 2400)
	dc.SetRGB(1, 1, 1)
	dc.Clear()
	dc.SetRGB(0.2, 0.4, 0.8)
	dc.DrawRectangle(0, 0, 1080, 300)
	dc.Fill()
	return dc
}

// drawLayerContent paints the kind of content a widget group produces: a
// few filled cards covering roughly a third of the surface, with
// anti-aliased rounded corners so partially transparent pixels are present.
func drawLayerContent(dc *Context) {
	dc.SetRGB(0.9, 0.9, 0.9)
	for y := 100.0; y < 1000; y += 180 {
		dc.DrawRoundedRectangle(40, y, 1000, 150, 24)
		dc.Fill()
	}
	dc.SetRGB(0.1, 0.1, 0.1)
	dc.DrawCircle(540, 1800, 200)
	dc.Fill()
}

// BenchmarkCompositeLayer measures compositeLayer alone: one phone-sized
// layer with typical content composited over the base surface at 85%
// opacity, the drag-preview case.
func BenchmarkCompositeLayer(b *testing.B) {
	dc := phoneLayerContext()
	base := dc.pixmap
	dc.PushLayer(BlendNormal, 0.85)
	drawLayerContent(dc)
	layer := dc.layerStack.layers[0]
	// Composite onto a scratch copy so every iteration sees the same parent.
	parent := NewPixmap(base.Width(), base.Height())
	b.SetBytes(int64(len(base.Data())))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(parent.Data(), base.Data())
		dc.compositeLayer(layer, parent)
	}
}

// BenchmarkCompositeLayerOpaque is the alpha-1 case: a group that pushes a
// layer for structure (mask, blend) rather than for fading.
func BenchmarkCompositeLayerOpaque(b *testing.B) {
	dc := phoneLayerContext()
	base := dc.pixmap
	dc.PushLayer(BlendNormal, 1)
	drawLayerContent(dc)
	layer := dc.layerStack.layers[0]
	parent := NewPixmap(base.Width(), base.Height())
	b.SetBytes(int64(len(base.Data())))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy(parent.Data(), base.Data())
		dc.compositeLayer(layer, parent)
	}
}

// BenchmarkPushPopLayer measures the whole round trip a PushOpacity /
// PopOpacity pair costs: layer allocation and clear, content, composite.
func BenchmarkPushPopLayer(b *testing.B) {
	dc := phoneLayerContext()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc.PushLayer(BlendNormal, 0.85)
		drawLayerContent(dc)
		dc.PopLayer()
	}
}

// BenchmarkPushPopLayerEmpty isolates the fixed cost of a layer that
// nothing draws into: allocation, clear, and the composite scan.
func BenchmarkPushPopLayerEmpty(b *testing.B) {
	dc := phoneLayerContext()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dc.PushLayer(BlendNormal, 0.85)
		dc.PopLayer()
	}
}
