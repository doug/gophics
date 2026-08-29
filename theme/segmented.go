package theme

import (
	"time"

	"github.com/doug/gophics/geom"
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
	width   float32 // captured at paint, maps a tap x → segment index
	pressed int     // segment under the last pointer-down, committed on tap
}

// segH is the control's height; its half is the pill corner radius.
const segH = 32

// indexAt maps a local x to a segment index, clamped to the valid range.
func (s *segmentedState) indexAt(x float32) int {
	n := len(s.W().Options)
	if n == 0 || s.width <= 0 {
		return -1
	}
	i := max(int(x/(s.width/float32(n))), 0)
	if i >= n {
		i = n - 1
	}
	return i
}

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
	cells := make([]widget.Widget, n)
	for i, label := range sg.Options {
		col, font := th.Text, ""
		if i == sel {
			col, font = th.OnPrimary, FontBold
		}
		cells[i] = widget.Expand(widget.Sized{H: segH, Child: widget.Center(
			widget.Text{S: label, Font: font, Size: th.Type.Label, Color: col},
		)})
	}
	labels := widget.Row(cells...)

	return widget.Interactive{
		Gestures: widget.Gestures{
			OnPress: func(p geom.Pt) { s.pressed = s.indexAt(p.X) },
			OnTap: func() {
				if f := sg.OnChange; f != nil && s.pressed >= 0 && s.pressed != sg.Selected {
					haptic(ctx, shell.HapticSelection)
					f(s.pressed)
				}
			},
		},
		// AnimateFloat tweens the indicator's index toward the selection, so the
		// accent pill slides between segments; the Canvas draws the track and the
		// pill under the labels each frame from the interpolated position.
		Child: widget.AnimateFloat(float32(sel), 180*time.Millisecond, func(pos float32) widget.Widget {
			indicator := widget.Canvas{H: segH, Draw: func(c paint.Canvas, size geom.Size) {
				s.width = size.W
				r := geom.Rect{Max: size.Pt()}
				c.FillRRect(r, size.H/2, th.Outline) // track groove
				segW := size.W / float32(n)
				const inset = 2
				ir := geom.RectXYWH(pos*segW+inset, inset, segW-2*inset, size.H-2*inset)
				c.FillRRect(ir, (size.H-2*inset)/2, th.Primary)
			}}
			return widget.Stack{Children: []widget.Widget{indicator, labels}}
		}),
	}
}
