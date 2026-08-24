package sound

import (
	"math"
	"time"
)

// A sung note, as opposed to Tone's sine and Harmonics' fixed series.
//
// It is called Sung rather than Voice because Voice is already the mixer's
// handle to a playing sound, and one of those names had to give.

// Vowel selects the formant set a sung note is shaped by.
//
// Formants are the fixed resonances of the vocal tract, and they are what makes
// a vowel a vowel: "ah" and "ee" on the same pitch differ only in which
// harmonics the tract amplifies. A plain decaying harmonic series has no
// formants at all, which is why it reads as a buzzer however many partials it
// is given.
type Vowel int

const (
	// VowelAh is the open vowel singers practise on, and the default.
	VowelAh Vowel = iota
	VowelEe
	VowelOo
	VowelOh
)

// formant is one vocal-tract resonance: centre frequency, bandwidth, weight.
type formant struct{ freq, bw, gain float64 }

// The conventional averages for an adult voice. Not tuned to any individual —
// the aim is to sound like a person, not like one particular person.
var vowelFormants = map[Vowel][]formant{
	VowelAh: {{730, 90, 1.0}, {1090, 110, 0.50}, {2440, 170, 0.22}},
	VowelEe: {{270, 60, 1.0}, {2290, 100, 0.55}, {3010, 180, 0.30}},
	VowelOo: {{300, 60, 1.0}, {870, 90, 0.42}, {2240, 160, 0.12}},
	VowelOh: {{570, 80, 1.0}, {840, 100, 0.55}, {2410, 170, 0.15}},
}

// VoiceOptions shape a sung note. The zero value is a gentle "ah".
type VoiceOptions struct {
	Vowel Vowel
	// Vibrato is the depth of the pitch wobble in cents and Rate its speed in
	// Hz. A trained voice sits near ±40 cents at 5-6 Hz. The default is
	// shallower, because a note meant to be matched should offer the listener
	// one pitch to aim at rather than a range.
	Vibrato, Rate float64
	// Delay holds the note steady before the vibrato arrives. Real vibrato does
	// not begin with the note, and one that wobbles from the first instant is
	// harder to pitch against.
	Delay time.Duration
	// Tilt is the spectral rolloff in dB per octave above the fundamental.
	// Without it the upper harmonics buzz.
	Tilt float64
}

func (o VoiceOptions) withDefaults() VoiceOptions {
	if o.Vibrato == 0 {
		o.Vibrato = 20
	}
	if o.Rate == 0 {
		o.Rate = 5.2
	}
	if o.Delay == 0 {
		o.Delay = 400 * time.Millisecond
	}
	if o.Tilt == 0 {
		o.Tilt = 9
	}
	return o
}

// Sung returns a one-shot note shaped like a voice: a harmonic series filtered
// through vowel formants, with vibrato and a soft envelope.
//
// It exists because a synthesized tone is a poor thing to ask someone to
// imitate. Pitch matching is measurably more accurate against a human voice
// than against a piano or a pure tone, and the gap is widest for the least
// accurate singers — so a training app's reference note is worth the arithmetic.
//
// Every harmonic shares one phase, so vibrato moves the fundamental and all its
// partials together, which is what a real voice does and what keeps the wobble
// from sounding like a chorus effect.
func Sung(freq, seconds, amp float64, o VoiceOptions) Source {
	o = o.withDefaults()
	return &sung{
		freq:     freq,
		amp:      amp,
		amps:     vowelHarmonics(freq, o),
		total:    samples(seconds),
		attack:   samples(0.035),
		release:  samples(0.12),
		vibDepth: math.Exp2(o.Vibrato/1200) - 1,
		vibRate:  o.Rate,
		vibDelay: samples(o.Delay.Seconds()),
	}
}

// samples converts seconds to a sample count. It is a function because Go will
// not convert an untyped constant with a fractional part straight to int.
func samples(s float64) int { return int(math.Round(s * float64(SampleRate))) }

// vowelHarmonics evaluates the formant resonances at each harmonic.
func vowelHarmonics(f0 float64, o VoiceOptions) []float64 {
	set := vowelFormants[o.Vowel]
	var amps []float64
	for h := 1; h <= 64; h++ {
		f := f0 * float64(h)
		// Above Nyquist a partial folds back down as an inharmonic tone, which
		// is precisely the artefact that makes synthesis sound synthetic.
		if f >= SampleRate/2 {
			break
		}
		var a float64
		for _, fm := range set {
			d := (f - fm.freq) / fm.bw
			a += fm.gain / (1 + d*d)
		}
		amps = append(amps, a*math.Pow(10, -o.Tilt*math.Log2(float64(h))/20))
	}
	// Normalize against the sum so the peak sample lands near amp whatever the
	// formants happen to favour at this pitch.
	var sum float64
	for _, a := range amps {
		sum += a
	}
	if sum > 0 {
		for i := range amps {
			amps[i] /= sum
		}
	}
	return amps
}

type sung struct {
	freq, amp         float64
	amps              []float64
	phase             float64
	pos, total        int
	attack, release   int
	vibDepth, vibRate float64
	vibDelay          int
}

func (v *sung) gain(pos int) float64 {
	// Raised cosine rather than a linear ramp: a straight ramp has a corner in
	// it that is audible as a click at these lengths.
	if pos < v.attack && v.attack > 0 {
		return 0.5 - 0.5*math.Cos(math.Pi*float64(pos)/float64(v.attack))
	}
	if rem := v.total - pos; rem < v.release && v.release > 0 {
		return 0.5 - 0.5*math.Cos(math.Pi*float64(rem)/float64(v.release))
	}
	return 1
}

func (v *sung) Process(out []float32) bool {
	if v.pos >= v.total {
		return false
	}
	for i := range out {
		if v.pos >= v.total {
			for j := i; j < len(out); j++ {
				out[j] = 0
			}
			break
		}

		// Vibrato fades in rather than switching on.
		var vib float64
		if v.pos > v.vibDelay {
			ramp := float64(v.pos-v.vibDelay) / (0.4 * SampleRate)
			if ramp > 1 {
				ramp = 1
			}
			vib = v.vibDepth * ramp * math.Sin(2*math.Pi*v.vibRate*float64(v.pos)/SampleRate)
		}

		var s float64
		for h, a := range v.amps {
			s += a * math.Sin(2*math.Pi*float64(h+1)*v.phase)
		}
		out[i] = float32(s * v.amp * v.gain(v.pos))

		v.phase += v.freq * (1 + vib) / SampleRate
		if v.phase > 1 {
			v.phase -= math.Floor(v.phase)
		}
		v.pos++
	}
	return v.pos < v.total
}
