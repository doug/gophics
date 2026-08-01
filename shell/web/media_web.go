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
	"encoding/binary"
	"errors"
	"image"
	_ "image/jpeg" // register decoders for captured photos
	_ "image/png"
	"math"
	"syscall/js"
	"time"

	"github.com/doug/gossamer/shell"
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

// Audio returns the audio capability, or nil when the browser lacks
// getUserMedia or Web Audio (e.g. an insecure context).
func (w *window) Audio() shell.Audio {
	md := js.Global().Get("navigator").Get("mediaDevices")
	if md.IsUndefined() || md.IsNull() || audioContextCtor().IsUndefined() {
		return nil
	}
	if w.aud == nil {
		w.aud = &webAudio{mediaDevices: md}
	}
	return w.aud
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

type webAudio struct{ mediaDevices js.Value }

func (a *webAudio) Authorize(cb func(shell.Permission)) {
	promise := a.mediaDevices.Call("getUserMedia", map[string]any{"audio": true})
	go func() {
		stream, err := await(promise)
		if err != nil {
			cb(shell.PermissionDenied)
			return
		}
		stopTracks(stream)
		cb(shell.PermissionGranted)
	}()
}

func (a *webAudio) Record(_ shell.RecordOptions, done func(shell.Recorder, error)) {
	promise := a.mediaDevices.Call("getUserMedia", map[string]any{"audio": true})
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

func (a *webAudio) Play(clip shell.Clip, done func(shell.Playback, error)) {
	ctx := audioContextCtor().New()
	u8 := bytesToJS(clip.Data)
	go func() {
		buf, err := await(ctx.Call("decodeAudioData", u8.Get("buffer")))
		if err != nil {
			ctx.Call("close")
			done(nil, err)
			return
		}
		p := &webPlayback{ctx: ctx, buffer: buf,
			duration: time.Duration(buf.Get("duration").Float() * float64(time.Second))}
		p.startFrom(0)
		done(p, nil)
	}()
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

func (r *webRecorder) Stop(done func(shell.Clip, error)) {
	if r.stopped {
		done(shell.Clip{}, errors.New("already stopped"))
		return
	}
	samples, rate, envelope := r.samples, r.sampleRate, r.envelope
	r.teardown()
	var dur time.Duration
	if rate > 0 {
		dur = time.Duration(len(samples)) * time.Second / time.Duration(rate)
	}
	done(shell.Clip{
		Data:     shell.EncodeWAV(samples, rate),
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

// --- helpers -----------------------------------------------------------------

func stopTracks(stream js.Value) {
	tracks := stream.Call("getTracks")
	for i := 0; i < tracks.Length(); i++ {
		tracks.Index(i).Call("stop")
	}
}

func jsToBytes(u8 js.Value) []byte {
	b := make([]byte, u8.Length())
	js.CopyBytesToGo(b, u8)
	return b
}

func bytesToJS(b []byte) js.Value {
	u8 := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(u8, b)
	return u8
}

// await blocks the calling goroutine until the JS promise settles. Safe on
// wasm's single thread; must be called off the event-loop goroutine.
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	var then, catch js.Func
	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- result{v: v}
		return nil
	})
	catch = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "promise rejected"
		if len(args) > 0 && args[0].Truthy() {
			msg = args[0].Call("toString").String()
		}
		ch <- result{err: errors.New(msg)}
		return nil
	})
	p.Call("then", then).Call("catch", catch)
	r := <-ch
	then.Release()
	catch.Release()
	return r.v, r.err
}
