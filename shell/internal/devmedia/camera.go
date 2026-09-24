//go:build ((darwin && !ios) || (linux && !android) || windows) && !js

package devmedia

import (
	"errors"
	"image"
	"time"

	"github.com/doug/gophics/internal/camera"
	"github.com/doug/gophics/shell"
)

// Live camera preview and still capture, over internal/camera.
//
// macOS goes through AVFoundation, Linux through V4L2 and Windows through
// Media Foundation, but internal/camera presents all three identically, so
// this adapter is written once.

// CameraPreview returns live camera preview.
//
// Non-nil even on a machine with no camera, for the same reason Microphone is:
// whether a usable device exists cannot be known without opening it, and on
// macOS opening it is what raises the permission prompt. A missing or refused
// camera surfaces as an error from Start, which is where an app can report it.
func CameraPreview() shell.CameraPreview { return deviceCamera{} }

type deviceCamera struct{}

// Authorize reports the current capture authorization.
//
// It does not raise the prompt: on macOS that happens when the device is
// opened, so a "prompt" result means asking will happen on Start rather than
// now. Reporting it separately still matters, because a denied camera starts a
// session that simply delivers nothing — silence and refusal look identical
// from the frame side.
func (deviceCamera) Authorize(done func(shell.Permission)) {
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

// capture is what Start and Capture need from an open camera. An interface
// rather than *camera.Capture so the wait for the first frame can be tested
// with a fake, on machines that have no camera.
type capture interface {
	Frame() *image.RGBA
	Stop()
}

// openCamera is camera.Open behind the interface; a test substitutes it.
var openCamera = func(o camera.Options) (capture, error) {
	c, err := camera.Open(o)
	if err != nil {
		return nil, err // not the typed nil, which would be a non-nil capture
	}
	return c, nil
}

// Start opens the camera and hands back a frame source.
//
// The open and the wait run on their own goroutine, as every other slow
// desktop capability's work does (the file pickers, folder I/O, the
// notifiers). Start is reached through the generated Posted wrapper on the UI
// goroutine, and the wrapper marshals done back there — so doing this work
// inline froze the whole app for the camera's warm-up, and for the full
// timeout when the camera never delivered.
func (deviceCamera) Start(o shell.PreviewOptions, done func(shell.Frames, error)) {
	if done == nil {
		return
	}
	facing := camera.FacingFront
	if o.Facing == shell.FacingBack {
		facing = camera.FacingBack
	}
	go func() {
		c, err := openCamera(camera.Options{Facing: facing, Width: o.Width})
		if err != nil {
			done(nil, err)
			return
		}
		// Wait for the first frame before reporting success.
		//
		// Opening proves almost nothing: a camera that has been unplugged or
		// handed to a virtual machine still enumerates, still reports itself
		// connected and not in use, and still starts a session — it simply
		// never delivers. An app that trusted Open then shows a black
		// rectangle forever with nothing to tell the user, which is exactly
		// what this machine did once its only camera was passed through to a
		// VM.
		//
		// The cost is one frame of latency on the path that works, in exchange
		// for the guarantee that a Frames handed to an app has already
		// produced one.
		if !waitForFrame(c, firstFrameTimeout) {
			c.Stop()
			done(nil, camera.ErrNoFrames)
			return
		}
		done(deviceFrames{c}, nil)
	}()
}

// firstFrameTimeout bounds that wait. Cameras take a moment to expose and
// focus, and a cold USB device can take longer than a built-in one; three
// seconds is well past either and still short enough to report rather than
// hang.
const firstFrameTimeout = 3 * time.Second

func waitForFrame(c capture, within time.Duration) bool {
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if c.Frame() != nil {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return c.Frame() != nil
}

type deviceFrames struct{ c capture }

func (f deviceFrames) Frame() *image.RGBA { return f.c.Frame() }

func (f deviceFrames) Stop() { f.c.Stop() }

// Camera returns still capture, taken from the preview stream.
//
// There is no separate stills path on macOS worth having: AVCapturePhotoOutput
// exists, but it means a second output, a second delegate class built at
// runtime, and a shutter sound the user did not ask for — to produce the same
// pixels the preview is already delivering. One frame off the stream is the
// same photo, and it reuses a path that a hardware test already covers.
func Camera() shell.Camera { return deviceStill{} }

type deviceStill struct{}

func (deviceStill) Authorize(done func(shell.Permission)) { deviceCamera{}.Authorize(done) }

// captureTimeout bounds the wait for the first frame. A camera that opens but
// never delivers — in use by another app, or covered by a privacy shutter —
// would otherwise hang the callback forever.
const captureTimeout = 5 * time.Second

// Capture opens the camera, takes the first frame it delivers, and closes it.
// Off the caller's goroutine for the same reason as Start: this is reached
// from the UI goroutine and can take seconds.
func (deviceStill) Capture(o shell.CaptureOptions, done func(image.Image, error)) {
	if done == nil {
		return
	}
	facing := camera.FacingFront
	if o.Facing == shell.FacingBack {
		facing = camera.FacingBack
	}
	go func() {
		c, err := openCamera(camera.Options{Facing: facing, Width: o.MaxDim})
		if err != nil {
			done(nil, err)
			return
		}
		defer c.Stop()

		if !waitForFrame(c, captureTimeout) {
			done(nil, errors.New("devmedia: no frame within 5s"))
			return
		}
		f := c.Frame()
		// Copied because the capture reuses its pool: the caller keeps this
		// image, and the next frame would otherwise overwrite it.
		out := image.NewRGBA(f.Rect)
		copy(out.Pix, f.Pix)
		done(out, nil)
	}()
}
