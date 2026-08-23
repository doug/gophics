// Package camera captures live video frames from the system camera.
//
// Each platform gets there its own way — AVFoundation on macOS, V4L2 on Linux
// — but they agree on what a caller sees: Open returns a Capture, Frame
// returns the newest image or nil before the first arrives, and Stop releases
// the device. Everything above this package is written once.
//
// Access is gated differently per platform, so Authorization is reported
// separately rather than inferred from silence: a denied camera and a camera
// pointed at a dark room both deliver nothing, and only the first is worth
// telling the user about.
package camera

import (
	"image"
	"sync"
)

// Facing selects a camera on a device with more than one.
type Facing int

const (
	FacingFront Facing = iota
	FacingBack
)

// Status is the current capture authorization.
type Status int

const (
	StatusPrompt  Status = iota // not yet decided; opening the camera asks
	StatusGranted               // allowed
	StatusDenied                // refused, or restricted by policy
)

// Options configures a capture.
type Options struct {
	Facing Facing
	// Width is a hint. The camera picks the nearest mode it has, so read the
	// real size off the image rather than assuming this one.
	Width int
}

// frames is the buffer rotation every backend needs.
//
// Frames rotate through a small pool rather than repainting one image, because
// the scene compares images by identity: handing back the same *image.RGBA
// with new pixels would never trigger a repaint. Three is enough that the one
// being drawn is never the one being written.
type frames struct {
	mu      sync.Mutex
	pool    [3]*image.RGBA
	cur     int
	last    *image.RGBA
	stopped bool
}

// deliver publishes one frame. fill writes the pixels into the pool buffer,
// which is sized and reused for it; it runs under the lock and on whatever
// thread the platform delivers frames on, so it must only convert.
func (f *frames) deliver(w, h int, fill func(dst *image.RGBA)) {
	if w <= 0 || h <= 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return
	}
	i := f.cur % len(f.pool)
	img := f.pool[i]
	if img == nil || img.Rect.Dx() != w || img.Rect.Dy() != h {
		img = image.NewRGBA(image.Rect(0, 0, w, h))
		f.pool[i] = img
	}
	fill(img)
	f.cur++
	f.last = img
}

// Frame returns the newest frame, or nil before the first arrives.
func (f *frames) Frame() *image.RGBA {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.last
}

// stop marks capture finished, reporting false if it already was.
func (f *frames) stop() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.stopped {
		return false
	}
	f.stopped = true
	return true
}
