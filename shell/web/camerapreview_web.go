//go:build js && wasm

package web

import (
	"image"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Web implementation of the camera-preview capability: getUserMedia video into
// per-frame RGBA reads. Split from microphone_web.go, which had accreted five
// media concerns; the shared getUserMedia plumbing lives in mediautil_web.go.

// CameraPreview returns the live-preview capability, or nil where getUserMedia
// isn't available — an insecure context, or a browser without it.
func (w *window) CameraPreview() shell.CameraPreview {
	if md := mediaDevices(); md.IsUndefined() {
		return nil
	}
	return &webPreview{doc: w.doc}
}

// Microphone returns the live-monitoring capability, or nil without both
// getUserMedia and Web Audio.

type webPreview struct{ doc js.Value }

func (p *webPreview) Authorize(cb func(shell.Permission)) {
	if cb == nil {
		return
	}
	requestStream(map[string]any{"video": true}, func(stream js.Value, err error) {
		if err != nil {
			cb(shell.PermissionDenied)
			return
		}
		stopTracks(stream)
		cb(shell.PermissionGranted)
	})
}

func (p *webPreview) Start(opts shell.PreviewOptions, done func(shell.Frames, error)) {
	if done == nil {
		return // result-only: the Frames handle is the whole point
	}
	video := map[string]any{}
	if opts.Facing == shell.FacingFront {
		video["facingMode"] = "user"
	} else {
		video["facingMode"] = "environment"
	}
	if opts.Width > 0 {
		// "ideal", not "exact": a camera that has no mode at this width should
		// give us its nearest one, not fail to open at all.
		video["width"] = map[string]any{"ideal": opts.Width}
	}

	requestStream(map[string]any{"video": video}, func(stream js.Value, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		el := p.doc.Call("createElement", "video")
		el.Set("srcObject", stream)
		el.Set("muted", true)       // a preview must never echo the room
		el.Set("playsInline", true) // iOS Safari otherwise takes the video fullscreen
		el.Set("autoplay", true)
		el.Call("play")
		done(&webFrames{stream: stream, video: el, doc: p.doc}, nil)
	})
}

// webFrames pulls pixels out of a <video> through a 2D canvas.
type webFrames struct {
	doc           js.Value
	stream, video js.Value
	canvas, ctx   js.Value
	w, h          int
	pool          [3]*image.RGBA
	next          int
	cur           *image.RGBA
	lastTime      float64
	stopped       bool
	u8ctor        js.Value
}

func (f *webFrames) Frame() *image.RGBA {
	if f.stopped {
		return f.cur
	}
	// readyState < HAVE_CURRENT_DATA: no frame is decoded right now. That is the
	// first few hundred milliseconds of every preview, and it can also happen
	// mid-stream if the track stalls — so hold the last frame rather than
	// returning nil, which would blink the picture out.
	if f.video.Get("readyState").Int() < 2 {
		return f.cur
	}
	// currentTime only advances when the element has a genuinely new frame, so
	// this is what stops a 120 Hz repaint from doing 120 readbacks a second off
	// a 30 Hz camera. The first frame passes because lastTime starts at zero.
	t := f.video.Get("currentTime").Float()
	if t == f.lastTime && f.cur != nil {
		return f.cur
	}
	f.lastTime = t

	if f.w == 0 && !f.setup() {
		return nil
	}
	f.ctx.Call("drawImage", f.video, 0, 0, f.w, f.h)
	data := f.ctx.Call("getImageData", 0, 0, f.w, f.h).Get("data")

	img := f.pool[f.next]
	f.next = (f.next + 1) % len(f.pool)
	// getImageData hands back a Uint8ClampedArray, which CopyBytesToGo won't
	// take; a Uint8Array view over the same buffer is free and it will.
	view := f.u8ctor.New(data.Get("buffer"), data.Get("byteOffset"), len(img.Pix))
	js.CopyBytesToGo(img.Pix, view)
	f.cur = img
	return img
}

// setup allocates the canvas and the frame pool once the camera has told us
// what size it actually chose.
func (f *webFrames) setup() bool {
	w, h := f.video.Get("videoWidth").Int(), f.video.Get("videoHeight").Int()
	if w <= 0 || h <= 0 {
		return false
	}
	f.w, f.h = w, h
	f.canvas = f.doc.Call("createElement", "canvas")
	f.canvas.Set("width", w)
	f.canvas.Set("height", h)
	// willReadFrequently tells the browser to keep this canvas on the CPU.
	// Without it, every getImageData stalls on a GPU readback.
	f.ctx = f.canvas.Call("getContext", "2d", map[string]any{"willReadFrequently": true})
	for i := range f.pool {
		f.pool[i] = image.NewRGBA(image.Rect(0, 0, w, h))
	}
	f.u8ctor = js.Global().Get("Uint8Array")
	return true
}

func (f *webFrames) Stop() {
	if f.stopped {
		return
	}
	f.stopped = true
	f.video.Call("pause")
	f.video.Set("srcObject", js.Null())
	stopTracks(f.stream)
	f.cur, f.canvas, f.ctx = nil, js.Undefined(), js.Undefined()
}
