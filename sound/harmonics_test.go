package sound_test

import (
	"math"
	"testing"

	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/sound/pitch"
)

// render pulls a source dry, in blocks, as the mixer would.
func render(src sound.Source, n int) []float32 {
	out := make([]float32, 0, n)
	block := make([]float32, 256)
	for len(out) < n {
		if !src.Process(block) {
			out = append(out, block...)
			break
		}
		out = append(out, block...)
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}

var vowel = []float64{1, 0.6, 0.35, 0.2, 0.1}

// TestHarmonicsHasTheRightPitch is the property that matters: a richer timbre
// must not move the note. A singer asked to match it is matching the
// fundamental.
func TestHarmonicsHasTheRightPitch(t *testing.T) {
	d := &pitch.Detector{SampleRate: sound.SampleRate}
	for _, name := range []string{"C3", "G3", "A4", "C5"} {
		n := pitch.MustParse(name)
		buf := render(sound.Harmonics(n.Freq(), 1, 0.8, vowel), 8192)
		// Skip the attack ramp.
		got := d.Detect(buf[2048:6144])
		if !got.Voiced {
			t.Errorf("%s: not voiced", name)
			continue
		}
		if cents := pitch.Cents(n.Freq(), got.Freq); math.Abs(cents) > 5 {
			t.Errorf("%s: %.1f cents off (got %.1f Hz)", name, cents, got.Freq)
		}
	}
}

// TestHarmonicsIsNotASine checks the timbre is actually richer — the whole
// reason to use it over Tone.
func TestHarmonicsIsNotASine(t *testing.T) {
	const freq = 220
	sine := render(sound.Tone(freq, 1, 0.8), 8192)[2048:6144]
	rich := render(sound.Harmonics(freq, 1, 0.8, vowel), 8192)[2048:6144]

	// Crest factor (peak/RMS) separates them: a sine's is sqrt(2) ~ 1.414, and
	// summed harmonics push it higher.
	crest := func(x []float32) float64 {
		var peak, sum float64
		for _, v := range x {
			a := math.Abs(float64(v))
			if a > peak {
				peak = a
			}
			sum += float64(v) * float64(v)
		}
		return peak / math.Sqrt(sum/float64(len(x)))
	}
	cs, cr := crest(sine), crest(rich)
	if math.Abs(cs-math.Sqrt2) > 0.05 {
		t.Errorf("sine crest factor %.3f, want ~1.414", cs)
	}
	if cr <= cs+0.1 {
		t.Errorf("harmonic crest factor %.3f is no richer than the sine's %.3f", cr, cs)
	}
}

// TestHarmonicsNormalizesAmplitude: adding partials must not make the note
// louder, or every extra harmonic would edge it toward clipping.
func TestHarmonicsNormalizesAmplitude(t *testing.T) {
	peak := func(x []float32) float64 {
		var p float64
		for _, v := range x {
			if a := math.Abs(float64(v)); a > p {
				p = a
			}
		}
		return p
	}
	one := peak(render(sound.Harmonics(220, 1, 0.5, []float64{1}), 8192)[2048:6144])
	many := peak(render(sound.Harmonics(220, 1, 0.5, vowel), 8192)[2048:6144])
	if one > 0.55 {
		t.Errorf("single partial peaked at %.3f, want <= amp (0.5)", one)
	}
	if many > 0.55 {
		t.Errorf("five partials peaked at %.3f — normalization is not holding", many)
	}
}

// TestHarmonicsSkipsAliasing: partials above Nyquist must be dropped, not
// folded back as inharmonic tones.
func TestHarmonicsSkipsAliasing(t *testing.T) {
	// At 8 kHz the 4th harmonic (32 kHz) is well past Nyquist (22.05 kHz).
	d := &pitch.Detector{SampleRate: sound.SampleRate, MinFreq: 200, MaxFreq: 12000}
	buf := render(sound.Harmonics(8000, 1, 0.8, []float64{1, 0.6, 0.35, 0.2, 0.1}), 8192)
	got := d.Detect(buf[2048:6144])
	// The exact reading matters less than the absence of a low aliased tone.
	if got.Voiced && got.Freq < 4000 {
		t.Errorf("aliasing produced a spurious low tone at %.0f Hz", got.Freq)
	}
}

func TestHarmonicsDegradesToTone(t *testing.T) {
	if sound.Harmonics(440, 0.5, 0.5, nil) == nil {
		t.Error("nil partials returned no source")
	}
	if sound.Harmonics(440, 0.5, 0.5, []float64{0, 0}) == nil {
		t.Error("all-zero partials returned no source")
	}
}

// TestHarmonicsEnds: a one-shot must finish, or a voice would linger in the
// mixer forever.
func TestHarmonicsEnds(t *testing.T) {
	src := sound.Harmonics(440, 0.05, 0.5, vowel)
	block := make([]float32, 512)
	for i := 0; i < 100; i++ {
		if !src.Process(block) {
			return
		}
	}
	t.Error("a 50 ms note never reported finished")
}
