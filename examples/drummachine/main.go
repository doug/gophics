// Command drummachine is a 16-step drum sequencer that live-synthesizes its own
// kick/snare/clap/hi-hat voices (no audio assets) and mixes them through the
// pure-Go audio engine — CoreAudio/WASAPI/PulseAudio via sound/device. Tap the
// grid to program a beat, Space to start/stop, and adjust the tempo. It is the
// driver example for real-time audio output: a tight rhythmic clock scheduling
// polyphonic one-shots, versus the fire-and-forget sfx the games use.
//
//	go run ./examples/drummachine
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/sound/device"
	"github.com/doug/gophics/widget"
)

const (
	steps     = 16
	numVoices = 5
	minBPM    = 40
	maxBPM    = 240
)

var (
	bg       = paint.RGB(0.11, 0.12, 0.14)
	cellOff  = paint.RGB(0.19, 0.20, 0.24)
	cellBeat = paint.RGB(0.24, 0.25, 0.30) // downbeat columns, a touch lighter
	labelCol = paint.RGB(0.72, 0.75, 0.80)
	titleCol = paint.RGB(0.96, 0.97, 0.99)
	subCol   = paint.RGB(0.52, 0.55, 0.62)
	headCol  = paint.Color{R: 1, G: 1, B: 1, A: 0.10} // playhead column wash
	btnBg    = paint.RGB(0.22, 0.23, 0.28)
	btnFg    = paint.RGB(0.91, 0.93, 0.96)
	stopBg   = paint.RGB(0.86, 0.30, 0.34) // Play button while playing (a Stop)
	playBg   = paint.RGB(0.22, 0.62, 0.40) // Play button while stopped
)

// voiceCol tints each voice's active steps.
var voiceCol = [numVoices]paint.Color{
	paint.RGB(0.95, 0.36, 0.36), // kick
	paint.RGB(0.97, 0.62, 0.26), // snare
	paint.RGB(0.96, 0.82, 0.32), // clap
	paint.RGB(0.32, 0.80, 0.72), // closed hat
	paint.RGB(0.44, 0.66, 0.98), // open hat
}

type App struct{ Mixer *sound.Mixer }

func (App) CreateState() widget.State { return &drum{} }

type drum struct {
	widget.StateBase[App]
	mixer   *sound.Mixer
	voices  []voice
	grid    [numVoices][steps]bool
	bpm     float64
	playing bool
	step    int     // current playhead step
	acc     float64 // seconds accumulated toward the next step

	ctx     widget.Ctx
	cell    [numVoices][steps]geom.Rect
	playBtn geom.Rect
	tempoDn geom.Rect
	tempoUp geom.Rect
	clrBtn  geom.Rect
}

// stateHook, if set, receives the state on mount — for tests to drive input.
var stateHook func(*drum)

func (s *drum) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.mixer = s.W().Mixer
	s.voices = kit()
	s.bpm = 120
	s.loadDefaultPattern()
	s.start()
	ctx.AddTicker(s)
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *drum) loadDefaultPattern() {
	s.grid = [numVoices][steps]bool{}
	set := func(v int, at ...int) {
		for _, st := range at {
			s.grid[v][st] = true
		}
	}
	set(0, 0, 8)                      // kick on the ones
	set(1, 4, 12)                     // snare backbeat
	set(2, 4, 12)                     // clap doubles the snare
	set(3, 0, 2, 4, 6, 8, 10, 12, 14) // closed hats on eighths
	set(4, 14)                        // open hat pickup
}

// start (re)starts playback from step 0 and sounds the downbeat.
func (s *drum) start() {
	s.playing = true
	s.step = 0
	s.acc = 0
	s.trigger(0)
}

func (s *drum) togglePlay() {
	if s.playing {
		s.playing = false
	} else {
		s.start()
	}
	s.ctx.Invalidate()
}

// trigger sounds every voice active on the given step.
func (s *drum) trigger(step int) {
	if s.mixer == nil {
		return
	}
	for v := range s.voices {
		if s.grid[v][step] {
			s.mixer.Play(s.voices[v].sample, sound.PlayOptions{Volume: s.voices[v].vol})
		}
	}
}

// stepDur is the seconds per 16th-note step at the current tempo.
func (s *drum) stepDur() float64 { return 60.0 / s.bpm / 4 }

// Tick advances the rhythmic clock, firing each step it crosses.
func (s *drum) Tick(dt float64) bool {
	if !s.playing {
		return false
	}
	dur := s.stepDur()
	s.acc += dt
	for s.acc >= dur {
		s.acc -= dur
		s.step = (s.step + 1) % steps
		s.trigger(s.step)
	}
	s.ctx.Invalidate()
	return true
}

func (s *drum) setTempo(delta float64) {
	s.bpm = clampf(s.bpm+delta, minBPM, maxBPM)
	s.ctx.Invalidate()
}

func (s *drum) clear() {
	s.grid = [numVoices][steps]bool{}
	s.ctx.Invalidate()
}

// toggleCell flips a step and, when turning it on, previews the voice.
func (s *drum) toggleCell(v, st int) {
	s.grid[v][st] = !s.grid[v][st]
	if s.grid[v][st] && s.mixer != nil {
		s.mixer.Play(s.voices[v].sample, sound.PlayOptions{Volume: s.voices[v].vol})
	}
	s.ctx.Invalidate()
}

