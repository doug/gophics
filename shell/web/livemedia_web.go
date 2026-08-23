//go:build js && wasm

// Web implementation of the live-capture capabilities (shell.CameraPreview and
// shell.Microphone). Both sit on getUserMedia:
//
//   - The preview pipes the video track into a hidden <video>, draws it into an
//     offscreen <canvas> on demand, and copies the pixels back with getImageData.
//     There is no way to read a MediaStream's pixels in a browser that doesn't
//     go through a canvas, so the readback is the price of the capability.
//   - The monitor runs the audio track through a Web Audio AnalyserNode, which
//     does the FFT natively; Go only reads the bins out.
//
// Both are pull-based, so nothing is captured on a frame the app doesn't paint,
// and neither retains anything — a preview left running costs one frame of
// memory, not a growing buffer.
package web

import (
	"errors"
	"image"
	"math"
	"syscall/js"

	"github.com/doug/gophics/internal/mic"
	"github.com/doug/gophics/shell"
)

// The window opts into live capture by implementing shell.LiveMediaWindow;
// this is the compile-time check that it still does.
var _ shell.LiveMediaWindow = (*window)(nil)

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
func (w *window) Microphone() shell.Microphone {
	if md := mediaDevices(); md.IsUndefined() || audioContextCtor().IsUndefined() {
		return nil
	}
	return &webMicrophone{}
}

func mediaDevices() js.Value {
	md := js.Global().Get("navigator").Get("mediaDevices")
	if md.IsNull() {
		return js.Undefined()
	}
	return md
}

// --- Camera preview ----------------------------------------------------------

type webPreview struct{ doc js.Value }

