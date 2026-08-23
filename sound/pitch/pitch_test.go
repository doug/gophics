package pitch_test

import (
	"math"
	"math/rand"
	"testing"

	"github.com/doug/gophics/sound/pitch"
)

const rate = 44100

// tone synthesizes n samples of a sine at freq.
func tone(freq float64, n int, amp float64) []float32 {
	out := make([]float32, n)
	for i := range out {
		out[i] = float32(amp * math.Sin(2*math.Pi*freq*float64(i)/rate))
	}
	return out
}

// voiced synthesizes a harmonically rich tone whose fundamental is deliberately
// weaker than its partials — the shape of an open sung vowel, and the case a
// spectral peak-picker gets wrong by an octave.
func voiced(freq float64, n int, partials []float64) []float32 {
	out := make([]float32, n)
	for i := range out {
		var s float64
		t := float64(i) / rate
		for h, amp := range partials {
			s += amp * math.Sin(2*math.Pi*freq*float64(h+1)*t)
		}
		out[i] = float32(s * 0.3)
	}
	return out
}

func newDetector() *pitch.Detector {
	return &pitch.Detector{SampleRate: rate}
}

// TestDetectPureTones checks accuracy across the singing range. The tolerance
// is 5 cents: a trained ear notices about 10, so a detector that is to judge
// intonation must be comfortably better than the listener.
func TestDetectPureTones(t *testing.T) {
	d := newDetector()
	n := pitch.WindowFor(pitch.DefaultMinFreq, rate)
	for _, freq := range []float64{82.41, 110, 146.83, 220, 261.63, 440, 587.33, 880} {
		got := d.Detect(tone(freq, n, 0.5))
		if !got.Voiced {
			t.Errorf("%.2f Hz: not voiced", freq)
			continue
		}
		if cents := pitch.Cents(freq, got.Freq); math.Abs(cents) > 5 {
			t.Errorf("%.2f Hz: got %.2f Hz (%.1f cents off)", freq, got.Freq, cents)
		}
		if got.Clarity < 0.8 {
			t.Errorf("%.2f Hz: clarity %.2f too low for a pure tone", freq, got.Clarity)
		}
	}
}

// TestMissingFundamental is the octave-error regression test. The fundamental
// is absent entirely and the second harmonic is loudest; the perceived pitch —
// and the correct answer — is still the fundamental.
func TestMissingFundamental(t *testing.T) {
	d := newDetector()
	n := pitch.WindowFor(pitch.DefaultMinFreq, rate)
	for _, freq := range []float64{110, 220, 261.63} {
		// Partial amplitudes: no fundamental, strong 2nd and 3rd.
		got := d.Detect(voiced(freq, n, []float64{0, 1.0, 0.8, 0.4, 0.2}))
		if !got.Voiced {
			t.Errorf("%.2f Hz: not voiced", freq)
			continue
		}
		if cents := pitch.Cents(freq, got.Freq); math.Abs(cents) > 10 {
			t.Errorf("missing fundamental %.2f Hz: got %.2f Hz (%.1f cents off) — likely an octave error",
				freq, got.Freq, cents)
		}
	}
}

// TestVowelLikeTone checks a realistic sung spectrum: strong fundamental with a
// decaying harmonic series.
func TestVowelLikeTone(t *testing.T) {
	d := newDetector()
	n := pitch.WindowFor(pitch.DefaultMinFreq, rate)
	for _, freq := range []float64{130.81, 196, 329.63, 523.25} {
		got := d.Detect(voiced(freq, n, []float64{1.0, 0.6, 0.4, 0.25, 0.15, 0.1}))
		if !got.Voiced {
			t.Errorf("%.2f Hz: not voiced", freq)
			continue
		}
		if cents := pitch.Cents(freq, got.Freq); math.Abs(cents) > 5 {
			t.Errorf("%.2f Hz: got %.2f Hz (%.1f cents off)", freq, got.Freq, cents)
		}
	}
}

