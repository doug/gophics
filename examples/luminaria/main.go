// Command luminaria is a generative musical instrument in the lineage of Toshio
// Iwai's Electroplankton and the Tenori-On: a 16×16 matrix that crawlers walk
// across, lighting nodes and playing them.
//
// Tap a cell to cycle it — a node, then a clockwise turn, then a
// counter-clockwise turn, then empty again — and drag to paint. Crawlers
// advance one cell per beat, wrap at the edges, and rotate when they hit a
// turn. A node they cross lights up, rings out, and pushes a note into the
// mixer: pitch from its row, stereo pan from its column.
//
// It is the driver example for *melodic* synthesis (examples/drummachine is the
// percussive one): synth.go builds Karplus-Strong, FM, and detuned-pad voices in
// Go at the mixer's sample rate, with no audio assets and nothing
// platform-specific, so the same notes sound on desktop, in the browser, and on
// a phone. It is also the example where the widget layer and the paint escape
// hatch share a screen — the control panel is ordinary themed widgets, the
// matrix is one widget.Canvas.
//
//	go run ./examples/luminaria
package main

import (
	"fmt"
	"log"
	"math/rand"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/sound/device"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

const (
	cols = 16
	rows = 16

	minBPM = 60
	maxBPM = 320

	maxCrawlers = 6
	panelW      = 268
	pagePad     = 18
)

// cell is what a grid square holds. Tapping cycles through them in this order,
// which is also why empty is the zero value: a fresh board is silent.
type cell uint8

const (
	empty   cell = iota
	node         // lights, rings, and plays its row's pitch
	turnCW       // rotates a crawler a quarter turn clockwise
	turnCCW      // …and the other way
)

// next is the tap cycle.
func (c cell) next() cell {
	if c == turnCCW {
		return empty
	}
	return c + 1
}

// Directions, indexed by the crawler's dir. North is up the screen.
var delta = [4][2]int{{0, -1}, {1, 0}, {0, 1}, {-1, 0}}

type crawler struct {
	x, y int // current cell
	dir  int // where it leaves this cell
	from int // the direction it arrived by, for drawing the step it just took
	col  paint.Color
}

// ripple is the expanding ring a struck node throws off; t runs 0→1.
type ripple struct {
	x, y float32 // cell centre, in cell units
	t    float32
	col  paint.Color
}

// pending is an echo repeat waiting for its turn — the delay effect is
// scheduled here rather than mixed as a feedback line, so it costs one struct
// per repeat and stays deterministic.
type pending struct {
	in     float64 // seconds until it sounds
	freq   float64
	pan    float64
	vol    float64
	left   int // repeats still to come after this one
	gx, gy float32
	col    paint.Color
}

var (
	bg       = paint.RGB(0.055, 0.06, 0.085)
	gridDot  = paint.Color{R: 1, G: 1, B: 1, A: 0.07}
	gridLine = paint.Color{R: 1, G: 1, B: 1, A: 0.035}
	nodeCol  = paint.RGB(0.42, 0.74, 0.98)
	cwCol    = paint.RGB(0.98, 0.68, 0.32)
	ccwCol   = paint.RGB(0.72, 0.52, 0.98)
)

// crawlerCols tints each crawler and everything it lights, so two crawlers
// crossing the same node read as two separate voices.
var crawlerCols = [maxCrawlers]paint.Color{
	paint.RGB(0.42, 0.92, 0.76),
	paint.RGB(0.98, 0.51, 0.62),
	paint.RGB(0.98, 0.84, 0.40),
	paint.RGB(0.55, 0.72, 0.99),
	paint.RGB(0.78, 0.55, 0.98),
	paint.RGB(0.46, 0.95, 0.46),
}

type App struct{ Mixer *sound.Mixer }

func (App) CreateState() widget.State { return &lum{} }

type lum struct {
	widget.StateBase[App]
	ctx   widget.Ctx
	mixer *sound.Mixer
	rng   *rand.Rand

	grid     [rows][cols]cell
	flash    [rows][cols]float32 // 0..1, decays; how lit a node is right now
	crawlers []crawler
	ripples  []ripple
	pending  []pending

	playing bool
	bpm     float64
	acc     float64 // seconds into the current step
	scale   int
	voice   Voice
	octave  int     // index into octaveRoots
	sustain float64 // seconds
	echo    float32 // 0 = dry

	// Painting: a press cycles the cell it lands on and remembers the result,
	// which a drag then paints onto every cell it crosses. Without the memory a
	// drag would cycle each cell by however many move events it received.
	painting bool
	paintTo  cell
	lastCell [2]int

	area geom.Rect // the matrix's rect inside the canvas, cached at paint
	step float32   // one cell's side, cached with it

	// The two rotation glyphs, authored once in a unit square (see drawTurn).
	glyphArc  [2]*paint.Path
	glyphHead [2]*paint.Path
	seq       int64 // seeds the noise-excited voices, so playback is repeatable
}

var octaveNames = []string{"Low", "Mid", "High"}
var octaveRoots = []int{45, 57, 69} // MIDI A2 / A3 / A4

// stateHook, if set, receives the state on mount — for tests to drive input.
var stateHook func(*lum)

func (s *lum) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.mixer = s.W().Mixer
	s.rng = rand.New(rand.NewSource(7))
	s.bpm = 132
	s.octave = 1
	s.sustain = 1.4
	s.echo = 0.35
	s.playing = true
	s.seed()
	s.addCrawler()
	s.addCrawler()
	ctx.AddTicker(s)
	if stateHook != nil {
		stateHook(s)
	}
}

