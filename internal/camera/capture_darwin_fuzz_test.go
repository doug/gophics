//go:build darwin && !ios

package camera

import (
	"image"
	"testing"
)

// The same contract the other two converters have: any geometry, any buffer,
// convert what fits and never read past the end.
func FuzzBGRAToRGBA(f *testing.F) {
	f.Add(make([]byte, 4), 1, 1, 4)
	f.Add(make([]byte, 64*48*4), 64, 48, 64*4)
	// CoreVideo pads rows, so a stride wider than the pixels is the normal
	// case rather than the exception.
	f.Add(make([]byte, 8*64), 8, 8, 64)
	// A stride narrower than a row of pixels, which is what ran the Windows
	// converter off the end.
	f.Add(make([]byte, 10*4), 10, 4, 4)
	f.Add([]byte{}, 4, 4, 16)

	f.Fuzz(func(t *testing.T, src []byte, w, h, stride int) {
		if w < 0 || h < 0 || w > 512 || h > 512 || stride < -1<<20 || stride > 1<<20 {
			t.Skip()
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		bgraToRGBA(src, img, w, h, stride)
	})
}
