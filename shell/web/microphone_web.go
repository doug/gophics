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
	"encoding/binary"
	"errors"
	"math"
	"syscall/js"
	"time"

	"github.com/doug/gophics/internal/dsp"
	"github.com/doug/gophics/internal/wav"
	"github.com/doug/gophics/shell"
)

// The window opts into the preview and the microphone by implementing their
// Window interfaces;
// this is the compile-time check that it still does.
// Verifying this path needs a browser, and the compiler will not help.
//
// syscall/js calls are opaque to it: moving a getUserMedia call — as splitting
// the media capabilities by device did — compiles cleanly for js/wasm, passes
// every Go test, and can still be broken. Headless Chrome supplies a synthetic
// camera and microphone for exactly this:
//
//	go run ./cmd/gophics build -p web ./examples/mirror
//	(cd build/web && python3 -m http.server 8731) &
//	"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" \
//	  --headless=new --user-data-dir=/tmp/p --remote-debugging-port=9222 \
//	  --use-fake-ui-for-media-stream --use-fake-device-for-media-stream \
//	  http://localhost:8731/
//
// Wrap navigator.mediaDevices.getUserMedia in the page before the wasm loads,
// record each request and its outcome into document.title, and read that back
// from http://127.0.0.1:9222/json — which needs no WebSocket client. Use real
// time, not --virtual-time-budget: the media promises never resolve under it.
//
// Two results are worth knowing in advance. An app that waits for a click will
// need a synthetic PointerEvent dispatched at its start control. And sampling
// the canvas reads back empty while the GPU path is in use, which is a limit
// of the sampling and not a black frame — run with Renderer: RendererCPU to
// see the pixels. Checked this way after the split: video and audio each
// yielded a track, and the canvas came back 100% non-black.

var _ shell.CameraPreviewWindow = (*window)(nil)
var _ shell.MicrophoneWindow = (*window)(nil)

func (w *window) Microphone() shell.Microphone {
	if md := mediaDevices(); md.IsUndefined() || audioContextCtor().IsUndefined() {
		return nil
	}
	return &webMicrophone{}
}

// --- Microphone --------------------------------------------------------------

type webMicrophone struct{}

func (m *webMicrophone) Authorize(cb func(shell.Permission)) {
	if cb == nil {
		return
	}
	requestStream(map[string]any{"audio": true}, func(stream js.Value, err error) {
		if err != nil {
			cb(shell.PermissionDenied)
			return
		}
		stopTracks(stream)
		cb(shell.PermissionGranted)
	})
}

func (m *webMicrophone) Record(_ shell.RecordOptions, done func(shell.Recorder, error)) {
	if done == nil {
		return // result-only: recording without the Recorder handle leaks a live mic
	}
	promise := mediaDevices().Call("getUserMedia", map[string]any{"audio": true})
	go func() {
		stream, err := await(promise)
		if err != nil {
			done(nil, err)
			return
		}
		r := newWebRecorder(stream)
		done(r, nil)
	}()
}

func (m *webMicrophone) Listen(done func(shell.Monitor, error)) {
	if done == nil {
		return // result-only: the Monitor handle is the whole point
	}
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
// The grouping itself lives in internal/dsp because every native shell needs
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
	return dsp.FoldBands(f, dst)
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

// webRecorder captures raw PCM from the mic via a ScriptProcessorNode and
// encodes it to a portable WAV clip on stop — so a web recording plays back
// unchanged on desktop/mobile once those shells land.
type webRecorder struct {
	stream, audioCtx, source, proc js.Value
	onProc                         js.Func
	sampleRate                     int
	samples                        []int16
	level                          float32
	envelope                       []float32
	start                          time.Time
	stopped                        bool
}

func newWebRecorder(stream js.Value) *webRecorder {
	r := &webRecorder{stream: stream, start: time.Now()}
	r.audioCtx = audioContextCtor().New()
	r.sampleRate = r.audioCtx.Get("sampleRate").Int()
	r.source = r.audioCtx.Call("createMediaStreamSource", stream)
	r.proc = r.audioCtx.Call("createScriptProcessor", 4096, 1, 1)
	r.onProc = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if r.stopped || len(args) == 0 {
			return nil
		}
		in := args[0].Get("inputBuffer").Call("getChannelData", 0) // Float32Array
		n := in.Length()
		raw := make([]byte, n*4)
		js.CopyBytesToGo(raw, js.Global().Get("Uint8Array").New(in.Get("buffer"), in.Get("byteOffset"), n*4))
		var peak float32
		for i := 0; i < n; i++ {
			f := math.Float32frombits(binary.LittleEndian.Uint32(raw[i*4:]))
			if f > 1 {
				f = 1
			} else if f < -1 {
				f = -1
			}
			r.samples = append(r.samples, int16(f*32767))
			if af := f; af < 0 {
				if -af > peak {
					peak = -af
				}
			} else if af > peak {
				peak = af
			}
		}
		r.level = peak
		r.envelope = append(r.envelope, peak) // one bucket per audio block
		return nil
	})
	r.proc.Set("onaudioprocess", r.onProc)
	r.source.Call("connect", r.proc)
	r.proc.Call("connect", r.audioCtx.Get("destination")) // required to run; emits silence
	return r
}