func (s *drum) onPress(p geom.Pt) {
	for v := 0; v < numVoices; v++ {
		for st := 0; st < steps; st++ {
			if s.cell[v][st].Contains(p) {
				s.toggleCell(v, st)
				return
			}
		}
	}
	switch {
	case s.playBtn.Contains(p):
		s.togglePlay()
	case s.tempoDn.Contains(p):
		s.setTempo(-5)
	case s.tempoUp.Contains(p):
		s.setTempo(5)
	case s.clrBtn.Contains(p):
		s.clear()
	}
}

func (s *drum) Build(_ widget.Ctx) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{
			OnKey: func(k shell.Key) {
				if k.Kind != shell.KeyPress {
					return
				}
				switch k.Code {
				case shell.KeySpace:
					s.togglePlay()
				case shell.KeyUp:
					s.setTempo(5)
				case shell.KeyDown:
					s.setTempo(-5)
				}
			},
			OnPress: func(p geom.Pt) { s.onPress(p) },
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
}

func (s *drum) draw(c paint.Canvas, sz geom.Size) {
	c.Clear(bg)
	const pad = 22
	c.TextIn("", "Drum Machine", geom.Pt{X: pad, Y: 44}, 26, titleCol)
	c.TextIn("", fmt.Sprintf("%.0f BPM", s.bpm), geom.Pt{X: sz.W - pad - 74, Y: 44}, 18, subCol)

	const (
		labelW  = 84
		cellGap = 5
		beatGap = 10 // extra space between groups of four
		rowH    = 46
		bh      = 40 // transport button height
	)
	// Center the grid + transport block vertically in the space under the title,
	// so it sits well both in the app window and the (taller) gallery card.
	blockH := float32(numVoices*rowH + 10 + bh)
	gridTop := 64 + (sz.H-64-blockH)/2
	if gridTop < 64 {
		gridTop = 64
	}
	gridLeft := float32(pad + labelW)
	gridRight := sz.W - pad
	availW := gridRight - gridLeft
	cellW := (availW - 15*cellGap - 3*beatGap) / 16
	if cellW < 12 {
		return
	}
	cellH := float32(rowH - 12)
	xOf := func(st int) float32 {
		return gridLeft + float32(st)*(cellW+cellGap) + float32(st/4)*beatGap
	}
	yOf := func(v int) float32 { return gridTop + float32(v)*rowH }

	// Playhead column wash behind the active step.
	if s.playing {
		x := xOf(s.step)
		c.FillRRect(geom.RectXYWH(x-2, gridTop-4, cellW+4, numVoices*rowH-2), 5, headCol)
	}

	for v := 0; v < numVoices; v++ {
		c.TextIn("", s.voices[v].name, geom.Pt{X: pad, Y: yOf(v) + cellH*0.72}, 14, labelCol)
		for st := 0; st < steps; st++ {
			r := geom.RectXYWH(xOf(st), yOf(v), cellW, cellH)
			s.cell[v][st] = r
			switch {
			case s.grid[v][st]:
				col := voiceCol[v]
				if s.playing && st == s.step {
					col = lighten(col, 0.25) // flash the playing step
				}
				c.FillRRect(r, 5, col)
			case st%4 == 0:
				c.FillRRect(r, 5, cellBeat)
			default:
				c.FillRRect(r, 5, cellOff)
			}
		}
	}

	// Transport row.
	ty := gridTop + float32(numVoices*rowH+10)
	s.playBtn = geom.RectXYWH(gridLeft, ty, 96, bh)
	pbg, plabel := playBg, "Play"
	if s.playing {
		pbg, plabel = stopBg, "Stop"
	}
	s.button(c, s.playBtn, plabel, pbg, paint.RGB(1, 1, 1))

	s.tempoDn = geom.RectXYWH(gridLeft+110, ty, 40, bh)
	s.tempoUp = geom.RectXYWH(gridLeft+214, ty, 40, bh)
	s.button(c, s.tempoDn, "–", btnBg, btnFg)
	s.button(c, s.tempoUp, "+", btnBg, btnFg)
	c.TextIn("", "tempo", geom.Pt{X: gridLeft + 160, Y: ty + bh/2 + 5}, 14, subCol)

	s.clrBtn = geom.RectXYWH(gridRight-96, ty, 96, bh)
	s.button(c, s.clrBtn, "Clear", btnBg, btnFg)
}

func (s *drum) button(c paint.Canvas, r geom.Rect, label string, bgc, fg paint.Color) {
	c.FillRRect(r, 8, bgc)
	w := s.ctx.Painter().MeasureWidthIn("", label, 15)
	c.TextIn("", label, geom.Pt{X: r.Min.X + (r.Dx()-w)/2, Y: r.Min.Y + r.Dy()/2 + 5}, 15, fg)
}

func lighten(c paint.Color, amt float32) paint.Color {
	return paint.Color{
		R: c.R + (1-c.R)*amt,
		G: c.G + (1-c.G)*amt,
		B: c.B + (1-c.B)*amt,
		A: c.A,
	}
}

func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func main() {
	// Audio is best-effort: if the device won't open, the sequencer runs silent.
	mixer := sound.NewMixer()
	if closer, err := device.Open(mixer); err != nil {
		log.Printf("audio disabled: %v", err)
	} else {
		defer closer.Close()
	}

	if err := app.Run(App{Mixer: mixer}, app.Config{
		Title:      "Drum Machine",
		Size:       geom.Size{W: 760, H: 424},
		Background: bg,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
