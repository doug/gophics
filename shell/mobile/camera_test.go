package mobile

import (
	"sync"
	"testing"

	"github.com/doug/gophics/shell"
)

// fakePreviewHost stands in for Android Camera2 / iOS AVFoundation.
type fakePreviewHost struct {
	mu       sync.Mutex
	started  []int
	stopped  []int
	facing   int
	width    int
	authReqs []int
}

func (h *fakePreviewHost) AuthorizeCamera(reqID int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.authReqs = append(h.authReqs, reqID)
}

func (h *fakePreviewHost) StartPreview(reqID, facing, width int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.started = append(h.started, reqID)
	h.facing, h.width = facing, width
}

func (h *fakePreviewHost) StopPreview(reqID int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.stopped = append(h.stopped, reqID)
}

func newPreviewBridge(t *testing.T) (*Bridge, *fakePreviewHost) {
	t.Helper()
	b := NewBridge(nil)
	h := &fakePreviewHost{}
	b.SetPreviewHost(h)
	return b, h
}

// Without a host there is no camera, and an app must be able to see that
// rather than be handed something that never delivers.
func TestCameraPreviewNilWithoutAHost(t *testing.T) {
	if got := NewBridge(nil).CameraPreview(); got != nil {
		t.Error("a Bridge with no PreviewHost still published a camera")
	}
}

// The whole path: start, frames arrive, Frame returns the newest.
func TestPreviewDeliversFrames(t *testing.T) {
	b, host := newPreviewBridge(t)

	var frames shell.Frames
	var err error
	b.CameraPreview().Start(shell.PreviewOptions{Facing: shell.FacingBack, Width: 640},
		func(f shell.Frames, e error) { frames, err = f, e })

	host.mu.Lock()
	if len(host.started) != 1 {
		host.mu.Unlock()
		t.Fatalf("the host was asked to start %d times, want 1", len(host.started))
	}
	id := host.started[0]
	if host.facing != 1 {
		t.Errorf("facing reached the host as %d, want 1 (back)", host.facing)
	}
	if host.width != 640 {
		t.Errorf("width reached the host as %d, want 640", host.width)
	}
	host.mu.Unlock()

	// Nothing is delivered until the host says it is ready.
	if frames != nil || err != nil {
		t.Fatal("Start's callback ran before the host reported ready")
	}
	b.DeliverPreviewReady(id)
	if err != nil {
		t.Fatalf("ready reported an error: %v", err)
	}
	if frames == nil {
		t.Fatal("ready produced no frame source")
	}
	if frames.Frame() != nil {
		t.Error("a frame was available before any arrived")
	}

	px := make([]byte, 2*2*4)
	for i := range px {
		px[i] = 0x40
	}
	b.DeliverPreviewFrame(id, px, 2, 2)

	f := frames.Frame()
	if f == nil {
		t.Fatal("no frame after one was delivered")
	}
	if b := f.Bounds(); b.Dx() != 2 || b.Dy() != 2 {
		t.Errorf("frame is %v, want 2x2", b)
	}
	if f.Pix[0] != 0x40 {
		t.Errorf("pixel data did not survive: got %#x", f.Pix[0])
	}
}

// Successive frames must be distinct image values. The scene compares images
// by identity, so a source that repainted one buffer would never repaint.
func TestPreviewFramesRotate(t *testing.T) {
	b, host := newPreviewBridge(t)
	var frames shell.Frames
	b.CameraPreview().Start(shell.PreviewOptions{}, func(f shell.Frames, _ error) { frames = f })
	host.mu.Lock()
	id := host.started[0]
	host.mu.Unlock()
	b.DeliverPreviewReady(id)

	px := make([]byte, 4*4*4)
	seen := map[any]bool{}
	for i := range 3 {
		px[0] = byte(i + 1)
		b.DeliverPreviewFrame(id, px, 4, 4)
		seen[any(frames.Frame())] = true
	}
	if len(seen) < 3 {
		t.Errorf("three frames produced %d distinct images; a preview that hands "+
			"back the same image will never repaint", len(seen))
	}
}

// A frame that does not match its dimensions is dropped rather than read past
// the end of the host's buffer.
func TestPreviewRejectsShortFrames(t *testing.T) {
	b, host := newPreviewBridge(t)
	var frames shell.Frames
	b.CameraPreview().Start(shell.PreviewOptions{}, func(f shell.Frames, _ error) { frames = f })
	host.mu.Lock()
	id := host.started[0]
	host.mu.Unlock()
	b.DeliverPreviewReady(id)

	b.DeliverPreviewFrame(id, make([]byte, 8), 64, 64) // far too small
	if frames.Frame() != nil {
		t.Error("a frame shorter than its declared size was accepted")
	}
}

