package gg

import "testing"

// TestPushLayerHiDPI guards against a regression where PushLayer allocated the
// layer pixmap at logical size (c.width, c.height) rather than the physical
// (device-scaled) size of the backing pixmap. On deviceScale > 1 that clipped
// all layer drawing outside the top-left logical-sized region, so content in
// the bottom/right of a HiDPI surface silently vanished — and a scene with many
// small opacity groups appeared to "drop" its later groups.
//
// Here we draw one filled rect per layer across the whole surface at
// deviceScale 2 and require every rect to survive compositing.
func TestPushLayerHiDPI(t *testing.T) {
	const dim, n = 480, 64
	dc := NewContextWithScale(dim, dim, 2)
	dc.SetGPUDisabled(true)
	dc.SetRGB(0, 0, 0)
	dc.Clear()
	for i := 0; i < n; i++ {
		dc.PushLayer(BlendNormal, 0.9)
		dc.SetRGB(1, 1, 1)
		dc.DrawRectangle(float64(i%8)*56+21, float64(i/8)*56+21, 18, 18)
		dc.Fill()
		dc.PopLayer()
	}

	// Each 18px logical rect is 36px physical; 64 of them = 82944 px if none
	// were clipped. Count near-white pixels and require essentially all.
	img := dc.Image()
	b := img.Bounds()
	painted := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if r, _, _, _ := img.At(x, y).RGBA(); r > 0x8000 {
				painted++
			}
		}
	}
	const want = n * 36 * 36
	if painted < want*9/10 {
		t.Errorf("HiDPI layer content clipped: %d/%d px rendered (bottom/right layers dropped)", painted, want)
	}
}
