package theme

import (
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Tabs is a tab bar: a row of labels over a hairline divider, with an accent
// underline that slides to the active tab. Controlled — Selected is the source
// of truth and OnChange reports the requested index; the caller swaps the panel
// content in response. It fills the available width and divides it evenly.
// Themed.
type Tabs struct {
	Tabs     []string
	Selected int
	OnChange func(int)
}

func (t Tabs) CreateState() widget.State { return &tabsState{} }

type tabsState struct {
	widget.StateBase[Tabs]
	width   float32 // captured at paint, maps a tap x → tab index
	pressed int     // tab under the last pointer-down, committed on tap
}

// tabH is the height of the label row; underlineH is the indicator strip below.
const (
	tabH       = 40
	underlineH = 3
)

// indexAt maps a local x to a tab index, clamped to the valid range.
func (s *tabsState) indexAt(x float32) int {
	n := len(s.W().Tabs)
	if n == 0 || s.width <= 0 {
		return -1
	}
	i := max(int(x/(s.width/float32(n))), 0)
	if i >= n {
		i = n - 1
	}
	return i
}

func (s *tabsState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	tb := s.W()
	n := len(tb.Tabs)
	if n == 0 {
		return widget.Sized{}
	}
	sel := max(tb.Selected, 0)
	if sel >= n {
		sel = n - 1
	}

	// Even-width label cells; the active tab is accent-colored and bold, the
	// rest muted — the underline reinforces which one is live.
	cells := make([]widget.Widget, n)
	for i, label := range tb.Tabs {
		col, font := th.Muted, ""
		if i == sel {
			col, font = th.Primary, FontBold
		}
		cells[i] = widget.Expand(widget.Sized{H: tabH, Child: widget.Center(
			widget.Text{S: label, Font: font, Size: th.Type.Label, Color: col},
		)})
	}
	labels := widget.Row(cells...)

	return widget.Interactive{
		Gestures: widget.Gestures{
			OnPress: func(p geom.Pt) { s.pressed = s.indexAt(p.X) },
			OnTap: func() {
				if f := tb.OnChange; f != nil && s.pressed >= 0 && s.pressed != tb.Selected {
					haptic(ctx, shell.HapticSelection)
					f(s.pressed)
				}
			},
		},
		// AnimateFloat tweens the underline's index toward the selection, so the
		// accent bar slides between tabs over the hairline divider.
		Child: widget.AnimateFloat(float32(sel), 180*time.Millisecond, func(pos float32) widget.Widget {
			underline := widget.Canvas{H: underlineH, Draw: func(c paint.Canvas, size geom.Size) {
				s.width = size.W
				c.FillRect(geom.RectXYWH(0, size.H-1, size.W, 1), th.Border) // hairline divider
				segW := size.W / float32(n)
				inset := segW * 0.18 // a bar a touch narrower than the tab
				c.FillRRect(geom.RectXYWH(pos*segW+inset, 0, segW-2*inset, size.H), size.H/2, th.Primary)
			}}
			return widget.Column(labels, underline)
		}),
	}
}