// A failure has to reach the app, or Start's callback never fires and the UI
// waits forever on a camera that will not open.
func TestPreviewFailureReachesTheCaller(t *testing.T) {
	b, host := newPreviewBridge(t)
	var frames shell.Frames
	var err error
	b.CameraPreview().Start(shell.PreviewOptions{}, func(f shell.Frames, e error) { frames, err = f, e })
	host.mu.Lock()
	id := host.started[0]
	host.mu.Unlock()

	b.FailPreview(id, "camera in use")
	if err == nil {
		t.Fatal("a failed open reported no error")
	}
	if frames != nil {
		t.Error("a failed open still produced a frame source")
	}
}

// Stop must reach the host — the camera stays on, and lit, until it does — and
// must be safe to call twice.
func TestPreviewStopReachesTheHostOnce(t *testing.T) {
	b, host := newPreviewBridge(t)
	var frames shell.Frames
	b.CameraPreview().Start(shell.PreviewOptions{}, func(f shell.Frames, _ error) { frames = f })
	host.mu.Lock()
	id := host.started[0]
	host.mu.Unlock()
	b.DeliverPreviewReady(id)

	frames.Stop()
	frames.Stop()

	host.mu.Lock()
	defer host.mu.Unlock()
	if len(host.stopped) != 1 {
		t.Errorf("the host was told to stop %d times, want exactly 1", len(host.stopped))
	}
}

// Frames arriving after Stop must be dropped: the host's camera thread does not
// stop the instant Go asks it to.
func TestPreviewIgnoresFramesAfterStop(t *testing.T) {
	b, host := newPreviewBridge(t)
	var frames shell.Frames
	b.CameraPreview().Start(shell.PreviewOptions{}, func(f shell.Frames, _ error) { frames = f })
	host.mu.Lock()
	id := host.started[0]
	host.mu.Unlock()
	b.DeliverPreviewReady(id)
	frames.Stop()

	b.DeliverPreviewFrame(id, make([]byte, 4*4*4), 4, 4)
	if frames.Frame() != nil {
		t.Error("a frame delivered after Stop was accepted")
	}
}

// A frame already past the map lookup when Stop runs must still be dropped.
//
// Stop removes the preview from the Bridge, which turns away every frame that
// arrives afterwards — but not one already in flight on the camera thread,
// holding the pointer it read a moment earlier. That is the case the flag
// inside deliver exists for, and it is not reachable through the public path,
// so the test reaches the same way a racing frame would.
func TestPreviewDropsAFrameAlreadyInFlightAtStop(t *testing.T) {
	b, host := newPreviewBridge(t)
	var frames shell.Frames
	b.CameraPreview().Start(shell.PreviewOptions{}, func(f shell.Frames, _ error) { frames = f })
	host.mu.Lock()
	id := host.started[0]
	host.mu.Unlock()
	b.DeliverPreviewReady(id)

	// What the camera thread holds when it is about to deliver.
	b.prevMu.Lock()
	inflight := b.previews[id]
	b.prevMu.Unlock()
	if inflight == nil {
		t.Fatal("no preview registered")
	}

	frames.Stop()
	inflight.deliver(make([]byte, 4*4*4), 4, 4)

	if frames.Frame() != nil {
		t.Error("a frame that was in flight when Stop ran was still published; " +
			"the camera thread does not stop the instant Go asks it to")
	}
}

// Frames arrive on the camera thread while the UI goroutine reads them. Run
// with -race; without the mutex this is a data race, not a theoretical one.
func TestPreviewFrameDeliveryIsConcurrencySafe(t *testing.T) {
	b, host := newPreviewBridge(t)
	var frames shell.Frames
	b.CameraPreview().Start(shell.PreviewOptions{}, func(f shell.Frames, _ error) { frames = f })
	host.mu.Lock()
	id := host.started[0]
	host.mu.Unlock()
	b.DeliverPreviewReady(id)

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		px := make([]byte, 8*8*4)
		for range 200 {
			b.DeliverPreviewFrame(id, px, 8, 8)
		}
	}()
	go func() {
		defer wg.Done()
		for range 200 {
			_ = frames.Frame()
		}
	}()
	wg.Wait()
}
