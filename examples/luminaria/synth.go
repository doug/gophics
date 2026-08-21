package main

import (
	"math"
	"math/rand"

	"github.com/doug/gophics/sound"
)

// Every voice here is synthesized in Go at the mixer's rate — no audio assets,
// no platform synth. Each is a sound.Source the mixer pulls from, so a note is
// a few hundred bytes of state rather than a decoded buffer, and the whole file
// is deterministic (seeded noise only) and therefore unit-testable with no
// audio hardware.

const sr = float64(sound.SampleRate)

// Voice selects the timbre a lit node plays.
type Voice int

const (
	VoicePluck Voice = iota // Karplus-Strong string
	VoiceBell               // two-operator FM with an inharmonic ratio
	VoiceGlass              // detuned triangle pad, slow attack
)

var voiceNames = []string{"Pluck", "Bell", "Glass"}

// Note builds a one-shot voice at freq Hz that decays over roughly sustain
// seconds. seed makes the noise-excited voices reproducible.
func Note(v Voice, freq, sustain float64, seed int64) sound.Source {
	if sustain < 0.05 {
		sustain = 0.05
	}
	switch v {
	case VoiceBell:
		return newBell(freq, sustain)
	case VoiceGlass:
		return newGlass(freq, sustain)
	default:
		return newPluck(freq, sustain, seed)
	}
}

// --- Pluck -------------------------------------------------------------------

// pluck is Karplus-Strong: a delay line one period long, filled with noise and
// then repeatedly averaged with its neighbour. The averaging is a one-pole
// lowpass, so the high partials in the noise die first and what is left settles
// into a pitched, slowly darkening string.
type pluck struct {
	buf  []float32
	idx  int
	gain float32 // per-period loop gain, tuned to reach silence at `sustain`
	rem  int     // samples left before the voice retires
}

func newPluck(freq, sustain float64, seed int64) *pluck {
	n := int(math.Round(sr / freq))
	if n < 2 {
		n = 2
	}
	rng := rand.New(rand.NewSource(seed))
	buf := make([]float32, n)
	for i := range buf {
		buf[i] = float32(rng.Float64()*2 - 1)
	}
	// Fade the excitation into the loop point so the first wrap doesn't click.
	for i := 0; i < n/8; i++ {
		buf[n-1-i] *= float32(i) / float32(n/8)
	}
	// Each slot in the delay line is rewritten once per trip around the loop,
	// not once per sample — so the gain applied there is the *per-period* one.
	// The loop runs freq times a second: solve g^(freq*sustain) = 1e-3.
	return &pluck{
		buf:  buf,
		gain: float32(math.Exp(math.Log(1e-3) / (freq * sustain))),
		rem:  int(sustain * sr),
	}
}

func (p *pluck) Process(out []float32) bool {
	for i := range out {
		if p.rem <= 0 {
			for j := i; j < len(out); j++ {
				out[j] = 0
			}
			return false
		}
		cur := p.buf[p.idx]
		next := p.buf[(p.idx+1)%len(p.buf)]
		p.buf[p.idx] = (cur + next) * 0.5 * p.gain
		out[i] = cur * 0.6
		p.idx = (p.idx + 1) % len(p.buf)
		p.rem--
	}
	return true
}

// --- Bell --------------------------------------------------------------------

// bell is a two-operator FM voice. The modulator sits at an inharmonic ratio
// (1.41 — near √2, so its partials never line up with the carrier's), which is
// what makes a struck-metal sound rather than a brass one. The modulation index
// decays faster than the amplitude, so the strike is bright and the tail is
// nearly a sine.
type bell struct {
	pc, pm     float64 // carrier and modulator phase
	fc, fm     float64
	pos, total int
	tau        float64 // amplitude decay constant, in samples
}

func newBell(freq, sustain float64) *bell {
	return &bell{
		fc:    freq,
		fm:    freq * 1.41,
		total: int(sustain * sr),
		tau:   sustain * sr / 5, // ~e⁻⁵ by the end
	}
}

