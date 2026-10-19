// Command transition is the cookbook's animation recipe. It shows both of the
// two ways gophics animates, side by side, because which one to reach for is
// the only real decision here.
//
//	go run ./docs/recipes/transition
//
// Implicit: set a new Value on an Animated wrapper and it tweens there from
// wherever it currently is. No controller, no ticker, no Dispose. This is what
// a panel expanding, a colour changing or a card lifting should use.
//
// Explicit: hold an *anim.Controller in your state, register it as a ticker,
// and read Value() in Build. Reach for it when the motion is not a tween
// between two states — when it loops, reverses mid-flight, or drives several
// properties off one clock.
//
// A limitation worth knowing before you design around it: there is no
// velocity handoff. A drag released mid-flight cannot pass its speed into the
// animation that continues it, so a flick-to-dismiss will not carry the
// gesture's momentum. The scroll implementation has a real critically-damped
// solver, but it is private to it.
package main

import (
	"log"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

type Demo struct{}

func (Demo) CreateState() widget.State { return &demoState{} }

type demoState struct {
	widget.StateBase[Demo]
	ctx  widget.Ctx
	open bool
	// pulse is the explicit side: a controller the state owns, ticked by the
	// runtime, read in Build.
	pulse *anim.Controller
}

func (s *demoState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.pulse = &anim.Controller{
		Duration: 900 * time.Millisecond,
		Curve:    anim.EaseInOut,
		// OnChange fires every frame the value moves, and once at the end.
		// SetState(nil) means "nothing to mutate, just repaint".
		OnChange: func() { s.SetState(nil) },
	}
	ctx.AddTicker(s.pulse)
	s.pulse.Forward()
}

// Dispose is not optional for a ticker. An element that goes away without
// removing its ticker leaves the runtime calling into dead state, and the app
// never reaches an idle frame.
func (s *demoState) Dispose() { s.ctx.RemoveTicker(s.pulse) }

func (s *demoState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)

	// --- implicit: three tweens driven by one bool -----------------------
	//
	// Each wrapper compares its Value to the one it last saw. Changed, it
	// animates from the current interpolated point — not from the old target —
	// so reversing mid-flight is continuous rather than a jump.
	target, tint := float32(1), th.Surface
	if s.open {
		target, tint = 1.6, th.Primary
	}

	grow := widget.AnimatedScale(target, 220*time.Millisecond,
		swatch(th, "AnimatedScale"))

	recolour := widget.AnimateColor(tint, 220*time.Millisecond, func(c paint.Color) widget.Widget {
		return widget.Decorated{Color: c, Radius: 10,
			Child: widget.Sized{W: 96, H: 56}}
	})

	// A scalar tween is the general case: feed it anything that lerps, here
	// an opacity. Duration 0 means the 150ms default.
	fade := widget.AnimateFloat(boolTo(s.open, 1, 0.25), 0, func(a float32) widget.Widget {
		return widget.Opacity{Alpha: a, Child: swatch(th, "AnimateFloat")}
	})

	// --- explicit: one clock, two properties -----------------------------
	//
	// Value() is in [0,1]. Everything derived from it moves on the same frame,
	// which is the thing two independent implicit tweens cannot promise.
	v := s.pulse.Value()
	pulse := widget.Transform{
		T:      paint.Transform{Rotation: v * 0.5},
		Center: true,
		Child: widget.Decorated{
			Color:  paint.Lerp(th.Surface, th.Warning, v),
			Radius: 10,
			Child:  widget.Sized{W: 96, H: 56},
		},
	}

	row := widget.Row(grow, widget.Sized{W: 14}, recolour, widget.Sized{W: 14}, fade)
	row.CrossAlign = layout.CrossCenter

	col := widget.Column(
		theme.Label("Implicit — one bool, three tweens"),
		widget.Sized{H: 12},
		row,
		widget.Sized{H: 20},
		theme.Button{Label: "Toggle", Primary: true,
			OnTap: func() { s.SetState(func() { s.open = !s.open }) }},
		widget.Sized{H: 24},
		theme.Label("Explicit — one controller, two properties"),
		widget.Sized{H: 12},
		pulse,
		widget.Sized{H: 12},
		theme.Button{Label: "Run again", OnTap: func() {
			// Jump(0) rewinds and repaints; Forward() starts the run.
			s.pulse.Jump(0)
			s.pulse.Forward()
		}},
	)
	col.CrossAlign = layout.CrossCenter

	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 20, Child: widget.Center(col)}}}
}

func swatch(th theme.Theme, label string) widget.Widget {
	return widget.Decorated{
		BorderColor: th.Border, BorderWidth: 1.5, Radius: 10,
		Child: widget.Sized{W: 96, H: 56, Child: widget.Center(
			widget.Text{Value: label, Size: th.Type.Caption, Color: th.Muted})},
	}
}

func boolTo(b bool, yes, no float32) float32 {
	if b {
		return yes
	}
	return no
}

func main() {
	if err := app.Run(Demo{}, config()); err != nil {
		log.Fatal(err)
	}
}

func config() app.Config {
	return app.Config{
		Title:          "recipe · transitions",
		Size:           geom.Size{W: 460, H: 520},
		Background:     theme.Light().Bg,
		BackgroundDark: theme.Dark().Bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}
}
