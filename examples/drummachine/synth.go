package main

import (
	"math"
	"math/rand"

	"github.com/doug/gophics/sound"
)

// Drum voices are synthesized from scratch into mono float32 PCM at the mixer's
// SampleRate — no audio assets. Each is a short one-shot the sequencer triggers
// on the beat. The math is deterministic (a seeded PRNG for the noise voices),
// so the samples are unit-testable.

const sr = float64(sound.SampleRate)

// kick: a sine whose pitch drops fast from ~120 Hz to ~48 Hz under a quick
// amplitude decay — the classic synthesized bass drum.
func kick() *sound.Sample {
	n := int(0.42 * sr)
	buf := make([]float32, n)
	phase := 0.0
	for i := range buf {
		t := float64(i) / sr
		f := 48 + (120-48)*math.Exp(-t*34) // pitch envelope
		phase += f / sr
		amp := math.Exp(-t * 9)
		buf[i] = float32(math.Sin(2*math.Pi*phase) * amp * 0.95)
	}
	return sound.NewSample(buf)
}

// snare: a short tonal body (~180 Hz) mixed with white noise, both decaying
// fast — body for the "thock", noise for the "sh".
func snare(rng *rand.Rand) *sound.Sample {
	n := int(0.2 * sr)
	buf := make([]float32, n)
	phase := 0.0
	for i := range buf {
		t := float64(i) / sr
		amp := math.Exp(-t * 24)
		phase += 180.0 / sr
		body := math.Sin(2*math.Pi*phase) * 0.5
		noise := rng.Float64()*2 - 1
		buf[i] = float32((body + noise*0.85) * amp * 0.7)
	}
	return sound.NewSample(buf)
}

// hat: high-passed white noise with an exponential decay. A short decay is a
// closed hi-hat, a long one an open hi-hat.
func hat(rng *rand.Rand, decay, amp float64) *sound.Sample {
	n := int((decay*4 + 0.01) * sr)
	buf := make([]float32, n)
	prev := 0.0
	for i := range buf {
		t := float64(i) / sr
		white := rng.Float64()*2 - 1
		hp := white - prev // crude one-pole high-pass (differencing)
		prev = white
		buf[i] = float32(hp * math.Exp(-t/decay) * amp)
	}
	return sound.NewSample(buf)
}

// clap: three quick high-passed noise bursts a few ms apart, then a short tail —
// the stacked transients that read as a hand clap.
func clap(rng *rand.Rand) *sound.Sample {
	n := int(0.24 * sr)
	buf := make([]float32, n)
	prev := 0.0
	for i := range buf {
		t := float64(i) / sr
		white := rng.Float64()*2 - 1
		hp := white - prev*0.6
		prev = white
		var env float64
		switch {
		case t < 0.009:
			env = math.Exp(-t / 0.003)
		case t < 0.018:
			env = math.Exp(-(t - 0.009) / 0.003)
		case t < 0.027:
			env = math.Exp(-(t - 0.018) / 0.004)
		default:
			env = math.Exp(-(t - 0.027) / 0.045)
		}
		buf[i] = float32(hp * env * 0.55)
	}
	return sound.NewSample(buf)
}

// voice is one row of the sequencer: a named drum sound at a mixing volume.
type voice struct {
	name   string
	sample *sound.Sample
	vol    float64
}

// kit builds the fixed five-voice drum kit, top row to bottom.
func kit() []voice {
	rng := rand.New(rand.NewSource(1)) // fixed seed → identical kit every run
	return []voice{
		{"Kick", kick(), 0.95},
		{"Snare", snare(rng), 0.8},
		{"Clap", clap(rng), 0.7},
		{"CH Hat", hat(rng, 0.03, 0.45), 0.6},
		{"OH Hat", hat(rng, 0.14, 0.4), 0.5},
	}
}