// seed lays down an opening pattern: a scatter of nodes plus a ring of turns
// that catches a crawler into a loop, so the demo has something to say before
// anyone has touched it.
func (s *lum) seed() {
	s.grid = [rows][cols]cell{}
	for _, p := range [][2]int{{3, 3}, {12, 3}, {12, 12}, {3, 12}} {
		s.grid[p[1]][p[0]] = turnCW
	}
	for range 22 {
		x, y := s.rng.Intn(cols), s.rng.Intn(rows)
		if s.grid[y][x] == empty {
			s.grid[y][x] = node
		}
	}
}

func (s *lum) addCrawler() {
	if len(s.crawlers) >= maxCrawlers {
		return
	}
	i := len(s.crawlers)
	s.crawlers = append(s.crawlers, crawler{
		x:   s.rng.Intn(cols),
		y:   s.rng.Intn(rows),
		dir: i % 4,
		col: crawlerCols[i],
	})
}

func (s *lum) removeCrawler() {
	if len(s.crawlers) > 0 {
		s.crawlers = s.crawlers[:len(s.crawlers)-1]
	}
}

func (s *lum) stepDur() float64 { return 60 / s.bpm / 2 } // eighth notes

// Tick runs the clock, the decay of everything that glows, and the echo queue.
// It reports true while it wants more frames, which is always: even stopped,
// ripples and flashes are still fading.
func (s *lum) Tick(dt float64) bool {
	if dt > 0.1 { // a backgrounded tab shouldn't fire a burst of steps on return
		dt = 0.1
	}
	if s.playing {
		dur := s.stepDur()
		for s.acc += dt; s.acc >= dur; s.acc -= dur {
			s.advance()
		}
	}
	s.decay(float32(dt))
	s.drainEcho(dt)
	s.ctx.Invalidate()
	return true
}

// advance walks every crawler one cell and applies whatever it landed on.
func (s *lum) advance() {
	for i := range s.crawlers {
		c := &s.crawlers[i]
		d := delta[c.dir]
		c.x = (c.x + d[0] + cols) % cols
		c.y = (c.y + d[1] + rows) % rows
		c.from = c.dir
		switch s.grid[c.y][c.x] {
		case node:
			s.strike(c.x, c.y, c.col)
		case turnCW:
			c.dir = (c.dir + 1) % 4
		case turnCCW:
			c.dir = (c.dir + 3) % 4
		}
	}
}

