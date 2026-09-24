package theme

import (
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
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
}

// tabH is the height of the label row; underlineH is the indicator strip below.
const (
	tabH       = 40
	underlineH = 3
)

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
	//
	// Each tab is its own Interactive rather than one hit area over the row
	// that maps a press x to an index. The index came only from the pointer,
	// so assistive activation — which has no pointer — always reported the
	// last pressed tab, and a screen reader saw one unlabeled button rather
	// than tabs with a selected one among them.
	cells := make([]widget.Widget, n)
	for i, label := range tb.Tabs {
		col, font := th.Muted, ""
		if i == sel {
			col, font = th.Primary, FontBold
		}
		cells[i] = widget.Expand(widget.Interactive{
			Sem: &layout.SemInfo{Role: layout.RoleTab, Label: label, Selected: i == sel},
			Gestures: widget.Gestures{OnTap: func() {
				if f := tb.OnChange; f != nil && i != tb.Selected {
					haptic(ctx, shell.HapticSelection)
					f(i)
				}
			}},
			Child: widget.Sized{H: tabH, Child: widget.Center(
				widget.Text{Value: label, Font: font, Size: th.Type.Label, Color: col},
			)},
		})
	}
	labels := widget.Row(cells...)

	bar := func(pos float32) widget.Widget {
		underline := widget.Canvas{H: underlineH, Draw: func(c paint.Canvas, size geom.Size) {
			c.FillRect(geom.RectXYWH(0, size.H-1, size.W, 1), th.Border) // hairline divider
			segW := size.W / float32(n)
			inset := segW * 0.18 // a bar a touch narrower than the tab
			c.FillRRect(geom.RectXYWH(pos*segW+inset, 0, segW-2*inset, size.H), size.H/2, th.Primary)
		}}
		return widget.Column(labels, underline)
	}
	// The underline jumps under reduce motion; otherwise AnimateFloat tweens
	// its index toward the selection so the accent bar slides between tabs.
	if ctx.ReduceMotion() {
		return bar(float32(sel))
	}
	return widget.AnimateFloat(float32(sel), 180*time.Millisecond, bar)
}