func (p *webPreview) Authorize(cb func(shell.Permission)) {
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

// --- Microphone --------------------------------------------------------------

type webMicrophone struct{}

func (m *webMicrophone) Authorize(cb func(shell.Permission)) {
	requestStream(map[string]any{"audio": true}, func(stream js.Value, err error) {
		if err != nil {
			cb(shell.PermissionDenied)
			return
		}
		stopTracks(stream)
		cb(shell.PermissionGranted)
	})
}

func (m *webMicrophone) Listen(done func(shell.Monitor, error)) {
	requestStream(map[string]any{"audio": true}, func(stream js.Value, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		ctor := audioContextCtor()
		if ctor.IsUndefined() {
			stopTracks(stream)
			done(nil, errors.New("web audio unavailable"))
			return
		}
		mon := &webMonitor{stream: stream, audioCtx: ctor.New()}
		mon.source = mon.audioCtx.Call("createMediaStreamSource", stream)
		mon.analyser = mon.audioCtx.Call("createAnalyser")
		mon.analyser.Set("fftSize", fftSize)
		mon.analyser.Set("smoothingTimeConstant", 0.72) // otherwise the bars strobe
		mon.source.Call("connect", mon.analyser)
		// Deliberately not connected to the destination: an analyser is a sink
		// as far as the graph is concerned, and routing the mic to the speakers
		// would feed back.
		mon.bins = fftSize / 2
		mon.freqJS = js.Global().Get("Uint8Array").New(mon.bins)
		mon.timeJS = js.Global().Get("Uint8Array").New(fftSize)
		mon.freq = make([]byte, mon.bins)
		mon.time = make([]byte, fftSize)

		// A second analyser, wider and unsmoothed, feeds Samples. Sharing the
		// display analyser would force one window size to serve two jobs that
		// want opposite things: the meter wants a short window so the bars
		// track the attack, while pitch detection wants a long one so two full
		// periods of the lowest note fit. An extra AnalyserNode is a few
		// kilobytes and no extra capture, so each gets the size it needs.
		mon.pcmNode = mon.audioCtx.Call("createAnalyser")
		mon.pcmNode.Set("fftSize", pcmSize)
		mon.source.Call("connect", mon.pcmNode)
		mon.pcmJS = js.Global().Get("Float32Array").New(pcmSize)
		// A byte view onto the same ArrayBuffer. syscall/js can bulk-copy bytes
		// but has no float equivalent, and reading 2048 elements one Index() at
		// a time would cost more than the detection does — so the floats come
		// back as raw little-endian bytes and are reassembled in Go.
		mon.pcmBytesJS = js.Global().Get("Uint8Array").New(mon.pcmJS.Get("buffer"))
		mon.pcmBytes = make([]byte, pcmSize*4)
		mon.rate = mon.audioCtx.Get("sampleRate").Int()
		done(mon, nil)
	})
}

// fftSize is the analyser window. 1024 samples is ~23 ms at 44.1 kHz — short
// enough to track a voice's attack, long enough for usable low-end resolution.
const fftSize = 1024

// pcmSize is the window Samples reports, and it is sized by the lowest note the
// app must hear rather than by the display. Autocorrelation needs two full
// periods to find a period at all: at 44.1 kHz, 2048 samples is ~46 ms, which
// holds two periods of any pitch down to ~43 Hz and so covers the whole singing
// range with margin. Halving it would silently cost the bottom of a bass's
// range; doubling it would blur a moving voice into its own average.
const pcmSize = 2048

type webMonitor struct {
	stream, audioCtx, source, analyser js.Value
	freqJS, timeJS                     js.Value
	freq, time                         []byte
	bins                               int
	stopped                            bool

	pcmNode    js.Value // wider analyser backing Samples
	pcmJS      js.Value // Float32Array the browser writes into
	pcmBytesJS js.Value // Uint8Array view onto pcmJS's buffer
	pcmBytes   []byte   // Go-side landing buffer for that view
	rate       int
}

func (m *webMonitor) Level() float32 {
	if m.stopped {
		return 0
	}
	m.analyser.Call("getByteTimeDomainData", m.timeJS)
	js.CopyBytesToGo(m.time, m.timeJS)
	var peak float32
	for _, b := range m.time {
		// Time-domain bytes are centred on 128; distance from it is amplitude.
		d := float32(int(b)-128) / 127
		if d < 0 {
			d = -d
		}
		if d > peak {
			peak = d
		}
	}
	return peak
}

func (m *webMonitor) Bands(dst []float32) int {
	if m.stopped || len(dst) == 0 {
		return 0
	}
	m.analyser.Call("getByteFrequencyData", m.freqJS)
	js.CopyBytesToGo(m.freq, m.freqJS)
	return foldBands(m.freq, dst)
}

// foldBands converts the analyser's byte bins to the shared folding routine's
// scale and delegates to it.
//
// The grouping itself lives in internal/mic because every native shell needs
// the same one: shell.Monitor promises that asking for N bands means the same
// thing on every platform, and two independent implementations of a
// logarithmic fold would quietly drift apart.
func foldBands(bins []byte, dst []float32) int {
	if len(dst) == 0 || len(bins) == 0 {
		return 0
	}
	if cap(foldScratch) < len(bins) {
		foldScratch = make([]float32, len(bins))
	}
	f := foldScratch[:len(bins)]
	for i, v := range bins {
		f[i] = float32(v) / 255
	}
	return mic.FoldBands(f, dst)
}

// foldScratch is reused across calls; the web shell is single-goroutine (the
// browser has one JS thread), so no lock is needed.
var foldScratch []float32

// Samples copies the newest PCM window out of the wide analyser.
//
// getFloatTimeDomainData always writes the full window, so a dst shorter than
// pcmSize takes the *most recent* tail of it rather than a stale prefix: a
// caller that only needs 1024 samples should get the last 23 ms, not the 23 ms
// before that.
func (m *webMonitor) Samples(dst []float32) int {
	if m.stopped || len(dst) == 0 {
		return 0
	}
	m.pcmNode.Call("getFloatTimeDomainData", m.pcmJS)
	js.CopyBytesToGo(m.pcmBytes, m.pcmBytesJS)

	n := len(dst)
	if n > pcmSize {
		n = pcmSize
	}
	off := pcmSize - n // take the tail
	for i := 0; i < n; i++ {
		b := m.pcmBytes[(off+i)*4:]
		bits := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		dst[i] = math.Float32frombits(bits)
	}
	return n
}

func (m *webMonitor) WindowSize() int { return pcmSize }

// SampleRate is whatever rate the browser chose for the AudioContext — commonly
// 48000, not the 44100 a caller might assume — so pitch math must read it
// rather than hardcode a rate.
func (m *webMonitor) SampleRate() int { return m.rate }

func (m *webMonitor) Stop() {
	if m.stopped {
		return
	}
	m.stopped = true
	m.source.Call("disconnect")
	m.audioCtx.Call("close")
	stopTracks(m.stream)
}

// requestStream wraps getUserMedia, which is a promise, into the callback shape
// the shell interfaces use.
func requestStream(constraints map[string]any, done func(js.Value, error)) {
	md := mediaDevices()
	if md.IsUndefined() {
		done(js.Undefined(), errors.New("getUserMedia unavailable (an insecure context?)"))
		return
	}
	promise := md.Call("getUserMedia", constraints)
	go func() {
		stream, err := await(promise)
		done(stream, err)
	}()
}
