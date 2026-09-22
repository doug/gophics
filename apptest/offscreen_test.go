package apptest_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// longList is a page of rows taller than any window in these tests, each row
// a button that records its own tap.
func longList(n int, tapped *[]string) widget.Widget {
	rows := make([]widget.Widget, n)
	for i := range rows {
		label := fmt.Sprintf("Row %d", i)
		rows[i] = theme.Button{Label: label, OnTap: func() { *tapped = append(*tapped, label) }}
	}
	return widget.Scroll{Child: widget.Column(rows...)}
}

// A row below the fold is still in the semantics tree — content exists there
// and a test may say so — but it is marked Offscreen, and ScrollTo is what
// makes it tappable.
func TestOffscreenRowScrollsIntoView(t *testing.T) {
	var tapped []string
	a := apptest.New(t, longList(40, &tapped), apptest.Size(200, 200))

	if !a.HasLabel("Row 30") {
		t.Fatalf("content below the fold must stay discoverable. Labels: %v", a.Labels())
	}
	if a.MustNode("Row 0").Offscreen {
		t.Fatal("the first row is on screen and must not be flagged")
	}
	if !a.MustNode("Row 30").Offscreen {
		t.Fatalf("Row 30 at %v is below a 200px viewport and should be Offscreen", a.MustNode("Row 30").Rect)
	}

	a.ScrollTo("Row 30")
	n := a.MustNode("Row 30")
	if n.Offscreen {
		t.Fatalf("after ScrollTo, Row 30 (rect %v) is still off screen", n.Rect)
	}
	if n.Rect.Min.Y < 0 || n.Rect.Max.Y > 200 {
		t.Fatalf("after ScrollTo, Row 30 should lie wholly inside the viewport, got %v", n.Rect)
	}
	if a.MustNode("Row 0").Offscreen != true {
		t.Error("having scrolled 30 rows down, Row 0 should now be the one off screen")
	}

	a.TapLabel("Row 30")
	if strings.Join(tapped, ",") != "Row 30" {
		t.Fatalf("tap after ScrollTo hit %v, want Row 30", tapped)
	}

	// And back up: ScrollTo works in both directions.
	a.ScrollTo("Row 2")
	a.TapLabel("Row 2")
	if strings.Join(tapped, ",") != "Row 30,Row 2" {
		t.Fatalf("taps: %v", tapped)
	}
}

// A row scrolled three-quarters under the top edge is still tappable — at the
// centre of the quarter that shows, not the centre of its full rect, which is
// under the clip where nothing of it is drawn. Visible says where.
func TestTapLabelLandsOnHalfVisibleRow(t *testing.T) {
	var tapped []string
	a := apptest.New(t, longList(40, &tapped), apptest.Size(200, 200))
	row := a.MustNode("Row 0")
	h := row.Rect.Dy()
	if row.Visible != row.Rect {
		t.Fatalf("a row in full view should be visible in full: rect %v, visible %v", row.Rect, row.Visible)
	}

	a.ScrollAt(geom.Pt{X: 10, Y: 10}, geom.Pt{Y: -0.75 * h}) // over the column, which the viewport shrink-wraps to
	row = a.MustNode("Row 0")
	if row.Offscreen || row.Rect.Min.Y >= 0 || row.Rect.Max.Y <= 0 {
		t.Fatalf("setup: Row 0 should straddle the top edge, got rect %v offscreen=%v", row.Rect, row.Offscreen)
	}
	if row.Visible.Min.Y != 0 || row.Visible.Max.Y != row.Rect.Max.Y {
		t.Fatalf("Visible should be the strip below the edge: rect %v, visible %v", row.Rect, row.Visible)
	}
	if c := row.Rect.Min.Y + h/2; c >= 0 {
		t.Fatalf("setup: the full rect's centre (y=%v) must lie above the edge for this test to mean anything", c)
	}

	a.TapLabel("Row 0")
	if strings.Join(tapped, ",") != "Row 0" {
		t.Fatalf("tapping a half-visible row hit %v, want Row 0", tapped)
	}
}

// A cell far to the right of a horizontal strip that is itself below the fold
// needs both viewports scrolled. The vertical wheel must reach the page and
// not be swallowed by the strip under the pointer.
func TestScrollToThroughNestedViewports(t *testing.T) {
	var tapped []string
	cells := make([]widget.Widget, 20)
	for i := range cells {
		label := fmt.Sprintf("Cell %d", i)
		cells[i] = widget.Sized{W: 80, H: 40, Child: theme.Button{Label: label, OnTap: func() { tapped = append(tapped, label) }}}
	}
	filler := make([]widget.Widget, 12)
	for i := range filler {
		filler[i] = theme.Label(fmt.Sprintf("Paragraph %d", i))
	}
	page := widget.Scroll{Child: widget.Column(append(filler,
		widget.Scroll{Axis: layout.Horizontal, Child: widget.Row(cells...)},
		theme.Label("Footer"),
	)...)}
	a := apptest.New(t, page, apptest.Size(200, 160))

	if !a.MustNode("Cell 15").Offscreen {
		t.Fatalf("Cell 15 at %v should start off screen", a.MustNode("Cell 15").Rect)
	}
	a.ScrollTo("Cell 15")
	n := a.MustNode("Cell 15")
	win := geom.RectXYWH(0, 0, 200, 160)
	inside := n.Rect.Min.X >= win.Min.X && n.Rect.Min.Y >= win.Min.Y &&
		n.Rect.Max.X <= win.Max.X && n.Rect.Max.Y <= win.Max.Y
	if n.Offscreen || !inside {
		t.Fatalf("after ScrollTo, Cell 15 should be wholly inside the window, got %v (offscreen=%v)", n.Rect, n.Offscreen)
	}
	a.TapLabel("Cell 15")
	if strings.Join(tapped, ",") != "Cell 15" {
		t.Fatalf("tap hit %v, want Cell 15", tapped)
	}
}