// TestNoisyTone adds white noise at a level a phone mic in a room would pick up.
func TestNoisyTone(t *testing.T) {
	d := newDetector()
	n := pitch.WindowFor(pitch.DefaultMinFreq, rate)
	rng := rand.New(rand.NewSource(1))
	for _, freq := range []float64{146.83, 220, 440} {
		x := voiced(freq, n, []float64{1.0, 0.6, 0.3})
		for i := range x {
			x[i] += float32(rng.NormFloat64() * 0.05)
		}
		got := d.Detect(x)
		if !got.Voiced {
			t.Errorf("%.2f Hz + noise: not voiced", freq)
			continue
		}
		if cents := pitch.Cents(freq, got.Freq); math.Abs(cents) > 15 {
			t.Errorf("%.2f Hz + noise: got %.2f Hz (%.1f cents off)", freq, got.Freq, cents)
		}
	}
}

// TestSilenceAndNoiseUnvoiced makes sure the app never draws a confident needle
// for a quiet room — the failure that makes a tuner feel broken.
func TestSilenceAndNoiseUnvoiced(t *testing.T) {
	d := newDetector()
	n := pitch.WindowFor(pitch.DefaultMinFreq, rate)

	if got := d.Detect(make([]float32, n)); got.Voiced {
		t.Errorf("silence reported voiced at %.2f Hz", got.Freq)
	}

	rng := rand.New(rand.NewSource(2))
	quiet := make([]float32, n)
	for i := range quiet {
		quiet[i] = float32(rng.NormFloat64() * 0.001)
	}
	if got := d.Detect(quiet); got.Voiced {
		t.Errorf("near-silence reported voiced at %.2f Hz (rms %.4f)", got.Freq, got.RMS)
	}
}

// TestLoudNoiseHasLowClarity: white noise is loud enough to pass the RMS gate,
// so clarity is what must keep the UI honest about it.
func TestLoudNoiseHasLowClarity(t *testing.T) {
	d := newDetector()
	n := pitch.WindowFor(pitch.DefaultMinFreq, rate)
	rng := rand.New(rand.NewSource(3))
	x := make([]float32, n)
	for i := range x {
		x[i] = float32(rng.NormFloat64() * 0.3)
	}
	if got := d.Detect(x); got.Voiced && got.Clarity > 0.6 {
		t.Errorf("white noise: clarity %.2f at %.2f Hz — too confident", got.Clarity, got.Freq)
	}
}

// TestOutOfRangeRejected keeps sub- and super-range energy from producing notes.
func TestOutOfRangeRejected(t *testing.T) {
	d := &pitch.Detector{SampleRate: rate, MinFreq: 100, MaxFreq: 500}
	n := pitch.WindowFor(100, rate)
	if got := d.Detect(tone(60, n, 0.5)); got.Voiced && got.Freq < 100 {
		t.Errorf("60 Hz passed a 100 Hz floor: %.2f Hz", got.Freq)
	}
	if got := d.Detect(tone(900, n, 0.5)); got.Voiced && got.Freq > 500 {
		t.Errorf("900 Hz passed a 500 Hz ceiling: %.2f Hz", got.Freq)
	}
}

// TestShortWindowDoesNotLie: a window too short to hold two periods of the
// lowest note must decline rather than invent a pitch.
func TestShortWindowDoesNotLie(t *testing.T) {
	d := newDetector()
	// 256 samples is ~5.8 ms — under one period of 65 Hz.
	if got := d.Detect(tone(65, 256, 0.5)); got.Voiced && got.Freq < 100 {
		t.Errorf("65 Hz in a 256-sample window claimed %.2f Hz", got.Freq)
	}
}

func TestWindowFor(t *testing.T) {
	// Two periods of 65 Hz at 44.1 kHz is ~1357 samples.
	if got := pitch.WindowFor(65, rate); got < 1300 || got > 1400 {
		t.Errorf("WindowFor(65, 44100) = %d, want ~1357", got)
	}
	if got := pitch.WindowFor(0, rate); got != 0 {
		t.Errorf("WindowFor(0, ...) = %d, want 0", got)
	}
}

