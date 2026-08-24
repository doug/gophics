//go:build windows

package camera

import (
	"image"
	"testing"
)

// The same contract the V4L2 converters have: any geometry, any buffer,
// convert what fits and never read past the end.
func FuzzRGB32ToRGBA(f *testing.F) {
	f.Add(make([]byte, 4), 1, 1, 4)
	f.Add(make([]byte, 64*48*4), 64, 48, 64*4)
	// Bottom-up, which is RGB32's default and flips the row order.
	f.Add(make([]byte, 8*8*4), 8, 8, -8*4)
	// A stride narrower than a row of pixels. The buffer is stride*height, so
	// the final row's w*4 slice ran past the end. This panicked.
	f.Add(make([]byte, 10*4), 10, 4, 4)
	f.Add([]byte{}, 4, 4, 16)

	f.Fuzz(func(t *testing.T, src []byte, w, h, stride int) {
		if w < 0 || h < 0 || w > 512 || h > 512 || stride < -1<<20 || stride > 1<<20 {
			t.Skip()
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		rgb32ToRGBA(src, img, w, h, stride)
	})
}
