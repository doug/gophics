package main

import (
	"math"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/theme"
)

// --- Synthesis ---------------------------------------------------------------

// render pulls a whole voice into one buffer, block by block the way the mixer
// does, and reports the samples plus whether the source retired on its own.
func render(src sound.Source, secs float64) ([]float32, bool) {
	out := make([]float32, 0, int(secs*sr))
	block := make([]float32, 512)
	for len(out) < cap(out) {
		if !src.Process(block) {
			out = append(out, block...)
			return out, true
		}
		out = append(out, block...)
	}
	return out, false
}

func peak(buf []float32) float32 {
	var p float32
	for _, v := range buf {
		if a := absf(v); a > p {
			p = a
		}
	}
	return p
}

func absf(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// TestVoicesAreSaneOneShots is the audio contract every voice has to meet:
// audible, unclipped, finite, and finished (not merely quiet) before the mixer
// would otherwise carry it forever.
func TestVoicesAreSaneOneShots(t *testing.T) {
	const sustain = 0.6
	for v := VoicePluck; v <= VoiceGlass; v++ {
		buf, done := render(Note(v, 440, sustain, 1), sustain*4)
		name := voiceNames[v]
		if !done {
			t.Errorf("%s: never retired within %.1fs of a %.1fs sustain", name, sustain*4, sustain)
		}
		p := peak(buf)
		if p > 1 {
			t.Errorf("%s: peak %.3f clips past 1.0", name, p)
		}
		if p < 0.05 {
			t.Errorf("%s: peak %.3f is inaudibly quiet", name, p)
		}
		for _, x := range buf {
			if math.IsNaN(float64(x)) || math.IsInf(float64(x), 0) {
				t.Fatalf("%s: non-finite sample", name)
			}
		}
		// The tail has to be well down on the peak, or a held chord piles up.
		if tail := peak(buf[len(buf)*90/100:]); tail > 0.1*p {
			t.Errorf("%s: tail %.3f didn't decay (peak %.3f)", name, tail, p)
		}
	}
}

// TestVoicesAreDeterministic guards the property the whole file is written for:
// the noise-excited voice is seeded, so the same note renders identically twice
// and a golden test over audio would be stable.
func TestVoicesAreDeterministic(t *testing.T) {
	a, _ := render(Note(VoicePluck, 330, 0.4, 42), 0.4)
	b, _ := render(Note(VoicePluck, 330, 0.4, 42), 0.4)
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("seed 42 diverged at sample %d: %v vs %v", i, a[i], b[i])
		}
	}
	c, _ := render(Note(VoicePluck, 330, 0.4, 43), 0.4)
	if len(c) == len(a) && peak(c) == peak(a) {
		t.Error("a different seed produced an identical excitation")
	}
}

// TestPluckTracksPitch checks the Karplus-Strong delay line is actually the
// length its frequency asks for — the one thing that, if wrong, makes every
// note in tune with itself and out of tune with the scale.
func TestPluckTracksPitch(t *testing.T) {
	for _, freq := range []float64{110, 220, 440, 880} {
		p := newPluck(freq, 0.5, 1)
		want := int(math.Round(sr / freq))
		if len(p.buf) != want {
			t.Errorf("%.0f Hz: delay line %d samples, want %d", freq, len(p.buf), want)
		}
	}
}

// TestScaleFreqClimbs walks a scale past its own length and checks it keeps
// rising, an octave per wrap — the mapping that puts high notes at the top of
// the matrix.
func TestScaleFreqClimbs(t *testing.T) {
	for _, sc := range scales {
		prev := 0.0
		for step := 0; step < len(sc.Degrees)*3; step++ {
			f := sc.Freq(step, 57)
			if f <= prev {
				t.Fatalf("%s: step %d gave %.2f Hz, not above %.2f", sc.Name, step, f, prev)
			}
			prev = f
		}
		root := sc.Freq(0, 57)
		oct := sc.Freq(len(sc.Degrees), 57)
		if ratio := oct / root; math.Abs(ratio-2) > 0.001 {
			t.Errorf("%s: a full scale spans %.4f×, want an octave", sc.Name, ratio)
		}
	}
}

// --- Simulation --------------------------------------------------------------

