package trace

import (
	"fmt"
	"image"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Scene is the reference scrollable: a tall list in a phone-sized viewport.
// Fixed so that two runs — or a run and a native twin built to the same
// dimensions — are comparing physics and not layout.
type Scene struct {
	Viewport geom.Size
	Rows     int
	RowH     float32
}

// DefaultScene is a 390×844 viewport (an iPhone 14) over 300 rows of 44px:
// 13,200px of content, enough that a hard flick never reaches an edge, so
// the decay is measured and not the bounce.
var DefaultScene = Scene{Viewport: geom.Size{W: 390, H: 844}, Rows: 300, RowH: 44}

// ReplayOptions configure one run.
type ReplayOptions struct {
	Hz float64 // frame rate; 0 means 120
	// Scale is the render scale for frames; 0 means 1. Only matters when
	// Frames is set.
	Scale float32
	// Frames receives each rendered frame, when non-nil.
	Frames func(i int, t float64, img image.Image)
	// MaxT bounds the run in seconds after release; 0 means 10.
	MaxT float64
	// Scene overrides DefaultScene when Rows > 0.
	Scene Scene
}

// Replay drives the finger phase in input through app.Headless at opts.Hz and
// returns the trace gophics produced: the same Input, gophics's Offset per
// frame, and the release time.
//
// Frames are stepped on a fixed clock, and input events are delivered before
// the first frame whose time is at or past theirs — which is exactly how a
// real shell delivers them, in a burst at the top of the frame. That
// alignment is why the harness reproduces the coalesced-event behavior a
// wall-clock test never could.
func Replay(input []Sample, opts ReplayOptions) (*Trace, error) {
	hz := opts.Hz
	if hz <= 0 {
		hz = 120
	}
	scale := opts.Scale
	if scale <= 0 {
		scale = 1
	}
	maxT := opts.MaxT
	if maxT <= 0 {
		maxT = 10
	}
	sc := opts.Scene
	if sc.Rows == 0 {
		sc = DefaultScene
	}

	ctrl := &widget.ScrollController{}
	h, err := app.NewHeadless(list{ctrl: ctrl, scene: sc}, app.Config{
		Size: sc.Viewport,
		Font: goregular.TTF,
	}, scale)
	if err != nil {
		return nil, err
	}
	h.Render() // mount

	out := &Trace{Source: "gophics", Hz: hz, Input: input}
	dt := 1 / hz
	// Start the finger two-thirds down the viewport, so an upward flick has
	// room and the start point is inside the list.
	pos := geom.Pt{X: sc.Viewport.W / 2, Y: sc.Viewport.H * 2 / 3}
	h.TouchPress(pos)
	out.Offset = append(out.Offset, Sample{T: 0, V: float64(ctrl.Offset())})

	next := 0
	released := false
	frame := 0
	quiet := 0
	for {
		frame++
		t := float64(frame) * dt
		// Deliver every input event due by this frame.
		for next < len(input) && input[next].T <= t {
			pos.Y += float32(input[next].V)
			h.TouchMove(pos)
			next++
		}
		if !released && next == len(input) {
			h.TouchRelease(pos)
			released = true
			out.ReleaseT = t
		}
		animating := h.Step(dt)
		if opts.Frames != nil {
			opts.Frames(frame, t, h.Render())
		}
		off := float64(ctrl.Offset())
		out.Offset = append(out.Offset, Sample{T: t, V: off})

		if released {
			if !animating && out.Offset[len(out.Offset)-2].V == off {
				quiet++
			} else {
				quiet = 0
			}
			// A few quiet frames past the end so the tail is in the record.
			if quiet >= 6 || t-out.ReleaseT > maxT {
				break
			}
		}
	}
	return out, nil
}

// list is the reference widget: a scroll over numbered, alternately-shaded
// rows. Shading makes motion legible in a video; numbers make a frame
// locatable.
type list struct {
	ctrl  *widget.ScrollController
	scene Scene
}

func (l list) Build(ctx widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, l.scene.Rows)
	for i := range rows {
		bg := paint.Color{R: 0.96, G: 0.96, B: 0.97, A: 1}
		if i%2 == 1 {
			bg = paint.Color{R: 0.90, G: 0.91, B: 0.94, A: 1}
		}
		// Explicit width: a row that sizes to its text is a narrow strip in a
		// centred column, and the pointer beside it hits nothing — so nothing
		// scrolls. The scroll-position tests learned this first.
		rows[i] = widget.Sized{W: l.scene.Viewport.W, H: l.scene.RowH, Child: widget.Decorated{Color: bg,
			Child: widget.Padding{Insets: geom.InsetsSymmetric(16, 12),
				Child: widget.Text{Value: fmt.Sprintf("Row %d", i), Size: 16,
					Color: paint.Color{R: 0.1, G: 0.1, B: 0.12, A: 1}}}}}
	}
	col := widget.Column(rows...)
	return widget.Fill{Color: paint.Color{R: 1, G: 1, B: 1, A: 1},
		Child: widget.Scroll{Controller: l.ctrl, Child: col}}
}
