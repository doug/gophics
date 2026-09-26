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

// TestHardwareOpenStopCycles opens and stops the camera repeatedly, with a
// Stop that lands while frames are flowing and one that lands before the
// first frame has arrived.
//
// Same opt-in as above. It is here because teardown used to race the frame
// path on two of the three backends: Linux unmapped the V4L2 ring while the
// stream goroutine could still be converting out of it, and Windows released
// the source reader while that goroutine was blocked inside ReadSample on it.
// Neither shows up in a single open/capture/stop, and both are the kind of
// crash a preview page that is opened and closed a few times produces. macOS
// leaked a session, an output, a delegate and a queue per cycle instead.
//
// The odd cycles, where Stop lands before the first frame, are the closest a
// working camera gets to the case the Windows join then got wrong: a device
// that never delivers leaves ReadSample blocked, and a Stop that only waited
// for it hung for good. That case needs a camera that produces nothing —
// unplugged but enumerated, or passed through to a VM — which this test
// cannot arrange; Stop's bounded join is what covers it.
func TestHardwareOpenStopCycles(t *testing.T) {
	if os.Getenv("GOPHICS_CAMERA_HW") == "" {
		t.Skip("set GOPHICS_CAMERA_HW=1 to run against a real camera")
	}
	for i := 0; i < 6; i++ {
		c, err := Open(Options{Facing: FacingFront, Width: 640})
		if err != nil {
			t.Fatalf("cycle %d: open: %v", i, err)
		}
		if i%2 == 0 {
			// Let frames flow so Stop interrupts a live stream.
			deadline := time.Now().Add(3 * time.Second)
			for c.Frame() == nil && time.Now().Before(deadline) {
				time.Sleep(20 * time.Millisecond)
			}
		}
		c.Stop()
		c.Stop() // idempotent
		if c.Frame() != nil && i%2 != 0 {
			t.Logf("cycle %d: a frame arrived before Stop", i)
		}
	}
}
