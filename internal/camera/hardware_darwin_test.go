//go:build darwin && !ios

package camera

import (
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
	var f = c.Frame()
	for f == nil && time.Now().Before(deadline) {
		time.Sleep(50 * time.Millisecond)
		f = c.Frame()
	}
	if f == nil {
		t.Fatal("no frame within 6s")
	}

	w, h := f.Rect.Dx(), f.Rect.Dy()
	if w <= 0 || h <= 0 {
		t.Fatalf("empty frame %dx%d", w, h)
	}
	if got, want := len(f.Pix), w*h*4; got != want {
		t.Fatalf("pix length %d, want %d for %dx%d", got, want, w, h)
	}

	// A frame that is uniformly zero means the copy read the wrong memory; a
	// frame with a zero alpha channel means the conversion dropped a lane.
	var lit, opaque int
	for i := 0; i < len(f.Pix); i += 4 {
		if int(f.Pix[i])+int(f.Pix[i+1])+int(f.Pix[i+2]) > 0 {
			lit++
		}
		if f.Pix[i+3] == 0xFF {
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
	t.Logf("captured %dx%d, %d%% non-black", w, h, lit*100/px)
}
