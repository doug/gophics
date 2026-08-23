//go:build (darwin && !ios) || (linux && !android) || windows

package camera

import (
	"image/png"
	"os"
	"testing"
	"time"
)

// TestHardwareCapture opens the real camera and checks that pixels arrive.
//
// Off by default: it needs a camera, and on macOS the first run raises the
// system permission prompt, which no unattended run can answer. Set
// GOPHICS_CAMERA_HW=1 to include it.
//
// It runs on every platform with a real backend, and is the same test on each,
// because the point is that they agree.
//
// It exists because the frame path is unsafe.Pointer arithmetic over
// CoreVideo's buffers, and every way of getting that wrong — a bad base
// pointer, a stride confused for a width, a channel order swapped — produces
// something that still compiles, still delivers a frame of the right size, and
// is still caught by nothing else in the suite.
func TestHardwareCapture(t *testing.T) {
	if os.Getenv("GOPHICS_CAMERA_HW") == "" {
		t.Skip("set GOPHICS_CAMERA_HW=1 to run against a real camera")
	}
	c, err := Open(Options{Facing: FacingFront, Width: 640})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer c.Stop()

	deadline := time.Now().Add(6 * time.Second)
	var frame = c.Frame()
	for frame == nil && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		frame = c.Frame()
	}
	if frame == nil {
		t.Fatal("no frame within 6s")
	}

	w, h := frame.Rect.Dx(), frame.Rect.Dy()
	if w <= 0 || h <= 0 {
		t.Fatalf("empty frame %dx%d", w, h)
	}
	if got, want := len(frame.Pix), w*h*4; got != want {
		t.Fatalf("pix length %d, want %d for %dx%d", got, want, w, h)
	}

	// A frame that is uniformly zero means the copy read the wrong memory; a
	// frame with a zero alpha channel means the conversion dropped a lane.
	var lit, opaque int
	for i := 0; i < len(frame.Pix); i += 4 {
		if int(frame.Pix[i])+int(frame.Pix[i+1])+int(frame.Pix[i+2]) > 0 {
			lit++
		}
		if frame.Pix[i+3] == 0xFF {
			opaque++
		}
	}
	px := w * h
	if lit*100/px < 50 {
		t.Errorf("only %d%% of pixels are non-black; the copy is probably reading the wrong address", lit*100/px)
	}
	if opaque != px {
		t.Errorf("%d of %d pixels are not opaque; alpha lane is wrong", px-opaque, px)
	}
	// An assertion cannot see that a frame is upside down or colour-swapped,
	// and both are the classic mistakes here — a negative stride on Windows,
	// a channel order on any of them. GOPHICS_CAMERA_DUMP writes the frame out
	// so a person can look at it, which is the only check that catches those.
	if out := os.Getenv("GOPHICS_CAMERA_DUMP"); out != "" {
		dst, err := os.Create(out)
		if err != nil {
			t.Fatalf("dump: %v", err)
		}
		defer dst.Close()
		if err := png.Encode(dst, frame); err != nil {
			t.Fatalf("encode: %v", err)
		}
		t.Logf("wrote %s", out)
	}
	t.Logf("captured %dx%d, %d%% non-black", w, h, lit*100/px)
}
