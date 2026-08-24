package chart

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A chart inside a scroller must not swallow the scroll.
//
// Selecting on OnPress with the default DragAny meant a finger landing on a
// chart selected a datum immediately and then claimed every subsequent move,
// whichever way it went. The page under it did not scroll, and the tooltip and
// crosshair the press had drawn stayed on screen after the gesture — on a
// phone, painted over whatever the chart had been scrolled beneath.
//
// Selection is a scrub along x, so the chart claims horizontal drags only, and
// commits the selection on tap rather than on press: crossing the tap slop
// cancels the pending tap, so a scroll leaves nothing behind.
func scrolledChart(t *testing.T) (*app.Headless, *chartState, *widget.ScrollController) {
	t.Helper()
	var st *chartState
	stateHook = func(s *chartState) { st = s }
	defer func() { stateHook = nil }()

	ctrl := &widget.ScrollController{}
	root := widget.Scroll{
		Axis:       layout.Vertical,
		Controller: ctrl,
		Child: widget.Flex{Axis: layout.Vertical, Children: []widget.Widget{
			widget.Sized{H: 400, Child: Chart{
				Marks: []Mark{BarMark{Data: Values("a", 3, "b", 7, "c", 2, "d", 5)}},
			}},
			// Enough content below to leave room to scroll.
			widget.Sized{H: 1200},
		}},
	}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 420, H: 300}, Background: paint.RGB(1, 1, 1),
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if st == nil {
		t.Fatal("chart state not mounted")
	}
	return h, st, ctrl
}

func TestVerticalDragOnAChartScrollsAndSelectsNothing(t *testing.T) {
	h, st, ctrl := scrolledChart(t)

	h.Drag(geom.Pt{X: 210, Y: 250}, geom.Pt{X: 210, Y: 60})
	h.Render()

	if st.sel >= 0 {
		t.Errorf("a vertical drag selected datum %d; it should have scrolled and selected nothing", st.sel)
	}
	if ctrl.Offset() <= 0 {
		t.Errorf("scroll offset is %v; the chart swallowed the vertical drag", ctrl.Offset())
	}
}

func TestHorizontalDragOnAChartScrubsAndDoesNotScroll(t *testing.T) {
	h, st, ctrl := scrolledChart(t)

	h.Drag(geom.Pt{X: 40, Y: 200}, geom.Pt{X: 395, Y: 200})
	h.Render()

	if st.sel < 0 {
		t.Error("a horizontal drag across the plot selected nothing; scrubbing is the chart's gesture")
	}
	if ctrl.Offset() != 0 {
		t.Errorf("scroll offset is %v; a horizontal scrub must not scroll the page", ctrl.Offset())
	}
}
