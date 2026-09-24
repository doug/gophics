//go:build ((darwin && !ios) || (linux && !android) || windows) && !js

package devmedia

import (
	"testing"
	"time"

	"github.com/doug/gophics/shell"
)

// The shell wiring has to deliver real frames, not just compile.
//
// internal/camera is tested by opening the device directly; this covers the
// layer above it — the options translation, the Frames adapter, and Stop
// actually releasing the camera. Those are exactly the joins where a working
// capture path turns into a black rectangle.
//
// Skipped where there is no camera or no grant: this runs on developer
// machines and CI runners alike, and a runner with no webcam is not a failure.
func TestDesktopCameraDeliversFrames(t *testing.T) {
	var status shell.Permission
	deviceCamera{}.Authorize(func(p shell.Permission) { status = p })
	if status == shell.PermissionDenied {
		t.Skip("camera access denied on this machine")
	}

	frames, err := startCamera(t, shell.PreviewOptions{Facing: shell.FacingFront, Width: 640})
	if err != nil {
		t.Skipf("no camera available: %v", err)
	}
	if frames == nil {
		t.Fatal("Start reported no error and no frames")
	}
	defer frames.Stop()

	// The camera takes a moment to warm up; poll rather than assuming.
	deadline := time.Now().Add(5 * time.Second)
	var first, second any
	for time.Now().Before(deadline) {
		f := frames.Frame()
		if f == nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		if first == nil {
			first = any(f)
			if b := f.Bounds(); b.Dx() <= 0 || b.Dy() <= 0 {
				t.Fatalf("a frame arrived with an empty bounds: %v", b)
			}
			continue
		}
		if any(f) != first {
			second = any(f)
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if first == nil {
		t.Fatal("no frame arrived within 5s; the camera opened and delivered nothing")
	}
	// Successive frames must be distinct values. The scene compares images by
	// identity, so a source that repainted one buffer in place would never
	// cause a repaint — the preview would freeze on frame one.
	if second == nil {
		t.Error("the frame value never changed; a preview that hands back the " +
			"same image will never repaint")
	}
}

// Stop must be safe to call twice: a widget that stops on dispose and again on
// an error path is ordinary, and the second call must not take the process down.
func TestDesktopCameraStopIsIdempotent(t *testing.T) {
	frames, err := startCamera(t, shell.PreviewOptions{})
	if err != nil || frames == nil {
		t.Skipf("no camera available: %v", err)
	}
	frames.Stop()
	frames.Stop()
}

// startCamera runs Start and waits for its answer. Start reports off the
// caller's goroutine — the open and the first-frame wait take seconds on a
// cold device — so a test has to wait for done the way the Posted wrapper
// would deliver it.
func startCamera(t *testing.T, o shell.PreviewOptions) (shell.Frames, error) {
	t.Helper()
	type result struct {
		frames shell.Frames
		err    error
	}
	got := make(chan result, 1)
	deviceCamera{}.Start(o, func(f shell.Frames, e error) { got <- result{f, e} })
	select {
	case r := <-got:
		return r.frames, r.err
	case <-time.After(firstFrameTimeout + 5*time.Second):
		t.Fatal("Start never reported")
		return nil, nil
	}
}
