// Package sound is a pure-Go DSP mixer for game and UI audio: PCM samples,
// oscillators, gain, and a mixer that a platform sink pulls from. It has no
// device dependency, so mixing is deterministic and unit-testable with no audio
// hardware; a driver (shell side) pulls interleaved frames via ReadFloat32s.
//
// The DSP core (Source/Osc/Tone/Gain/Mixer) is adapted from the author's
// github.com/doug/gophics/audio; sample playback and the pull adapter are added
// here for game audio.
package sound

import (
	"math"
	"sync"
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

// Gain scales another source by Level.
type Gain struct {
	Src   Source
	Level float64
}

func (g *Gain) Process(out []float32) bool {
	cont := g.Src.Process(out)
	for i := range out {
		out[i] *= float32(g.Level)
	}
	return cont
}

// Mixer sums active sources (mono, clamped to [-1,1]) and exposes the result as
// interleaved frames to a platform driver via ReadFloat32s. Add is safe to call
// from the UI goroutine while the driver pulls on the audio goroutine.
type Mixer struct {
	mu       sync.Mutex
	sources  []Source
	scratch  []float32
	mono     []float32
	channels int
	master   float64
}

// NewMixer returns an empty mixer producing `channels` interleaved output
// channels (0 → 2/stereo).
func NewMixer(channels int) *Mixer {
	if channels < 1 {
		channels = 2
	}
	return &Mixer{channels: channels, master: 1}
}

// Add starts playing src.
func (m *Mixer) Add(src Source) {
	if src == nil {
		return
	}
	m.mu.Lock()
	m.sources = append(m.sources, src)
	m.mu.Unlock()
}

// SetMasterVolume scales the whole mix (0..1+).
func (m *Mixer) SetMasterVolume(v float64) {
	m.mu.Lock()
	m.master = v
	m.mu.Unlock()
}

// Len reports the number of active sources.
func (m *Mixer) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.sources)
}

// Process sums active sources into mono `out`, dropping finished ones. Always
// returns true (a mixer never finishes).
func (m *Mixer) Process(out []float32) bool {
	for i := range out {
		out[i] = 0
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if cap(m.scratch) < len(out) {
		m.scratch = make([]float32, len(out))
	}
	sc := m.scratch[:len(out)]
	alive := m.sources[:0]
	for _, s := range m.sources {
		for i := range sc {
			sc[i] = 0
		}
		if s.Process(sc) {
			alive = append(alive, s)
		}
		for i := range out {
			out[i] += sc[i]
		}
	}
	m.sources = alive
	master := float32(m.master)
	for i := range out {
		v := out[i] * master
		if v > 1 {
			v = 1
		} else if v < -1 {
			v = -1
		}
		out[i] = v
	}
	return true
}

// ReadFloat32s fills buf with interleaved frames (mono duplicated across
// channels) — the audio.ReadFloat32er contract the platform driver pulls from.
func (m *Mixer) ReadFloat32s(buf []float32) (int, error) {
	ch := m.channels
	frames := len(buf) / ch
	if cap(m.mono) < frames {
		m.mono = make([]float32, frames)
	}
	mono := m.mono[:frames]
	m.Process(mono)
	for i := 0; i < frames; i++ {
		for c := 0; c < ch; c++ {
			buf[i*ch+c] = mono[i]
		}
	}
	return frames * ch, nil
}

// Render pulls n mono samples from src (for tests and offline rendering).
func Render(src Source, n int) []float32 {
	out := make([]float32, n)
	src.Process(out)
	return out
}