func TestNoteFreqRoundTrip(t *testing.T) {
	for n := pitch.Note(24); n <= 96; n++ {
		got, cents := pitch.FromFreq(n.Freq())
		if got != n {
			t.Errorf("%v (%.3f Hz) round-tripped to %v", n, n.Freq(), got)
		}
		if math.Abs(cents) > 1e-6 {
			t.Errorf("%v: exact frequency gave %.6f cents", n, cents)
		}
	}
}

func TestNoteNames(t *testing.T) {
	cases := []struct {
		n    pitch.Note
		want string
		hz   float64
	}{
		{pitch.A4, "A4", 440},
		{pitch.MiddleC, "C4", 261.626},
		{pitch.Note(21), "A0", 27.5},
		{pitch.Note(108), "C8", 4186.01},
		{pitch.Note(61), "C#4", 277.183},
	}
	for _, c := range cases {
		if got := c.n.String(); got != c.want {
			t.Errorf("Note(%d).String() = %q, want %q", c.n, got, c.want)
		}
		if got := c.n.Freq(); math.Abs(got-c.hz) > 0.01 {
			t.Errorf("%s.Freq() = %.3f, want %.3f", c.want, got, c.hz)
		}
	}
}

func TestFromFreqCents(t *testing.T) {
	// 15 cents sharp of A4.
	sharp := 440 * math.Exp2(15.0/1200)
	n, cents := pitch.FromFreq(sharp)
	if n != pitch.A4 {
		t.Errorf("got note %v, want A4", n)
	}
	if math.Abs(cents-15) > 0.01 {
		t.Errorf("got %.3f cents, want 15", cents)
	}
	// Exactly halfway between notes rounds consistently and stays in range.
	_, half := pitch.FromFreq(440 * math.Exp2(50.0/1200))
	if math.Abs(half) > 50.0001 {
		t.Errorf("half-semitone gave %.3f cents, want |c| <= 50", half)
	}
}

func TestParse(t *testing.T) {
	cases := map[string]pitch.Note{
		"A4": 69, "C4": 60, "C#4": 61, "Db4": 61, "Bb3": 58,
		"c4": 60, "G#2": 44, "A0": 21, "C8": 108, "Cs4": 61,
	}
	for s, want := range cases {
		got, err := pitch.Parse(s)
		if err != nil {
			t.Errorf("Parse(%q): %v", s, err)
			continue
		}
		if got != want {
			t.Errorf("Parse(%q) = %d (%v), want %d", s, got, got, want)
		}
	}
	for _, bad := range []string{"", "H4", "A", "A#", "4", "Az", "A4x"} {
		if _, err := pitch.Parse(bad); err == nil {
			t.Errorf("Parse(%q) succeeded, want error", bad)
		}
	}
}

func TestCents(t *testing.T) {
	if got := pitch.Cents(440, 880); math.Abs(got-1200) > 1e-9 {
		t.Errorf("octave = %.6f cents, want 1200", got)
	}
	if got := pitch.Cents(440, 440); got != 0 {
		t.Errorf("unison = %.6f cents, want 0", got)
	}
	if got := pitch.Cents(880, 440); math.Abs(got+1200) > 1e-9 {
		t.Errorf("descending octave = %.6f cents, want -1200", got)
	}
}

// TestDetectorReuse guards the scratch buffers: successive Detect calls at
// different window sizes must not leak state into one another.
func TestDetectorReuse(t *testing.T) {
	d := newDetector()
	big := pitch.WindowFor(pitch.DefaultMinFreq, rate)
	first := d.Detect(tone(220, big, 0.5))
	d.Detect(tone(440, 700, 0.5))
	again := d.Detect(tone(220, big, 0.5))
	if math.Abs(first.Freq-again.Freq) > 0.01 {
		t.Errorf("reuse changed the answer: %.3f then %.3f Hz", first.Freq, again.Freq)
	}
}

func BenchmarkDetect(b *testing.B) {
	d := newDetector()
	x := voiced(220, pitch.WindowFor(pitch.DefaultMinFreq, rate), []float64{1, 0.6, 0.3})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.Detect(x)
	}
}
