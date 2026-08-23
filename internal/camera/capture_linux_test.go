//go:build linux && !android

package camera

import (
	"image"
	"testing"
	"unsafe"
)

// TestStructSizesMatchTheIoctlNumbers is the load-bearing test in this file.
//
// A V4L2 request number encodes the size of its argument, so the kernel
// rejects a struct of the wrong size outright — which is a gift, but only if
// the sizes are what we think. These assertions come from the request
// constants themselves: if someone adds a field, the arithmetic here fails
// before an ioctl does, and says which struct drifted.
func TestStructSizesMatchTheIoctlNumbers(t *testing.T) {
	size := func(req uintptr) uintptr { return (req >> 16) & 0x3fff }

	for _, c := range []struct {
		name string
		got  uintptr
		req  uintptr
	}{
		{"v4l2_capability", unsafe.Sizeof(v4l2Capability{}), vidiocQuerycap},
		{"v4l2_format", unsafe.Sizeof(v4l2Format{}), vidiocSFmt},
		{"v4l2_requestbuffers", unsafe.Sizeof(v4l2RequestBuffers{}), vidiocReqbufs},
		{"v4l2_buffer", unsafe.Sizeof(v4l2Buffer{}), vidiocQuerybuf},
	} {
		if want := size(c.req); c.got != want {
			t.Errorf("%s is %d bytes, but its ioctl encodes %d", c.name, c.got, want)
		}
	}
}

// TestBufferFieldOffsets pins the two fields the driver writes that we read
// back. Size alone would not catch a swapped pair, and reading BytesUsed out
// of the wrong offset produces a plausible-looking frame of garbage.
func TestBufferFieldOffsets(t *testing.T) {
	var b v4l2Buffer
	for _, c := range []struct {
		name string
		got  uintptr
		want uintptr
	}{
		{"Index", unsafe.Offsetof(b.Index), 0},
		{"BytesUsed", unsafe.Offsetof(b.BytesUsed), 8},
		{"Timestamp", unsafe.Offsetof(b.Timestamp), 24},
		{"Sequence", unsafe.Offsetof(b.Sequence), 56},
		{"Memory", unsafe.Offsetof(b.Memory), 60},
		{"Offset", unsafe.Offsetof(b.Offset), 64},
		{"Length", unsafe.Offsetof(b.Length), 72},
	} {
		if c.got != c.want {
			t.Errorf("v4l2_buffer.%s at offset %d, want %d", c.name, c.got, c.want)
		}
	}
}

func TestFourCC(t *testing.T) {
	if got := fourCC(pixFmtYUYV); got != "YUYV" {
		t.Errorf("YUYV code decoded as %q", got)
	}
	if got := fourCC(pixFmtMJPEG); got != "MJPG" {
		t.Errorf("MJPEG code decoded as %q", got)
	}
}

// TestYUYVToRGBA checks the conversion against values computed by hand, and
// checks the thing that is easy to get wrong: the two pixels in a YUYV quad
// share chroma but have their own luma, so a converter that reuses Y0 for both
// produces a picture that looks right until something moves.
func TestYUYVToRGBA(t *testing.T) {
	const w, h = 2, 1
	// Y0=235 (white), Y1=16 (black), neutral chroma.
	src := []byte{235, 128, 16, 128}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	yuyvToRGBA(src, img, w, h, 4)

	white := img.Pix[0:4]
	black := img.Pix[4:8]
	for i, v := range white[:3] {
		if v < 250 {
			t.Errorf("first pixel channel %d = %d, want ~255", i, v)
		}
	}
	for i, v := range black[:3] {
		if v > 5 {
			t.Errorf("second pixel channel %d = %d, want ~0 (is Y0 being reused for both?)", i, v)
		}
	}
	if white[3] != 0xff || black[3] != 0xff {
		t.Error("alpha is not opaque")
	}
}

func TestYUYVAppliesChroma(t *testing.T) {
	// Mid luma, strong V: should come out red-dominant.
	src := []byte{128, 128, 128, 255}
	img := image.NewRGBA(image.Rect(0, 0, 2, 1))
	yuyvToRGBA(src, img, 2, 1, 4)
	r, g, b := img.Pix[0], img.Pix[1], img.Pix[2]
	if !(r > g && r > b) {
		t.Errorf("got r=%d g=%d b=%d; a strong V should read red, so chroma is being dropped", r, g, b)
	}
}

// TestYUYVShortBufferDoesNotPanic: a driver may report fewer bytes than a full
// frame, and a partial buffer must be skipped rather than read past.
func TestYUYVShortBufferDoesNotPanic(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	yuyvToRGBA([]byte{1, 2, 3, 4}, img, 64, 64, 128)
}

func TestPackedRGBAndBGROrder(t *testing.T) {
	src := []byte{10, 20, 30}
	rgb := image.NewRGBA(image.Rect(0, 0, 1, 1))
	packedToRGBA(src, rgb, 1, 1, 3, 0, 1, 2)
	if rgb.Pix[0] != 10 || rgb.Pix[2] != 30 {
		t.Errorf("RGB24 came out %v, want R=10 B=30", rgb.Pix[:3])
	}
	bgr := image.NewRGBA(image.Rect(0, 0, 1, 1))
	packedToRGBA(src, bgr, 1, 1, 3, 2, 1, 0)
	if bgr.Pix[0] != 30 || bgr.Pix[2] != 10 {
		t.Errorf("BGR24 came out %v, want R=30 B=10", bgr.Pix[:3])
	}
}