func (b *bell) Process(out []float32) bool {
	incC, incM := b.fc/sr, b.fm/sr
	for i := range out {
		if b.pos >= b.total {
			for j := i; j < len(out); j++ {
				out[j] = 0
			}
			return false
		}
		t := float64(b.pos) / b.tau
		env := math.Exp(-t)
		if b.pos < 200 { // 4.5 ms attack, enough to kill the click
			env *= float64(b.pos) / 200
		}
		index := 3.2 * math.Exp(-t*2.5) // brightness dies before loudness
		out[i] = float32(math.Sin(2*math.Pi*b.pc+index*math.Sin(2*math.Pi*b.pm)) * env * 0.42)
		b.pc += incC
		b.pm += incM
		b.pos++
	}
	return true
}

// --- Glass -------------------------------------------------------------------

// glass is a pad: three triangle oscillators — the root, a slightly sharp
// detune, and the fifth above — under a slow attack and a long release. The
// detune beats against the root at a couple of hertz, which is what stops a
// sustained chord from sounding like a synthesizer test tone.
type glass struct {
	p          [3]float64
	f          [3]float64
	pos, total int
	attack     int
}

func newGlass(freq, sustain float64) *glass {
	total := int(sustain * 1.6 * sr) // pads outlive their nominal sustain
	return &glass{
		f:      [3]float64{freq, freq * 1.004, freq * 1.5},
		total:  total,
		attack: int(float64(total) * 0.18),
	}
}

func (g *glass) Process(out []float32) bool {
	amp := [3]float64{0.5, 0.4, 0.22}
	for i := range out {
		if g.pos >= g.total {
			for j := i; j < len(out); j++ {
				out[j] = 0
			}
			return false
		}
		var env float64
		if g.pos < g.attack {
			env = float64(g.pos) / float64(g.attack)
		} else {
			rem := float64(g.total-g.pos) / float64(g.total-g.attack)
			env = rem * rem
		}
		var s float64
		for k := range g.f {
			s += triangle(g.p[k]) * amp[k]
			g.p[k] += g.f[k] / sr
			if g.p[k] >= 1 {
				g.p[k] -= 1
			}
		}
		out[i] = float32(s * env * 0.34)
		g.pos++
	}
	return true
}

func triangle(phase float64) float64 { return 4*math.Abs(phase-0.5) - 1 }

// --- Scales ------------------------------------------------------------------

// Scale is a set of semitone offsets from the root, repeating every octave.
// Every scale here is gapped or symmetric — no minor seconds against the root —
// so the crawlers cannot land on a combination that sounds like a mistake. That
// is the whole trick behind this kind of toy: constrain the pitch set and any
// pattern the user draws is consonant.
type Scale struct {
	Name    string
	Degrees []int
}

var scales = []Scale{
	{"Pentatonic", []int{0, 2, 4, 7, 9}},
	{"Minor pent.", []int{0, 3, 5, 7, 10}},
	{"Insen", []int{0, 1, 5, 7, 10}}, // the Japanese scale Iwai's toys lean on
	{"Dorian", []int{0, 2, 3, 5, 7, 9, 10}},
	{"Whole tone", []int{0, 2, 4, 6, 8, 10}},
}

func scaleNames() []string {
	out := make([]string, len(scales))
	for i, s := range scales {
		out[i] = s.Name
	}
	return out
}

// Freq maps a scale step (0 = the root) to hertz, walking up octaves as the
// step runs past the end of the scale. root is a MIDI note number.
func (s Scale) Freq(step, root int) float64 {
	n := len(s.Degrees)
	oct, deg := step/n, step%n
	if deg < 0 {
		deg += n
		oct--
	}
	semis := root + 12*oct + s.Degrees[deg]
	return 440 * math.Pow(2, float64(semis-69)/12)
}
