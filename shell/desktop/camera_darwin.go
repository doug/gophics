//go:build darwin && !ios && !js

package desktop

import (
	"errors"
	"image"
	"time"

	"github.com/doug/gophics/internal/camera"
	"github.com/doug/gophics/shell"
)

// Live camera preview on macOS, over AVFoundation (internal/camera).
//
// Only macOS for now. Linux wants V4L2 and Windows Media Foundation, both plain
// C rather than Objective-C, so they are closer to the microphone paths than to
// this one — see camera_other.go for the placeholder they share.

// CameraPreview returns live camera preview.
//
// Non-nil even on a machine with no camera, for the same reason Microphone is:
// whether a usable device exists cannot be known without opening it, and on
// macOS opening it is what raises the permission prompt. A missing or refused
// camera surfaces as an error from Start, which is where an app can report it.
func (w *window) CameraPreview() shell.CameraPreview { return desktopCamera{} }

type desktopCamera struct{}

// Authorize reports the current capture authorization.
//
// It does not raise the prompt: on macOS that happens when the device is
// opened, so a "prompt" result means asking will happen on Start rather than
// now. Reporting it separately still matters, because a denied camera starts a
// session that simply delivers nothing — silence and refusal look identical
// from the frame side.
func (desktopCamera) Authorize(done func(shell.Permission)) {
	if done == nil {
		return
	}
	switch camera.Authorization() {
	case camera.StatusGranted:
		done(shell.PermissionGranted)
	case camera.StatusDenied:
		done(shell.PermissionDenied)
	default:
		done(shell.PermissionPrompt)
	}
}

// Start opens the camera and hands back a frame source.
func (desktopCamera) Start(o shell.PreviewOptions, done func(shell.Frames, error)) {
	if done == nil {
		return
	}
	facing := camera.FacingFront
	if o.Facing == shell.FacingBack {
		facing = camera.FacingBack
	}
	c, err := camera.Open(camera.Options{Facing: facing, Width: o.Width})
	if err != nil {
		done(nil, err)
		return
	}
	done(desktopFrames{c}, nil)
}

type desktopFrames struct{ c *camera.Capture }

func (f desktopFrames) Frame() *image.RGBA { return f.c.Frame() }

func (f desktopFrames) Stop() { f.c.Stop() }

// Camera returns still capture, taken from the preview stream.
//
// There is no separate stills path on macOS worth having: AVCapturePhotoOutput
// exists, but it means a second output, a second delegate class built at
// runtime, and a shutter sound the user did not ask for — to produce the same
// pixels the preview is already delivering. One frame off the stream is the
// same photo, and it reuses a path that a hardware test already covers.
func (w *window) Camera() shell.Camera { return desktopStill{} }

type desktopStill struct{}

func (desktopStill) Authorize(done func(shell.Permission)) { desktopCamera{}.Authorize(done) }

// captureTimeout bounds the wait for the first frame. A camera that opens but
// never delivers — in use by another app, or covered by a privacy shutter —
// would otherwise hang the callback forever.
const captureTimeout = 5 * time.Second

func (desktopStill) Capture(o shell.CaptureOptions, done func(image.Image, error)) {
	if done == nil {
		return
	}
	facing := camera.FacingFront
	if o.Facing == shell.FacingBack {
		facing = camera.FacingBack
	}
	c, err := camera.Open(camera.Options{Facing: facing, Width: o.MaxDim})
	if err != nil {
		done(nil, err)
		return
	}
	defer c.Stop()

	deadline := time.Now().Add(captureTimeout)
	for {
		if f := c.Frame(); f != nil {
			// Copied because the capture reuses its pool: the caller keeps
			// this image, and the next frame would otherwise overwrite it.
			out := image.NewRGBA(f.Rect)
			copy(out.Pix, f.Pix)
			done(out, nil)
			return
		}
		if time.Now().After(deadline) {
			done(nil, errors.New("camera: no frame within 5s"))
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}