// strike lights a node and sounds it. Pitch climbs up the board (row 0 is the
// top and the highest step); pan follows the column, so a pattern that sweeps
// left to right sweeps across the stereo field with it.
func (s *lum) strike(x, y int, col paint.Color) {
	s.flash[y][x] = 1
	s.ripples = append(s.ripples, ripple{x: float32(x) + 0.5, y: float32(y) + 0.5, col: col})

	freq := scales[s.scale].Freq(rows-1-y, octaveRoots[s.octave])
	pan := (float64(x)/(cols-1))*1.6 - 0.8
	s.play(freq, pan, 0.5)
	if s.echo > 0.02 {
		s.pending = append(s.pending, pending{
			in: s.stepDur() * 1.5, freq: freq, pan: -pan * 0.7,
			vol: 0.5 * float64(s.echo), left: 2,
			gx: float32(x) + 0.5, gy: float32(y) + 0.5, col: col,
		})
	}
}

func (s *lum) play(freq, pan, vol float64) {
	if s.mixer == nil {
		return
	}
	s.seq++
	s.mixer.PlaySource(Note(s.voice, freq, s.sustain, s.seq), sound.PlayOptions{Volume: vol, Pan: pan})
}

// drainEcho sounds any repeats that came due, re-queueing the ones that still
// have repeats left at a lower volume and on the opposite side.
func (s *lum) drainEcho(dt float64) {
	out := s.pending[:0]
	for _, p := range s.pending {
		if p.in -= dt; p.in > 0 {
			out = append(out, p)
			continue
		}
		s.play(p.freq, p.pan, p.vol)
		s.ripples = append(s.ripples, ripple{x: p.gx, y: p.gy, col: p.col.WithAlpha(0.5)})
		if p.left > 0 {
			p.left--
			p.in = s.stepDur() * 1.5
			p.vol *= 0.55
			p.pan = -p.pan * 0.7
			out = append(out, p)
		}
	}
	s.pending = out
}

func (s *lum) decay(dt float32) {
	for y := range s.flash {
		for x := range s.flash[y] {
			if s.flash[y][x] > 0 {
				if s.flash[y][x] -= dt * 2.2; s.flash[y][x] < 0 {
					s.flash[y][x] = 0
				}
			}
		}
	}
	out := s.ripples[:0]
	for _, r := range s.ripples {
		if r.t += dt * 1.5; r.t < 1 {
			out = append(out, r)
		}
	}
	s.ripples = out
}

func (s *lum) clear() {
	s.SetState(func() {
		s.grid = [rows][cols]cell{}
		s.ripples = s.ripples[:0]
		s.pending = s.pending[:0]
	})
}

// --- Input -------------------------------------------------------------------

// cellAt maps a canvas point to a grid cell.
func (s *lum) cellAt(p geom.Pt) (int, int, bool) {
	if s.step <= 0 || !s.area.Contains(p) {
		return 0, 0, false
	}
	x := int((p.X - s.area.Min.X) / s.step)
	y := int((p.Y - s.area.Min.Y) / s.step)
	if x < 0 || x >= cols || y < 0 || y >= rows {
		return 0, 0, false
	}
	return x, y, true
}

func (s *lum) onPress(p geom.Pt) {
	x, y, ok := s.cellAt(p)
	if !ok {
		return
	}
	s.paintTo = s.grid[y][x].next()
	s.painting = true
	s.lastCell = [2]int{x, y}
	s.SetState(func() { s.set(x, y, s.paintTo) })
}

func (s *lum) onDrag(p geom.Pt, _ geom.Pt) {
	if !s.painting {
		return
	}
	x, y, ok := s.cellAt(p)
	if !ok || (x == s.lastCell[0] && y == s.lastCell[1]) {
		return
	}
	s.lastCell = [2]int{x, y}
	s.SetState(func() { s.set(x, y, s.paintTo) })
}

