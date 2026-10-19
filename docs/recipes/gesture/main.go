// Command gesture is the cookbook's gesture recipe: one Interactive wrapping
// one Canvas, reporting every pointer and keyboard callback gophics offers.
//
//	go run ./docs/recipes/gesture
//
// Interactive adds no visuals and takes its child's size; everything you see
// here is painted by the Canvas under it. That separation is deliberate — it
// is why a gesture can be put on anything, including a chart or a game board.
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Pad is a drag surface with a dot on it.
type Pad struct{}

func (Pad) CreateState() widget.State { return &padState{} }

type padState struct {
	widget.StateBase[Pad]
	at      geom.Pt // the dot, in the pad's own coordinates
	hovered bool
	last    string // the most recent callback, for the readout
}

func (s *padState) Init(widget.Ctx) { s.at = geom.Pt{X: 120, Y: 90} }

func (s *padState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)

	// Positions in every callback are local to the widget, so the dot can be
	// stored exactly as it arrives — no walking up the tree to subtract
	// parents' offsets.
	pad := widget.Interactive{
		Gestures: widget.Gestures{
			// Cursor is a request, not a command: the innermost widget under
			// the pointer wins, and on a platform with no pointer to shape
			// (mobile, the terminal) ctx.Cursor() is nil and nothing happens.
			Cursor: shell.CursorCrosshair,

			OnPress: func(pos geom.Pt) {
				s.SetState(func() { s.at, s.last = pos, "OnPress" })
			},
			// A drag past the tap slop cancels the pending tap, so these two
			// do not both fire for one gesture.
			OnDrag: func(pos, delta geom.Pt) {
				s.SetState(func() { s.at, s.last = pos, "OnDrag" })
			},
			OnTap:       func() { s.SetState(func() { s.last = "OnTap" }) },
			OnDoubleTap: func() { s.SetState(func() { s.last = "OnDoubleTap" }) },
			OnLongPress: func() { s.SetState(func() { s.last = "OnLongPress" }) },

			OnEnter: func() { s.SetState(func() { s.hovered, s.last = true, "OnEnter" }) },
			OnExit:  func() { s.SetState(func() { s.hovered, s.last = false, "OnExit" }) },

			// Setting OnKey (without OnText) makes the widget focusable and
			// claims focus when nothing else holds it, so the arrows work from
			// the first frame with no Autofocus and no tap.
			OnKey: func(k shell.Key) {
				if k.Kind != shell.KeyPress {
					return
				}
				const step = 8
				d, name := geom.Pt{}, ""
				switch k.Code {
				case shell.KeyLeft:
					d, name = geom.Pt{X: -step}, "KeyLeft"
				case shell.KeyRight:
					d, name = geom.Pt{X: step}, "KeyRight"
				case shell.KeyUp:
					d, name = geom.Pt{Y: -step}, "KeyUp"
				case shell.KeyDown:
					d, name = geom.Pt{Y: step}, "KeyDown"
				default:
					return
				}
				s.SetState(func() { s.at, s.last = s.at.Add(d), name })
			},
		},
		Child: widget.Canvas{H: 220, Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
			fill := th.Surface
			if s.hovered {
				fill = th.SurfaceHover
			}
			c.FillRRect(geom.Rect{Max: size.Pt()}, 12, fill)
			c.FillRRect(geom.RectXYWH(s.at.X-9, s.at.Y-9, 18, 18), 9, th.Primary)
		}},
	}

	col := widget.Column(
		theme.Label("Drag, tap, double-tap, hold, or use the arrow keys"),
		widget.Sized{H: 10},
		pad,
		widget.Sized{H: 10},
		widget.Text{
			Value: fmt.Sprintf("last: %s   at: %.0f, %.0f", s.last, s.at.X, s.at.Y),
			Size:  th.Type.Caption,
			Color: th.Muted,
		},
	)
	col.CrossAlign = layout.CrossStretch

	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 16, Child: col}}}
}

func main() {
	if err := app.Run(Pad{}, config()); err != nil {
		log.Fatal(err)
	}
}

func config() app.Config {
	return app.Config{
		Title:          "recipe · gestures",
		Size:           geom.Size{W: 420, H: 360},
		Background:     theme.Light().Bg,
		BackgroundDark: theme.Dark().Bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}
}
