package chart

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// A chart of marks that have no domains draws no axes and reserves no room
// for them.
//
// SectorMark reports an empty domain on both axes, correctly — a pie is drawn
// from its own values and has nothing to place against a scale. But
// resolveScales must return non-nil scales, so it substitutes 0..1, and that
// invented domain used to be treated as real: margins reserved space for its
// labels and the axis renderer drew its ticks. The result was a column of
// 0, 0.2, 0.4 ... beside the pie — including 0.6000000000000001, the tick
// arithmetic of a domain nobody asked for surfacing in the UI — with the pie
// squeezed into what was left.
func TestAChartWithNoDomainsHasNoAxes(t *testing.T) {
	pie := Chart{Marks: []Mark{SectorMark{Data: Values("a", 3, "b", 7)}}}
	if !axisless(pie) {
		t.Fatal("a chart of only sector marks should have no axes")
	}

	// An explicit scale means the caller wants that axis, whatever the marks say.
	if axisless(Chart{X: NewLinear(0, 10), Marks: pie.Marks}) {
		t.Error("an explicit X scale should keep its axis")
	}

	// And a mark that does carry a domain keeps its axes, so the rule cannot
	// swallow ordinary charts.
	bars := Chart{Marks: []Mark{BarMark{Data: Values("a", 3, "b", 7)}}}
	if axisless(bars) {
		t.Error("a bar chart has domains and must keep its axes")
	}
	if axisless(Chart{}) {
		t.Error("an empty chart is not the axis-less case")
	}
}

// The plot area is the visible half: an axis-less chart must claim no label
// space, or the pie is drawn small and off-centre even with the ticks gone.
// Measured through the real build, since that is where the axes are hidden.
func TestAxislessChartsReserveNoLabelMargin(t *testing.T) {
	areaOf := func(t *testing.T, c Chart) geom.Rect {
		t.Helper()
		var st *chartState
		stateHook = func(s *chartState) { st = s }
		defer func() { stateHook = nil }()
		h, err := app.NewHeadless(c, app.Config{
			Size: geom.Size{W: 300, H: 200}, Background: paint.RGB(1, 1, 1),
			Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
		}, 1)
		if err != nil {
			t.Fatal(err)
		}
		stateHook = nil
		h.Render()
		if st == nil {
			t.Fatal("chart state not mounted")
		}
		return st.area
	}

	pie := Chart{Marks: []Mark{SectorMark{Data: Values("a", 3, "b", 7)}}}
	hidden := pie
	hidden.XAxis.Hide, hidden.YAxis.Hide = true, true

	got, want := areaOf(t, pie), areaOf(t, hidden)
	if got != want {
		t.Errorf("axis-less chart got plot area %v, want the same as an explicitly hidden one %v", got, want)
	}
}
