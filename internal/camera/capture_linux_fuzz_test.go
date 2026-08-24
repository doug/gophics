//go:build linux && !android

package camera

import (
	"image"
	"testing"
)

// The converters take their geometry from the driver: a width, a height and a
// bytes-per-line that V4L2 fills in, then index a mmapped buffer with them.
// The buffer is not attacker-controlled in the way a downloaded file is, but a
// driver reporting a stride the buffer does not honour is an ordinary hardware
// quirk, and the result of getting the bound wrong is the same either way.
//
// The contract is total: for any geometry and any buffer, convert what fits
// and return — never read past the end.
func FuzzYUYVToRGBA(f *testing.F) {
	f.Add([]byte{235, 128, 16, 128}, 2, 1, 4)
	f.Add([]byte{}, 1, 1, 0)
	f.Add(make([]byte, 64*48*2), 64, 48, 64*2)
	// A stride the buffer cannot satisfy, which is the case the length check
	// exists for.
	f.Add(make([]byte, 16), 64, 48, 4096)
	// An odd width: the last pixel completes a quad whose partner does not
	// exist, so a row needs two bytes more than w*2. This panicked.
	f.Add([]byte{0, 0}, 1, 54, 0)

	f.Fuzz(func(t *testing.T, src []byte, w, h, stride int) {
		// Bound the allocation, not the logic: the destination is always sized
		// to w×h by the caller, so a mismatch there would be a different bug.
		if w < 0 || h < 0 || w > 512 || h > 512 || stride < -1<<20 || stride > 1<<20 {
			t.Skip()
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		yuyvToRGBA(src, img, w, h, stride)
	})
}

func FuzzPackedToRGBA(f *testing.F) {
	f.Add([]byte{10, 20, 30}, 1, 1, 3)
	f.Add([]byte{}, 4, 4, 12)
	f.Add(make([]byte, 32*24*3), 32, 24, 32*3)

	f.Fuzz(func(t *testing.T, src []byte, w, h, stride int) {
		if w < 0 || h < 0 || w > 512 || h > 512 || stride < -1<<20 || stride > 1<<20 {
			t.Skip()
		}
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		// Both channel orders, since they differ only in the index triple and
		// an out-of-range one would read the same memory either way.
		packedToRGBA(src, img, w, h, stride, 0, 1, 2)
		packedToRGBA(src, img, w, h, stride, 2, 1, 0)
	})
}