func (r *webRecorder) Level() float32 {
	if r.stopped {
		return 0
	}
	return r.level
}

func (r *webRecorder) Elapsed() time.Duration { return time.Since(r.start) }

// Stop is a side effect first: the recording ends and the mic is released
// whether or not anyone wants the clip, so a nil done skips only the report.
func (r *webRecorder) Stop(done func(shell.Clip, error)) {
	report := func(c shell.Clip, err error) {
		if done != nil {
			done(c, err)
		}
	}
	if r.stopped {
		report(shell.Clip{}, errors.New("already stopped"))
		return
	}
	samples, rate, envelope := r.samples, r.sampleRate, r.envelope
	r.teardown()
	var dur time.Duration
	if rate > 0 {
		dur = time.Duration(len(samples)) * time.Second / time.Duration(rate)
	}
	report(shell.Clip{
		Data:     wav.Encode(samples, rate),
		Mime:     "audio/wav",
		Duration: dur,
		Envelope: envelope,
	}, nil)
}

func (r *webRecorder) Cancel() { r.teardown() }

func (r *webRecorder) teardown() {
	if r.stopped {
		return
	}
	r.stopped = true
	r.proc.Call("disconnect")
	r.source.Call("disconnect")
	r.audioCtx.Call("close")
	stopTracks(r.stream)
	r.onProc.Release()
}

// webPlayback drives one decoded clip through a BufferSource. Web Audio has no
// seek, so seeking restarts a fresh source at an offset.
type webPlayback struct {
	ctx, buffer, source js.Value
	duration            time.Duration
	offset              float64 // seconds into the clip the current source started at
	startedAt           float64 // ctx.currentTime when the source started
	playing             bool
	live                bool // the current source has been started and not yet stopped/ended
	onEnded             js.Func
}

func (p *webPlayback) startFrom(offset float64) {
	if offset < 0 {
		offset = 0
	}
	p.ctx.Call("resume") // AudioContext may start suspended under autoplay policy
	p.source = p.ctx.Call("createBufferSource")
	p.source.Set("buffer", p.buffer)
	p.source.Call("connect", p.ctx.Get("destination"))
	if !p.onEnded.IsUndefined() {
		p.onEnded.Release()
	}
	p.onEnded = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		p.live = false
		// Natural end (not a seek/stop, which clear playing first).
		if p.playing {
			p.playing = false
			p.offset = p.duration.Seconds()
		}
		return nil
	})
	p.source.Call("addEventListener", "ended", p.onEnded)
	p.offset = offset
	p.startedAt = p.ctx.Get("currentTime").Float()
	p.source.Call("start", 0, offset)
	p.playing, p.live = true, true
}

func (p *webPlayback) Position() time.Duration {
	pos := p.offset
	if p.playing {
		pos += p.ctx.Get("currentTime").Float() - p.startedAt
	}
	if d := p.duration.Seconds(); pos > d {
		pos = d
	}
	return time.Duration(pos * float64(time.Second))
}

func (p *webPlayback) Duration() time.Duration { return p.duration }
func (p *webPlayback) Playing() bool           { return p.playing }

func (p *webPlayback) Seek(t time.Duration) {
	wasPlaying := p.playing
	p.stopSource()
	if wasPlaying {
		p.startFrom(t.Seconds())
	} else {
		p.offset = t.Seconds()
	}
}

func (p *webPlayback) Stop() {
	p.stopSource()
}

func (p *webPlayback) stopSource() {
	if p.playing {
		p.offset += p.ctx.Get("currentTime").Float() - p.startedAt
		p.playing = false
	}
	// Only stop a source that's actually live — calling stop() on one that has
	// already ended (or was never started) throws.
	if p.live && !p.source.IsUndefined() {
		p.source.Call("stop")
		p.live = false
	}
}
