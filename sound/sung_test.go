package sound_test

import (
	"math"
	"testing"

	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/sound/pitch"
)

// renderLong pulls a source dry for a long note. The package's other tests use
// render() for short ones; a sung note needs seconds, not milliseconds, before
// its vibrato has anything to show.
func renderLong(src sound.Source, n int) []float32 {
	out := make([]float32, 0, n)
	block := make([]float32, 256)
	for len(out) < n {
		more := src.Process(block)
		out = append(out, block...)
		if !more {
			break
		}
	}
	if len(out) > n {
		out = out[:n]
	}
	return out
}

// TestSungHasTheRightPitch is the property everything else rests on: a singer
// asked to match this must be matching the note we meant.
func TestSungHasTheRightPitch(t *testing.T) {
	d := &pitch.Detector{SampleRate: sound.SampleRate}
	for _, name := range []string{"G3", "C4", "E4", "A4", "C5"} {
		n := pitch.MustParse(name)
		// Vibrato off: its whole purpose is that the pitch moves, so measuring
		// it against one value would be measuring the wrong thing.
		buf := renderLong(sound.Sung(n.Freq(), 1.5, 0.8, sound.VoiceOptions{Vibrato: 0.0001}), 40000)
		got := d.Detect(buf[8000:14000])
		if !got.Voiced {
			t.Errorf("%s: not voiced", name)
			continue
		}
		if cents := pitch.Cents(n.Freq(), got.Freq); math.Abs(cents) > 8 {
			t.Errorf("%s: %.1f cents off (%.1f Hz)", name, cents, got.Freq)
		}
	}
}

// TestVowelsDifferInTimbre: if two vowels give the same spectrum then the
// formant filtering is not doing anything and this is a buzzer with extra steps.
func TestVowelsDifferInTimbre(t *testing.T) {
	grab := func(v sound.Vowel) []float32 {
		return renderLong(sound.Sung(220, 1, 0.8, sound.VoiceOptions{Vowel: v, Vibrato: 0.0001}), 30000)[8000:12000]
	}
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
	ah, ee := crest(grab(sound.VowelAh)), crest(grab(sound.VowelEe))
	if math.Abs(ah-ee) < 0.15 {
		t.Errorf("ah and ee crest factors are near-identical (%.3f vs %.3f) — "+
			"the formants are shaping nothing", ah, ee)
	}
}

// TestVibratoMovesThePitch: without it this is a synthesizer, with it a voice.
func TestVibratoMovesThePitch(t *testing.T) {
	d := &pitch.Detector{SampleRate: sound.SampleRate}
	buf := renderLong(sound.Sung(440, 3, 0.8, sound.VoiceOptions{Vibrato: 60, Rate: 5}), 120000)

	lo, hi := math.Inf(1), 0.0
	for start := 60000; start+3000 < len(buf); start += 2000 {
		if r := d.Detect(buf[start : start+3000]); r.Voiced {
			lo, hi = math.Min(lo, r.Freq), math.Max(hi, r.Freq)
		}
	}
	spread := pitch.Cents(lo, hi)
	if spread < 30 {
		t.Errorf("vibrato spread only %.0f cents — asked for +/-60", spread)
	}
	if spread > 220 {
		t.Errorf("vibrato spread %.0f cents — far wider than asked for", spread)
	}
}

// TestVibratoStartsLate: a note that wobbles from the first instant is harder
// to pitch against than one that arrives steady and then blooms.
func TestVibratoStartsLate(t *testing.T) {
	d := &pitch.Detector{SampleRate: sound.SampleRate}
	buf := renderLong(sound.Sung(440, 3, 0.8, sound.VoiceOptions{Vibrato: 80, Rate: 5}), 120000)

	lo, hi := math.Inf(1), 0.0
	var n int
	for start := 3000; start < 12000; start += 1500 {
		if r := d.Detect(buf[start : start+3000]); r.Voiced {
			lo, hi = math.Min(lo, r.Freq), math.Max(hi, r.Freq)
			n++
		}
	}
	if n < 3 {
		t.Fatal("not enough voiced windows at the onset")
	}
	if s := pitch.Cents(lo, hi); s > 45 {
		t.Errorf("the note wobbles %.0f cents in its first quarter second", s)
	}
}

func TestSungDoesNotClip(t *testing.T) {
	for _, v := range []sound.Vowel{sound.VowelAh, sound.VowelEe, sound.VowelOo, sound.VowelOh} {
		for _, s := range renderLong(sound.Sung(196, 1, 0.5, sound.VoiceOptions{Vowel: v}), 30000) {
			if math.Abs(float64(s)) > 0.62 {
				t.Errorf("vowel %d clipped at %.3f with amp 0.5", v, s)
				break
			}
		}
	}
}

func TestSungEnds(t *testing.T) {
	src := sound.Sung(440, 0.05, 0.5, sound.VoiceOptions{})
	block := make([]float32, 512)
	for range 200 {
		if !src.Process(block) {
			return
		}
	}
	t.Error("a 50ms note never reported finished")
}