// recordingTB captures Fatalf so a test can assert that the harness refuses
// something, instead of dying with it. Fatalf must not return, as with the
// real one; a panic recovered by the caller stands in for Goexit.
type recordingTB struct {
	testing.TB
	fatal string
}

type fatalCalled struct{}

func (r *recordingTB) Fatalf(format string, args ...any) {
	r.fatal = fmt.Sprintf(format, args...)
	panic(fatalCalled{})
}

func expectFatal(t *testing.T, r *recordingTB, fn func()) string {
	t.Helper()
	defer func() {
		if rec := recover(); rec != nil {
			if _, ok := rec.(fatalCalled); !ok {
				panic(rec)
			}
		}
	}()
	fn()
	t.Fatal("expected the harness to fail the test, and it did not")
	return ""
}

// Tapping a node the user cannot see used to land the tap on whatever was
// drawn there and carry on; now it is a failure that names the cure.
func TestTapLabelRefusesOffscreenNode(t *testing.T) {
	var tapped []string
	rec := &recordingTB{TB: t}
	a := apptest.New(rec, longList(40, &tapped), apptest.Size(200, 200))

	expectFatal(t, rec, func() { a.TapLabel("Row 30") })
	if !strings.Contains(rec.fatal, "Row 30") || !strings.Contains(rec.fatal, "ScrollTo") {
		t.Fatalf("the failure should name the node and suggest ScrollTo, got: %s", rec.fatal)
	}
	if len(tapped) != 0 {
		t.Fatalf("a refused tap must not be dispatched, but hit %v", tapped)
	}

	rec.fatal = ""
	expectFatal(t, rec, func() { a.TapText("Row 35") })
	if rec.fatal == "" {
		t.Fatal("TapText should refuse an off-screen node the same way")
	}
}

type navHome struct{}

func (navHome) Build(ctx widget.Ctx) widget.Widget {
	nav := ctx.MustOf[widget.Nav]()
	return widget.Column(
		theme.Label("Home"),
		theme.Button{Label: "Open", OnTap: func() { nav.Push(navDetail{}) }},
	)
}

type navDetail struct{}

func (navDetail) Build(ctx widget.Ctx) widget.Widget {
	nav := ctx.MustOf[widget.Nav]()
	return widget.Column(
		theme.Label("Detail"),
		theme.Button{Label: "Back", OnTap: func() { nav.Pop() }},
	)
}

// A pushed page slides in over 220ms. Mid-slide its controls are beyond the
// window's edge — Offscreen, and TapLabel says so — and Settle is what a test
// writes instead of stepping a guessed number of frames.
func TestSettleLandsTapOnPushedPage(t *testing.T) {
	a := apptest.New(t, widget.Navigator{Home: navHome{}}, apptest.Size(200, 200))

	a.TapLabel("Open")
	if n := a.Node("Back"); n != nil && !n.Offscreen {
		t.Logf("first frame of the slide already shows Back at %v; the root-clip check is moot here", n.Rect)
	}
	a.Settle()
	n := a.MustNode("Back")
	if n.Offscreen {
		t.Fatalf("after Settle the pushed page should be in place, but Back is at %v and off screen", n.Rect)
	}
	a.TapLabel("Back")
	a.Settle()
	a.AssertLabel("Open")
	a.AssertNoLabel("Back")
}

// Settle on a tree with nothing animating returns at once, and on one that
// never stops it fails rather than spinning.
func TestSettleBounds(t *testing.T) {
	var tapped []string
	a := apptest.New(t, longList(3, &tapped), apptest.Size(200, 200))
	a.Settle() // nothing running: must not fail

	rec := &recordingTB{TB: t}
	b := apptest.New(rec, forever{}, apptest.Size(50, 50))
	expectFatal(t, rec, b.Settle)
	if !strings.Contains(rec.fatal, "Settle") {
		t.Fatalf("Settle's timeout should say what timed out, got: %s", rec.fatal)
	}
}

// forever is a widget whose ticker never finishes.
type forever struct{}

func (forever) CreateState() widget.State { return &foreverState{} }

type foreverState struct {
	widget.StateBase[forever]
	ctx widget.Ctx
}

func (s *foreverState) Init(ctx widget.Ctx)            { s.ctx = ctx; ctx.AddTicker(s) }
func (s *foreverState) Dispose()                       { s.ctx.RemoveTicker(s) }
func (s *foreverState) Tick(float64) bool              { return true }
func (s *foreverState) Build(widget.Ctx) widget.Widget { return widget.Sized{W: 10, H: 10} }
