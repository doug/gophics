// Package sound is a pure-Go DSP mixer for game and UI audio: PCM samples,
// oscillators, gain, and a mixer that a platform sink pulls from. It has no
// device dependency, so mixing is deterministic and unit-testable with no audio
// hardware; a driver (shell side) pulls interleaved frames via ReadFloat32s.
//
// The DSP core (Source/Osc/Tone/Mixer) is adapted from the author's
// github.com/doug/gophics/audio; sample playback and the pull adapter are added
// here for game audio.
package sound

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// SampleRate is the mixing rate (Hz).
const SampleRate = 44100

// Source produces mono audio in [-1,1]. Process fills out with the next
// len(out) samples and reports whether the source is still producing (false →
// finished; the mixer drops it).
type Source interface {
	Process(out []float32) bool
}

// Wave selects an oscillator waveform.
type Wave int

const (
	Sine Wave = iota
	Square
	Saw
	Triangle
)

func waveform(w Wave, phase float64) float64 {
	switch w {
	case Square:
		if phase < 0.5 {
			return 1
		}
		return -1
	case Saw:
		return 2*phase - 1
	case Triangle:
		return 4*math.Abs(phase-0.5) - 1
	default:
		return math.Sin(2 * math.Pi * phase)
	}
}

// Osc is a continuous oscillator (Process always returns true).
type Osc struct {
	Wave  Wave
	Freq  float64 // Hz
	Amp   float64 // 0..1
	phase float64
}

func (o *Osc) Process(out []float32) bool {
	inc := o.Freq / SampleRate
	for i := range out {
		out[i] = float32(o.Amp * waveform(o.Wave, o.phase))
		o.phase += inc
		if o.phase >= 1 {
			o.phase -= 1
		}
	}
	return true
}

// tone is a fixed-duration sine with a short attack/release envelope.
type tone struct {
	osc           Osc
	pos, total    int
	attack, decay int
}

// Tone returns a one-shot sine note (frequency, seconds, amplitude) with a small
// attack/release so it doesn't click.
func Tone(freq, seconds, amp float64) Source {
	return &tone{
		osc:    Osc{Wave: Sine, Freq: freq, Amp: amp},
		total:  int(seconds * SampleRate),
		attack: int(math.Round(0.008 * SampleRate)),
		decay:  int(math.Round(0.06 * SampleRate)),
	}
}

func (t *tone) gain(pos int) float64 {
	if pos < t.attack && t.attack > 0 {
		return float64(pos) / float64(t.attack)
	}
	if rem := t.total - pos; rem < t.decay && t.decay > 0 {
		return float64(rem) / float64(t.decay)
	}
	return 1
}

func (t *tone) Process(out []float32) bool {
	if t.pos >= t.total {
		return false
	}
	t.osc.Process(out)
	for i := range out {
		if t.pos >= t.total {
			for j := i; j < len(out); j++ {
				out[j] = 0
			}
			break
		}
		out[i] *= float32(t.gain(t.pos))
		t.pos++
	}
	return t.pos < t.total
}

// PlayOptions configure a voice. Zero values mean natural (Volume 1, Pan center,
// Pitch 1).
type PlayOptions struct {
	Volume float64       // linear gain; 0 → 1
	Pan    float64       // -1 left … 0 center … +1 right
	Pitch  float64       // playback rate; 0 → 1 (samples only)
	Loop   bool          // loop samples
	FadeIn time.Duration // ramp the envelope 0→1 over this time
}

// Voice is a playing sound — a mono Source placed in the stereo field with a
// live-adjustable volume and pan, plus a fade envelope. It is the handle
// returned by Play.
type Voice struct {
	src     Source
	vol     atomic.Uint32 // Float32bits
	pan     atomic.Uint32 // Float32bits, -1..1
	stopped atomic.Bool

	env      float32       // envelope gain (advanced on the audio goroutine)
	envTgt   atomic.Uint32 // Float32bits target
	envRate  atomic.Uint32 // Float32bits per-sample ramp; 0 = instant
	fadeStop atomic.Bool   // drop when the envelope reaches 0
}

func newVoice(src Source, opts PlayOptions) *Voice {
	v := &Voice{src: src, env: 1}
	vol := opts.Volume
	if vol <= 0 {
		vol = 1
	}
	v.SetVolume(vol)
	v.SetPan(opts.Pan)
	v.setEnvTgt(1)
	if opts.FadeIn > 0 {
		v.env = 0
		v.setEnvRate(ramp(opts.FadeIn))
	}
	return v
}

// FadeOut ramps the voice to silence over d, then drops it.
func (v *Voice) FadeOut(d time.Duration) {
	if v == nil {
		return
	}
	v.setEnvTgt(0)
	v.setEnvRate(ramp(d))
	v.fadeStop.Store(true)
}