// set writes a cell and previews it, so painting a node is audible immediately
// rather than only when a crawler eventually reaches it.
func (s *lum) set(x, y int, c cell) {
	s.grid[y][x] = c
	if c == node {
		s.flash[y][x] = 1
		s.play(scales[s.scale].Freq(rows-1-y, octaveRoots[s.octave]),
			(float64(x)/(cols-1))*1.6-0.8, 0.34)
	}
}

func (s *lum) onKey(k shell.Key) {
	if k.Kind != shell.KeyPress {
		return
	}
	switch k.Code {
	case shell.KeySpace:
		s.SetState(func() { s.playing = !s.playing })
	case shell.KeyC:
		s.clear()
	case shell.KeyA:
		s.SetState(s.addCrawler)
	case shell.KeyR:
		s.SetState(s.seed)
	}
}

// --- Build -------------------------------------------------------------------

func (s *lum) Build(ctx widget.Ctx) widget.Widget {
	// The instrument is light-on-dark by nature — the whole point is nodes
	// glowing on an unlit field — so it pins the dark theme rather than
	// following the platform scheme, and app.Config paints the same colour
	// behind it in both.
	th := theme.Dark()
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: bg,
		Child: widget.Padding{All: pagePad, Child: widget.LayoutBuilder{
			Build: func(cs layout.Constraints) widget.Widget {
				if cs.BoundedW() && cs.Max.W < panelW*2.4 {
					return s.stacked(ctx, th)
				}
				return s.sideBySide(ctx, th)
			},
		}},
	}}
}

// sideBySide is the desktop shape: the matrix takes everything the fixed-width
// panel doesn't.
func (s *lum) sideBySide(ctx widget.Ctx, th theme.Theme) widget.Widget {
	return widget.Flex{
		Axis:       layout.Horizontal,
		CrossAlign: layout.CrossStretch,
		Children: []widget.Widget{
			widget.Expand(s.matrix()),
			widget.Sized{W: pagePad},
			widget.Sized{W: panelW, Child: widget.Scroll{Child: s.panel(ctx, th)}},
		},
	}
}

// stacked is the phone shape: a square matrix across the top with the panel
// under it, the whole page scrolling. The matrix keeps its own drag — the
// gesture arena hands a drag to the deepest handler whose axis matches, and the
// Canvas is deeper than the Scroll — so painting a pattern doesn't scroll the
// page out from under the finger.
func (s *lum) stacked(ctx widget.Ctx, th theme.Theme) widget.Widget {
	return widget.Scroll{Child: widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children: []widget.Widget{
			widget.AspectRatio{Ratio: 1, Child: s.matrix()},
			widget.Sized{H: pagePad},
			s.panel(ctx, th),
		},
	}}
}