func newApp(t *testing.T) (*app.Headless, *lum) {
	t.Helper()
	var st *lum
	stateHook = func(s *lum) { st = s }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(App{Mixer: sound.NewMixer()}, app.Config{
		Size:         geom.Size{W: 1040, H: 720},
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Resize(geom.Size{W: 1040, H: 720})
	h.Render() // mounts, and caches the matrix rect for hit testing
	if st == nil {
		t.Fatal("state never mounted")
	}
	return h, st
}

// TestCrawlerWalksAndWraps proves the agent moves one cell per step and comes
// back on at the far edge — the torus is what keeps crawlers alive with no
// spawn/despawn bookkeeping at all.
func TestCrawlerWalksAndWraps(t *testing.T) {
	_, s := newApp(t)
	s.grid = [rows][cols]cell{}
	s.crawlers = []crawler{{x: cols - 2, y: 5, dir: 1}} // heading east, near the edge

	s.advance()
	if got := s.crawlers[0]; got.x != cols-1 || got.y != 5 {
		t.Fatalf("after one step: (%d,%d), want (%d,5)", got.x, got.y, cols-1)
	}
	s.advance()
	if got := s.crawlers[0]; got.x != 0 || got.y != 5 {
		t.Fatalf("after the wrap: (%d,%d), want (0,5)", got.x, got.y)
	}
}

// TestTurnsCloseALoop is the mechanic in one assertion: four clockwise turns at
// the corners of a rectangle capture a crawler forever. It also pins the sign
// convention — get the rotation backwards and the crawler escapes on step one.
func TestTurnsCloseALoop(t *testing.T) {
	_, s := newApp(t)
	s.grid = [rows][cols]cell{}
	for _, p := range [][2]int{{2, 2}, {6, 2}, {6, 6}, {2, 6}} {
		s.grid[p[1]][p[0]] = turnCW
	}
	s.crawlers = []crawler{{x: 2, y: 2, dir: 1}} // on a corner, heading east

	seen := map[[2]int]bool{}
	for i := range 16 { // the loop is 16 cells around
		s.advance()
		c := s.crawlers[0]
		if c.x < 2 || c.x > 6 || c.y < 2 || c.y > 6 {
			t.Fatalf("step %d escaped the box at (%d,%d)", i, c.x, c.y)
		}
		seen[[2]int{c.x, c.y}] = true
	}
	if c := s.crawlers[0]; c.x != 2 || c.y != 2 {
		t.Errorf("after a full lap: (%d,%d), want (2,2)", c.x, c.y)
	}
	if len(seen) != 16 {
		t.Errorf("lap covered %d distinct cells, want 16", len(seen))
	}
}

// TestNodeStrikeSoundsAndRipples checks a crawler crossing a node lights it,
// throws a ripple, and hands a voice to the mixer.
func TestNodeStrikeSoundsAndRipples(t *testing.T) {
	_, s := newApp(t)
	s.grid = [rows][cols]cell{}
	s.grid[5][4] = node
	s.crawlers = []crawler{{x: 3, y: 5, dir: 1}}
	s.ripples = nil
	s.echo = 0

	before := s.mixer.Len()
	s.advance()

	if s.flash[5][4] == 0 {
		t.Error("the struck node didn't light")
	}
	if len(s.ripples) != 1 {
		t.Errorf("got %d ripples, want 1", len(s.ripples))
	}
	if s.mixer.Len() != before+1 {
		t.Errorf("mixer has %d voices, want %d", s.mixer.Len(), before+1)
	}
}

// TestEchoRepeatsThenStops checks the scheduled delay fires its repeats and
// then drains — an echo that never retires would leak a struct per note.
func TestEchoRepeatsThenStops(t *testing.T) {
	_, s := newApp(t)
	s.grid = [rows][cols]cell{}
	s.echo = 0.5
	s.bpm = 120 // eighth-note step = 0.25s, so a repeat lands every 0.375s
	s.pending = nil
	s.strike(4, 5, nodeCol)

	if len(s.pending) != 1 {
		t.Fatalf("a strike queued %d echoes, want 1", len(s.pending))
	}
	before := s.mixer.Len()
	for i := 0; i < 200 && len(s.pending) > 0; i++ { // up to 4s, at 20ms a go
		s.drainEcho(0.02)
	}
	if len(s.pending) != 0 {
		t.Errorf("%d echoes still queued after 4s", len(s.pending))
	}
	// One strike queues a repeat that re-queues twice more: three extra voices.
	if got := s.mixer.Len() - before; got != 3 {
		t.Errorf("the echo sounded %d times, want 3", got)
	}
}

// TestTickDrivesTheClock checks the step rate follows the tempo, so a BPM
// change is a tempo change and not just a label.
func TestTickDrivesTheClock(t *testing.T) {
	_, s := newApp(t)
	s.grid = [rows][cols]cell{}
	s.crawlers = []crawler{{x: 0, y: 0, dir: 1}}
	s.playing = true
	s.bpm = 120 // eighth note = 0.25s
	s.acc = 0

	// Fed at a real frame rate: 14 frames is 0.233s, still short of the beat.
	for range 14 {
		s.Tick(1.0 / 60)
	}
	if s.crawlers[0].x != 0 {
		t.Error("stepped before the beat was due")
	}
	for range 2 { // 0.267s — over the line
		s.Tick(1.0 / 60)
	}
	if s.crawlers[0].x != 1 {
		t.Errorf("x = %d after one beat, want 1", s.crawlers[0].x)
	}

	// A long stall — a backgrounded tab, a paused debugger — is clamped to one
	// frame's worth rather than replayed as a burst of steps, which would
	// otherwise teleport every crawler and dump a chord into the mixer.
	s.acc = 0
	x := s.crawlers[0].x
	s.Tick(5)
	if got := s.crawlers[0].x - x; got != 0 {
		t.Errorf("a 5s stall fired %d steps; the clamp should have swallowed it", got)
	}
}

// TestTapCyclesCell drives the real pointer path: a tap on the matrix walks the
// cell through node → turn ↻ → turn ↺ → empty.
func TestTapCyclesCell(t *testing.T) {
	h, s := newApp(t)
	s.playing = false
	s.grid = [rows][cols]cell{}
	h.Render()

	// Centre of cell (4,6), in window coordinates: the canvas sits inside the
	// root padding, and the matrix is centred within the canvas.
	p := geom.Pt{
		X: pagePad + s.area.Min.X + (4+0.5)*s.step,
		Y: pagePad + s.area.Min.Y + (6+0.5)*s.step,
	}
	for _, want := range []cell{node, turnCW, turnCCW, empty} {
		h.Tap(p)
		h.Render()
		if got := s.grid[6][4]; got != want {
			t.Fatalf("tap gave cell %d, want %d", got, want)
		}
	}
}

// TestKeyboardShortcuts checks the keys the footer advertises actually work.
func TestKeyboardShortcuts(t *testing.T) {
	h, s := newApp(t)
	s.playing = true

	h.Key(shell.KeySpace)
	if s.playing {
		t.Error("space didn't pause")
	}
	before := len(s.crawlers)
	h.Key(shell.KeyA)
	if len(s.crawlers) != before+1 {
		t.Errorf("A gave %d crawlers, want %d", len(s.crawlers), before+1)
	}
	h.Key(shell.KeyC)
	for y := range s.grid {
		for x := range s.grid[y] {
			if s.grid[y][x] != empty {
				t.Fatalf("C left cell (%d,%d) set", x, y)
			}
		}
	}
}

// TestCrawlerCapIsHonoured keeps the Add button from growing the fleet past the
// palette that colours it.
func TestCrawlerCapIsHonoured(t *testing.T) {
	_, s := newApp(t)
	for range maxCrawlers * 3 {
		s.addCrawler()
	}
	if len(s.crawlers) != maxCrawlers {
		t.Errorf("got %d crawlers, want the cap of %d", len(s.crawlers), maxCrawlers)
	}
}

// TestLayoutFollowsTheWindow checks both shapes the page takes: side by side on
// a desktop, where the matrix is squeezed by the fixed-width panel, and stacked
// on a phone, where it gets the full width and the panel scrolls beneath it. A
// matrix that stayed beside the panel at phone widths would collapse to a few
// pixels a cell and paint nothing at all.
func TestLayoutFollowsTheWindow(t *testing.T) {
	h, s := newApp(t)
	wide := s.area
	if wide.Dx() != wide.Dy() {
		t.Errorf("matrix is %v, not square", wide.Size())
	}
	if room := float32(1040 - panelW - 3*pagePad); wide.Dx() > room {
		t.Errorf("matrix is %.0f wide, past the %.0f the panel leaves", wide.Dx(), room)
	}

	h.Resize(geom.Size{W: 420, H: 860})
	h.Render()
	narrow := s.area
	if narrow.Dx() != narrow.Dy() {
		t.Errorf("stacked matrix is %v, not square", narrow.Size())
	}
	if want := float32(420 - 2*pagePad); narrow.Dx() < want-cols {
		t.Errorf("stacked matrix is %.0f wide, want about %.0f — the full page width", narrow.Dx(), want)
	}
	if s.step < 6 {
		t.Errorf("cell is %.1fpx; the matrix would paint nothing", s.step)
	}
}

// TestRendersWithoutPanic runs a few seconds of the real thing — clock, audio,
// ripples, and paint — through the headless renderer.
func TestRendersWithoutPanic(t *testing.T) {
	h, s := newApp(t)
	s.playing = true
	for range 120 {
		h.Step(1.0 / 60)
	}
	img := h.Render()
	if img.Bounds().Dx() != 1040 {
		t.Fatalf("rendered %v", img.Bounds())
	}
}
