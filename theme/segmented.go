package theme

import (
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Segmented is a horizontal pill of 2–5 mutually-exclusive options: one filled
// accent indicator that slides to the selected segment, with the labels sitting
// on top. Controlled — Selected is the source of truth and OnChange reports the
// requested index. It fills the available width and divides it evenly, so drop
// it into a bounded row (like Slider). Themed.
type Segmented struct {
	Options  []string
	Selected int
	OnChange func(int)
}

func (sg Segmented) CreateState() widget.State { return &segmentedState{} }

type segmentedState struct {
	widget.StateBase[Segmented]
}

// segH is the control's height; its half is the pill corner radius.
const segH = 32

func (s *segmentedState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	sg := s.W()
	n := len(sg.Options)
	if n == 0 {
		return widget.Sized{}
	}
	sel := max(sg.Selected, 0)
	if sel >= n {
		sel = n - 1
	}

	// Even-width label cells, each vertically centered in the pill. The selected
	// label swaps to the on-accent color (and bold) so it reads on the indicator.
	//
	// Each option is its own Interactive, for the reason Tabs gives: the
	// index of a press came from the pointer's x, so assistive activation
	// had no way to pick an option and nothing announced which was chosen.
	// The options are radios — one of a mutually exclusive set — and the
	// checked one says so.
	cells := make([]widget.Widget, n)
	for i, label := range sg.Options {
		col, font := th.Text, ""
		if i == sel {
			col, font = th.OnPrimary, FontBold
		}
		checked := i == sel
		cells[i] = widget.Expand(widget.Interactive{
			Sem: &layout.SemInfo{Role: layout.RoleRadio, Label: label, Checked: &checked},
			Gestures: widget.Gestures{OnTap: func() {
				if f := sg.OnChange; f != nil && i != sg.Selected {
					haptic(ctx, shell.HapticSelection)
					f(i)
				}
			}},
			Child: widget.Sized{H: segH, Child: widget.Center(
				widget.Text{Value: label, Font: font, Size: th.Type.Label, Color: col},
			)},
		})
	}
	labels := widget.Row(cells...)

	// The Canvas draws the track and the pill under the labels from the
	// indicator's (possibly interpolated) position.
	pill := func(pos float32) widget.Widget {
		indicator := widget.Canvas{H: segH, Draw: func(c paint.Canvas, size geom.Size) {
			r := geom.Rect{Max: size.Pt()}
			c.FillRRect(r, size.H/2, th.Outline) // track groove
			segW := size.W / float32(n)
			const inset = 2
			ir := geom.RectXYWH(pos*segW+inset, inset, segW-2*inset, size.H-2*inset)
			c.FillRRect(ir, (size.H-2*inset)/2, th.Primary)
		}}
		return widget.Stack{Children: []widget.Widget{indicator, labels}}
	}
	// The pill jumps under reduce motion; otherwise AnimateFloat tweens its
	// index toward the selection so it slides between segments.
	if ctx.ReduceMotion() {
		return pill(float32(sel))
	}
	return widget.AnimateFloat(float32(sel), 180*time.Millisecond, pill)
}
