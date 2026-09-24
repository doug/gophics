//go:build ((darwin && !ios) || (linux && !android) || windows) && !js

package devmedia

import (
	"errors"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/doug/gophics/internal/camera"
	"github.com/doug/gophics/shell"
)

// slowCamera delivers its first frame only after warmup, like a real one.
type slowCamera struct {
	warmup time.Duration
	opened time.Time

	mu      sync.Mutex
	stopped bool
	frame   *image.RGBA
}

func (c *slowCamera) Frame() *image.RGBA {
	if time.Since(c.opened) < c.warmup {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.frame == nil {
		c.frame = image.NewRGBA(image.Rect(0, 0, 2, 2))
		c.frame.Pix[0] = 0x7f
	}
	return c.frame
}

func (c *slowCamera) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
}

func (c *slowCamera) isStopped() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stopped
}

func useCamera(t *testing.T, open func(camera.Options) (capture, error)) {
	t.Helper()
	old := openCamera
	openCamera = open
	t.Cleanup(func() { openCamera = old })
}

// Start is called on the UI goroutine through the Posted wrapper, and a
// camera takes a moment to expose — up to firstFrameTimeout when it never
// does. That wait used to run inline, freezing the app for it. Start must
// return at once and report through done later.
func TestStartReturnsBeforeTheFirstFrame(t *testing.T) {
	const warmup = 300 * time.Millisecond
	cam := &slowCamera{warmup: warmup}
	useCamera(t, func(camera.Options) (capture, error) { cam.opened = time.Now(); return cam, nil })

	type result struct {
		frames shell.Frames
		err    error
	}
	got := make(chan result, 1)
	began := time.Now()
	deviceCamera{}.Start(shell.PreviewOptions{}, func(f shell.Frames, err error) { got <- result{f, err} })
	if took := time.Since(began); took >= warmup {
		t.Fatalf("Start blocked for %v; the camera's warm-up ran on the caller's goroutine", took)
	}

	select {
	case r := <-got:
		if r.err != nil || r.frames == nil {
			t.Fatalf("Start reported %v, %v", r.frames, r.err)
		}
		if r.frames.Frame() == nil {
			t.Error("a Frames was handed over before it had produced a frame")
		}
		r.frames.Stop()
		if !cam.isStopped() {
			t.Error("Stop did not reach the camera")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("done never fired")
	}
}

// Capture is the same shape with a still at the end: it must return at once,
// deliver a copy of the frame, and release the camera afterwards.
func TestCaptureReturnsBeforeTheFrameAndReleasesTheCamera(t *testing.T) {
	const warmup = 300 * time.Millisecond
	cam := &slowCamera{warmup: warmup}
	useCamera(t, func(camera.Options) (capture, error) { cam.opened = time.Now(); return cam, nil })

	type result struct {
		img image.Image
		err error
	}
	got := make(chan result, 1)
	began := time.Now()
	deviceStill{}.Capture(shell.CaptureOptions{}, func(img image.Image, err error) { got <- result{img, err} })
	if took := time.Since(began); took >= warmup {
		t.Fatalf("Capture blocked for %v; the camera's warm-up ran on the caller's goroutine", took)
	}

	select {
	case r := <-got:
		if r.err != nil || r.img == nil {
			t.Fatalf("Capture reported %v, %v", r.img, r.err)
		}
		out, ok := r.img.(*image.RGBA)
		if !ok || out == cam.frame || &out.Pix[0] == &cam.frame.Pix[0] {
			t.Error("Capture handed back the camera's own pooled frame rather than a copy")
		}
		if out.Pix[0] != 0x7f {
			t.Errorf("pixel data did not survive the copy: %#x", out.Pix[0])
		}
		if !cam.isStopped() {
			t.Error("Capture did not release the camera after taking its frame")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("done never fired")
	}
}

// A camera that cannot be opened still reports — through done, off the
// caller — rather than being lost on the goroutine.
func TestStartReportsOpenFailure(t *testing.T) {
	boom := errors.New("no camera")
	useCamera(t, func(camera.Options) (capture, error) { return nil, boom })
	got := make(chan error, 1)
	deviceCamera{}.Start(shell.PreviewOptions{}, func(_ shell.Frames, err error) { got <- err })
	select {
	case err := <-got:
		if !errors.Is(err, boom) {
			t.Errorf("Start reported %v, want %v", err, boom)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("done never fired")
	}
}
