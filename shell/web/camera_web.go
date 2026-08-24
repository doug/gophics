//go:build js && wasm

// Web implementation of the shell media-capture capabilities (shell/media.go).
//
// Camera capture uses a hidden <input type=file accept=image/* capture> — the
// idiomatic web camera: it opens the real camera UI on mobile and a file dialog
// on desktop, and needs no permission prompt. Audio recording uses getUserMedia
// + MediaRecorder (the clip) with a Web Audio AnalyserNode for the live level;
// playback decodes with Web Audio. This is the M1 target that settles the Go API
// before the native (gomobile) shells implement the same interfaces.
package web

import (
	"bytes"
	"errors"
	"image"
	_ "image/jpeg" // register decoders for captured photos
	_ "image/png"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// audioContextCtor returns the AudioContext constructor (or the webkit-prefixed
// one on older Safari), or undefined when Web Audio is unavailable.
func audioContextCtor() js.Value {
	c := js.Global().Get("AudioContext")
	if c.IsUndefined() {
		c = js.Global().Get("webkitAudioContext")
	}
	return c
}

// Camera returns the still-capture capability (always available on web via the
// file/camera input).
func (w *window) Camera() shell.Camera {
	if w.cam == nil {
		w.cam = &webCamera{doc: w.doc}
	}
	return w.cam
}

// --- Camera ------------------------------------------------------------------

type webCamera struct{ doc js.Value }

// Authorize is a no-op for the file/camera input path (no permission needed).
func (c *webCamera) Authorize(cb func(shell.Permission)) { cb(shell.PermissionGranted) }

func (c *webCamera) Capture(opts shell.CaptureOptions, done func(image.Image, error)) {
	input := c.doc.Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", "image/*")
	// The capture attribute hints the device camera (mobile); ignored on desktop.
	if opts.Facing == shell.FacingFront {
		input.Set("capture", "user")
	} else {
		input.Set("capture", "environment")
	}
	var onChange js.Func
	onChange = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		files := input.Get("files")
		if files.Length() == 0 {
			onChange.Release()
			done(nil, errors.New("no photo selected"))
			return nil
		}
		file := files.Index(0)
		go func() {
			defer onChange.Release()
			buf, err := await(file.Call("arrayBuffer"))
			if err != nil {
				done(nil, err)
				return
			}
			data := jsToBytes(js.Global().Get("Uint8Array").New(buf))
			img, _, derr := image.Decode(bytes.NewReader(data))
			done(img, derr)
		}()
		return nil
	})
	input.Call("addEventListener", "change", onChange)
	input.Call("click") // relies on the calling user gesture (Capture is called from a tap)
}

// --- Audio -------------------------------------------------------------------