// ramp is the per-sample envelope step for a fade of duration d.
func ramp(d time.Duration) float32 {
	frames := d.Seconds() * SampleRate
	if frames < 1 {
		frames = 1
	}
	return float32(1 / frames)
}

func (v *Voice) setEnvTgt(x float32)  { v.envTgt.Store(math.Float32bits(x)) }
func (v *Voice) setEnvRate(x float32) { v.envRate.Store(math.Float32bits(x)) }
func (v *Voice) envTarget() float32   { return math.Float32frombits(v.envTgt.Load()) }
func (v *Voice) envStep() float32     { return math.Float32frombits(v.envRate.Load()) }

// SetVolume sets the linear gain (live). Nil-safe: Play returns a nil *Voice
// for an empty sample, and every Voice method tolerates that, so callers never
// have to nil-check the handle.
func (v *Voice) SetVolume(x float64) {
	if v == nil {
		return
	}
	v.vol.Store(math.Float32bits(float32(x)))
}

// SetPan sets the stereo pan in [-1,1] (live). Nil-safe; see SetVolume.
func (v *Voice) SetPan(x float64) {
	if v == nil {
		return
	}
	if x < -1 {
		x = -1
	} else if x > 1 {
		x = 1
	}
	v.pan.Store(math.Float32bits(float32(x)))
}

// Stop ends the voice; the mixer drops it on the next block.
func (v *Voice) Stop() {
	if v != nil {
		v.stopped.Store(true)
	}
}

func (v *Voice) volume() float32 { return math.Float32frombits(v.vol.Load()) }
func (v *Voice) panpos() float32 { return math.Float32frombits(v.pan.Load()) }

// Mixer sums active voices into interleaved stereo, applying each voice's volume
// and constant-power pan. It implements the audio.ReadFloat32er contract the
// platform driver pulls from; Play is safe to call while the driver reads.
type Mixer struct {
	mu     sync.Mutex
	voices []*Voice
	mono   []float32
	master float64
}

// NewMixer returns an empty stereo mixer.
func NewMixer() *Mixer { return &Mixer{master: 1} }

func (m *Mixer) add(v *Voice) *Voice {
	if v == nil {
		return nil
	}
	m.mu.Lock()
	m.voices = append(m.voices, v)
	m.mu.Unlock()
	return v
}

// PlaySource starts an arbitrary Source as a voice.
func (m *Mixer) PlaySource(src Source, opts PlayOptions) *Voice {
	if src == nil {
		return nil
	}
	return m.add(newVoice(src, opts))
}

// SetMasterVolume scales the whole mix.
func (m *Mixer) SetMasterVolume(v float64) {
	m.mu.Lock()
	m.master = v
	m.mu.Unlock()
}

// Len reports the number of active voices.
func (m *Mixer) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.voices)
}

// ReadFloat32s fills buf with interleaved stereo frames (the audio.ReadFloat32er
// contract), summing voices with per-voice volume + constant-power pan, dropping
// finished or stopped ones, and clamping to [-1,1].
func (m *Mixer) ReadFloat32s(buf []float32) (int, error) {
	for i := range buf {
		buf[i] = 0
	}
	frames := len(buf) / 2
	m.mu.Lock()
	defer m.mu.Unlock()
	if cap(m.mono) < frames {
		m.mono = make([]float32, frames)
	}
	mono := m.mono[:frames]
	master := float32(m.master)
	alive := m.voices[:0]
	for _, v := range m.voices {
		if v.stopped.Load() {
			continue // dropped
		}
		for i := range mono {
			mono[i] = 0
		}
		cont := v.src.Process(mono)
		a := (float64(v.panpos()) + 1) * 0.5 * (math.Pi / 2)
		l := float32(math.Cos(a)) * v.volume() * master
		r := float32(math.Sin(a)) * v.volume() * master

		// Advance the fade envelope across the block, lerping per sample.
		env0 := v.env
		tgt, step := v.envTarget(), v.envStep()
		if step <= 0 {
			v.env = tgt
		} else if v.env < tgt {
			v.env = min(tgt, v.env+step*float32(frames))
		} else {
			v.env = max(tgt, v.env-step*float32(frames))
		}
		env0, env1 := env0, v.env

		for i := 0; i < frames; i++ {
			e := env0
			if frames > 1 {
				e = env0 + (env1-env0)*float32(i)/float32(frames)
			}
			buf[2*i] += mono[i] * l * e
			buf[2*i+1] += mono[i] * r * e
		}
		if cont && !(v.fadeStop.Load() && v.env <= 0.0002) {
			alive = append(alive, v)
		}
	}
	m.voices = alive
	for i := range buf {
		if buf[i] > 1 {
			buf[i] = 1
		} else if buf[i] < -1 {
			buf[i] = -1
		}
	}
	return len(buf), nil
}
