package mobile

import (
	"errors"
	"image"
	"sync"

	"github.com/doug/gophics/shell"
)

// Live camera preview on Android and iOS.
//
// The shape follows the microphone (livemedia.go) because the constraints are
// the same: the device belongs to the host, Go cannot open it, and frames have
// to cross the bind boundary without allocating per frame if the preview is to
// keep up. Go asks the host to start; the host reports back through the
// Deliver*/Fail* methods, correlating by request ID.
//
// Threading matches the microphone's, and for the same reasons:
//
//   - AuthorizeCamera, DeliverPreviewReady and FailPreview MUST arrive on the
//     host's UI thread. They run app callbacks that mutate the widget tree.
//   - DeliverPreviewFrame MUST NOT. It is called from the camera's own thread
//     tens of times a second, touches no app code, and marshalling it through
//     the UI thread would add a frame of latency to the one path whose whole
//     purpose is to be current.

// PreviewHost is implemented by the native host (Android Camera2, iOS
// AVFoundation) and registered with Bridge.SetPreviewHost.
type PreviewHost interface {
	// AuthorizeCamera requests camera permission. → DeliverPermission(reqID, granted)
	AuthorizeCamera(reqID int)
	// StartPreview opens the camera and begins streaming frames.
	// facing is 0 for front, 1 for back; width is a hint the host may ignore.
	// → DeliverPreviewReady(reqID) | FailPreview(reqID, msg), then
	// DeliverPreviewFrame(reqID, rgba, w, h) repeatedly until stopped.
	StartPreview(reqID, facing, width int)
	// StopPreview closes the camera and ends the stream.
	StopPreview(reqID int)
}

// SetPreviewHost registers the native camera backend. Until it is set,
// CameraPreview() returns nil and an app hides the affordance.
func (b *Bridge) SetPreviewHost(h PreviewHost) {
	b.prevMu.Lock()
	defer b.prevMu.Unlock()
	b.prevHost = h
	if b.previews == nil {
		b.previews = map[int]*mobilePreview{}
	}
	if b.prevCb == nil {
		b.prevCb = map[int]func(shell.Frames, error){}
	}
}

type mobileCameraPreview struct{ b *Bridge }

func (c *mobileCameraPreview) Authorize(cb func(shell.Permission)) {
	b := c.b
	b.prevMu.Lock()
	host := b.prevHost
	b.prevMu.Unlock()
	if host == nil {
		cb(shell.PermissionDenied)
		return
	}
	id := b.media.newReq()
	b.media.perm[id] = cb
	host.AuthorizeCamera(id)
}

func (c *mobileCameraPreview) Start(o shell.PreviewOptions, done func(shell.Frames, error)) {
	b := c.b
	b.prevMu.Lock()
	host := b.prevHost
	if host == nil {
		b.prevMu.Unlock()
		done(nil, errors.New("no camera on this device"))
		return
	}
	id := b.media.newReq()
	b.prevCb[id] = done
	b.prevMu.Unlock()

	facing := 0
	if o.Facing == shell.FacingBack {
		facing = 1
	}
	host.StartPreview(id, facing, o.Width)
}

// mobilePreview holds the frames arriving from the host.
//
// Frames rotate through a small pool rather than repainting one image, because
// the scene compares images by identity: handing back the same *image.RGBA with
// new pixels would never trigger a repaint. Three is enough that the one being
// drawn is never the one being written.
type mobilePreview struct {
	b  *Bridge
	id int

	mu      sync.Mutex
	pool    [3]*image.RGBA
	cur     int
	last    *image.RGBA
	stopped bool
}

func (p *mobilePreview) Frame() *image.RGBA {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.last
}

func (p *mobilePreview) Stop() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	b := p.b
	b.prevMu.Lock()
	host := b.prevHost
	delete(b.previews, p.id)
	b.prevMu.Unlock()

	if host != nil {
		host.StopPreview(p.id)
	}
}

// deliver copies one frame in. rgba is the host's buffer and is not retained.
func (p *mobilePreview) deliver(rgba []byte, w, h int) {
	if w <= 0 || h <= 0 || len(rgba) < w*h*4 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return
	}
	i := p.cur % len(p.pool)
	img := p.pool[i]
	if img == nil || img.Rect.Dx() != w || img.Rect.Dy() != h {
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		p.pool[i] = img
	}
	copy(img.Pix, rgba[:w*h*4])
	p.cur++
	p.last = img
}

// DeliverPreviewReady signals that the camera is open; the app's Start callback
// receives the frame source. Call on the host's UI thread.
func (b *Bridge) DeliverPreviewReady(reqID int) {
	b.prevMu.Lock()
	done := b.prevCb[reqID]
	delete(b.prevCb, reqID)
	p := &mobilePreview{b: b, id: reqID}
	b.previews[reqID] = p
	b.prevMu.Unlock()

	if done != nil {
		done(p, nil)
	}
}

// FailPreview reports that the camera could not be opened. Call on the host's
// UI thread.
func (b *Bridge) FailPreview(reqID int, msg string) {
	b.prevMu.Lock()
	done := b.prevCb[reqID]
	delete(b.prevCb, reqID)
	b.prevMu.Unlock()

	if done != nil {
		if msg == "" {
			msg = "the camera could not be opened"
		}
		done(nil, errors.New(msg))
	}
}

// DeliverPreviewFrame hands one RGBA8888 frame to the preview. Call from the
// camera thread, not the UI thread: this touches no app code, and routing it
// through the UI thread would add a frame of latency.
//
// The slice is copied; the host may reuse its buffer immediately.
func (b *Bridge) DeliverPreviewFrame(reqID int, rgba []byte, w, h int) {
	b.prevMu.Lock()
	p := b.previews[reqID]
	b.prevMu.Unlock()
	if p != nil {
		p.deliver(rgba, w, h)
	}
}
