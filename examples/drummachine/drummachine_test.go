package main

import (
	"math"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/sound"
)

// TestKitSynthesis checks every drum voice is a sane one-shot: non-empty,
// within [-1,1] (no clipping), and decaying to near-silence by its tail.
func TestKitSynthesis(t *testing.T) {
	for _, v := range kit() {
		d := v.sample.Data
		if len(d) < int(0.01*sr) {
			t.Fatalf("%s: sample too short (%d samples)", v.name, len(d))
		}
		var peak float32
		for _, x := range d {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				t.Fatalf("%s: non-finite sample", v.name)
			}
			if a := absf(x); a > peak {
				peak = a
			}
		}
		if peak > 1.0 {
			t.Errorf("%s: peak %.3f clips past 1.0", v.name, peak)
		}
		if peak < 0.05 {
			t.Errorf("%s: peak %.3f is inaudibly quiet", v.name, peak)
		}
		// Tail (last 5%) must have decayed well below the peak.
		var tail float32
		for _, x := range d[len(d)*95/100:] {
			if a := absf(x); a > tail {
				tail = a
			}
		}
		if tail > 0.2*peak {
			t.Errorf("%s: tail %.3f didn't decay (peak %.3f)", v.name, tail, peak)
		}
	}
}

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// TestClockAdvances proves the sequencer clock steps at the right rate and wraps.
func TestClockAdvances(t *testing.T) {
	var g *drum
	stateHook = func(gg *drum) { g = gg }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(App{Mixer: sound.NewMixer()}, app.Config{Size: geom.Size{W: 760, H: 424}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	g.bpm = 120 // 16th-note step = 60/120/4 = 0.125s
	g.start()   // step 0, acc 0

	// Advance just over four steps' worth of time; step should land on 4.
	h.Step(0.125*4 + 0.001)
	if g.step != 4 {
		t.Fatalf("after 4 steps, step=%d want 4", g.step)
	}
	// Advance a full bar (16 steps) more; step wraps back to 4.
	h.Step(0.125 * 16)
	if g.step != 4 {
		t.Fatalf("after wrapping a bar, step=%d want 4", g.step)
	}
}

// TestInputWiring drives the real Core dispatch: a grid tap toggles a step, the
// Play button and Space toggle playback, tempo buttons change the BPM, and
// Clear empties the grid.
func TestInputWiring(t *testing.T) {
	var g *drum
	stateHook = func(gg *drum) { g = gg }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(App{Mixer: sound.NewMixer()}, app.Config{Size: geom.Size{W: 760, H: 424}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // mount + draw (sets cell + button rects)
	if g == nil {
		t.Fatal("Init hook never fired")
	}

	center := func(r geom.Rect) geom.Pt {
		return geom.Pt{X: r.Min.X + r.Dx()/2, Y: r.Min.Y + r.Dy()/2}
	}

	// A grid tap toggles that step (pick one the default pattern leaves empty).
	if g.grid[0][5] {
		t.Fatal("expected step (kick,5) to start empty")
	}
	h.Tap(center(g.cell[0][5]))
	if !g.grid[0][5] {
		t.Fatal("grid tap didn't turn the step on")
	}
	h.Tap(center(g.cell[0][5]))
	if g.grid[0][5] {
		t.Fatal("second grid tap didn't turn the step off")
	}

	// Play button toggles playback.
	g.playing = true
	h.Tap(center(g.playBtn))
	if g.playing {
		t.Fatal("Play button didn't stop playback")
	}
	// Space toggles it back on.
	h.Key(shell.KeySpace)
	if !g.playing {
		t.Fatal("Space didn't start playback")
	}

	// Tempo buttons change BPM within bounds.
	g.bpm = 120
	h.Tap(center(g.tempoUp))
	if g.bpm != 125 {
		t.Fatalf("tempo+ = %.0f, want 125", g.bpm)
	}
	h.Tap(center(g.tempoDn))
	h.Tap(center(g.tempoDn))
	if g.bpm != 115 {
		t.Fatalf("tempo- twice = %.0f, want 115", g.bpm)
	}

	// Clear empties every step.
	h.Tap(center(g.clrBtn))
	for v := 0; v < numVoices; v++ {
		for st := 0; st < steps; st++ {
			if g.grid[v][st] {
				t.Fatalf("Clear left step (%d,%d) on", v, st)
			}
		}
	}
}