func (s *lum) matrix() widget.Widget {
	return widget.Interactive{
		Gestures: widget.Gestures{
			OnKey:      s.onKey,
			OnPress:    s.onPress,
			OnDrag:     s.onDrag,
			DragAxis:   widget.DragAny,
			OnPressEnd: func() { s.painting = false },
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
}

func (s *lum) panel(ctx widget.Ctx, th theme.Theme) widget.Widget {
	playLabel := "Play"
	if s.playing {
		playLabel = "Pause"
	}
	return widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children: []widget.Widget{
			widget.Text{Value: "Luminaria", Font: theme.FontBold, Size: th.Type.Title, Color: th.Text},
			widget.Sized{H: 4},
			widget.Text{Value: "Tap a cell to cycle it: node, turn right, turn left, empty. Drag to paint.",
				Size: th.Type.Caption, Color: th.Muted, Wrap: true},
			widget.Sized{H: 16},

			theme.Button{Label: playLabel, Primary: true,
				OnTap: func() { s.SetState(func() { s.playing = !s.playing }) }},

			s.slider(th, "Tempo", fmt.Sprintf("%.0f BPM", s.bpm),
				float32((s.bpm-minBPM)/(maxBPM-minBPM)),
				func(v float32) { s.bpm = minBPM + float64(v)*(maxBPM-minBPM) }),

			s.group(th, "Scale"),
			theme.Dropdown{Options: scaleNames(), Selected: s.scale,
				OnChange: func(i int) { s.SetState(func() { s.scale = i }) }},

			s.group(th, "Voice"),
			theme.Segmented{Options: voiceNames, Selected: int(s.voice),
				OnChange: func(i int) { s.SetState(func() { s.voice = Voice(i) }) }},

			s.group(th, "Register"),
			theme.Segmented{Options: octaveNames, Selected: s.octave,
				OnChange: func(i int) { s.SetState(func() { s.octave = i }) }},

			s.slider(th, "Sustain", fmt.Sprintf("%.1fs", s.sustain), float32((s.sustain-0.2)/3.3),
				func(v float32) { s.sustain = 0.2 + float64(v)*3.3 }),

			s.slider(th, "Echo", fmt.Sprintf("%d%%", int(s.echo*100+0.5)), s.echo,
				func(v float32) { s.echo = v }),

			s.group(th, fmt.Sprintf("Crawlers — %d", len(s.crawlers))),
			widget.Row(
				widget.Expand(theme.Button{Label: "Remove", OnTap: func() { s.SetState(s.removeCrawler) }}),
				widget.Sized{W: 8},
				widget.Expand(theme.Button{Label: "Add", OnTap: func() { s.SetState(s.addCrawler) }}),
			),
			widget.Sized{H: 16},
			widget.Row(
				widget.Expand(theme.Button{Label: "Clear", OnTap: s.clear}),
				widget.Sized{W: 8},
				widget.Expand(theme.Button{Label: "Reseed", OnTap: func() { s.SetState(s.seed) }}),
			),
			widget.Sized{H: 12},
			widget.Text{Value: "Space play · A crawler · C clear · R reseed",
				Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		},
	}
}

// group is a section label with the spacing above it that separates controls.
func (s *lum) group(th theme.Theme, label string) widget.Widget {
	return widget.Padding{Insets: geom.Insets{Top: 18, Bottom: 6},
		Child: widget.Align{X: 0, Y: 0.5,
			Child: widget.Text{Value: label, Size: th.Type.Label, Color: th.Muted}}}
}

// slider is a labelled slider with its value echoed on the right — the shape
// every parameter in the panel takes.
func (s *lum) slider(th theme.Theme, label, value string, v float32, set func(float32)) widget.Widget {
	return widget.Flex{
		Axis:       layout.Vertical,
		CrossAlign: layout.CrossStretch,
		Children: []widget.Widget{
			widget.Padding{Insets: geom.Insets{Top: 18, Bottom: 6}, Child: widget.Row(
				widget.Expand(widget.Text{Value: label, Size: th.Type.Label, Color: th.Muted}),
				widget.Text{Value: value, Font: theme.FontBold, Size: th.Type.Label, Color: th.Primary},
			)},
			theme.Slider{Value: v, Label: label, OnChange: func(x float32) { s.SetState(func() { set(x) }) }},
		},
	}
}

func main() {
	// Audio is best-effort: if no device opens, the matrix still runs, silent.
	mixer := sound.NewMixer()
	if closer, err := device.Open(mixer); err != nil {
		log.Printf("audio disabled: %v", err)
	} else {
		defer closer.Close()
	}

	if err := app.Run(App{Mixer: mixer}, app.Config{
		Title:          "Luminaria",
		AppID:          "com.gophics.luminaria",
		Size:           geom.Size{W: 1040, H: 720},
		Background:     bg,
		BackgroundDark: bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}); err != nil {
		log.Fatal(err)
	}
}
