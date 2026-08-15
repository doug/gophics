package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

// rowApp lays three fixed-width boxes in a Row under a given direction.
type rowApp struct {
	dir      widget.Direction
	noMirror bool
}

func (a rowApp) Build(widget.Ctx) widget.Widget {
	row := widget.Flex{
		Axis:     layout.Horizontal,
		NoMirror: a.noMirror,
		Children: []widget.Widget{
			widget.Semantics{Label: "first", Child: widget.Sized{W: 30, H: 20}},
			widget.Semantics{Label: "second", Child: widget.Sized{W: 40, H: 20}},
			widget.Semantics{Label: "third", Child: widget.Sized{W: 50, H: 20}},
		},
	}
	return widget.Directionality{Dir: a.dir, Child: widget.Align{X: 0, Y: 0, Child: row}}
}

// rectOf finds a labeled node's laid-out rect via the semantics tree, which is
// the supported way to observe geometry from outside the layout package.
func rectOf(t *testing.T, h *app.Headless, label string) geom.Rect {
	t.Helper()
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Label == label {
			return n.Rect
		}
	}
	t.Fatalf("no semantic node labeled %q", label)
	return geom.Rect{}
}

func TestRowMirrorsInRTL(t *testing.T) {
	ltr, err := app.NewHeadless(rowApp{dir: widget.DirLTR}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	ltr.Render()

	rtl, err := app.NewHeadless(rowApp{dir: widget.DirRTL}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	rtl.Render()

	// Left to right: first at 0, then 30, then 70.
	if got := rectOf(t, ltr, "first").Min.X; got != 0 {
		t.Errorf("LTR first.X = %v, want 0", got)
	}
	if got := rectOf(t, ltr, "third").Min.X; got != 70 {
		t.Errorf("LTR third.X = %v, want 70", got)
	}

	// Right to left: the row is 120 wide, so the first child now sits against
	// its right edge (120-30=90) and the last against the left (0).
	if got := rectOf(t, rtl, "first").Min.X; got != 90 {
		t.Errorf("RTL first.X = %v, want 90", got)
	}
	if got := rectOf(t, rtl, "second").Min.X; got != 50 {
		t.Errorf("RTL second.X = %v, want 50", got)
	}
	if got := rectOf(t, rtl, "third").Min.X; got != 0 {
		t.Errorf("RTL third.X = %v, want 0", got)
	}
}

// A row whose order is physical rather than textual must not mirror.
func TestRowNoMirrorOptOut(t *testing.T) {
	h, err := app.NewHeadless(rowApp{dir: widget.DirRTL, noMirror: true}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if got := rectOf(t, h, "first").Min.X; got != 0 {
		t.Errorf("NoMirror first.X = %v, want 0", got)
	}
}

// A Column reads top-to-bottom in every script gophics supports.
type colApp struct{ dir widget.Direction }

func (a colApp) Build(widget.Ctx) widget.Widget {
	col := widget.Column(
		widget.Semantics{Label: "top", Child: widget.Sized{W: 30, H: 20}},
		widget.Semantics{Label: "bottom", Child: widget.Sized{W: 30, H: 20}},
	)
	return widget.Directionality{Dir: a.dir, Child: widget.Align{X: 0, Y: 0, Child: col}}
}

func TestColumnDoesNotMirror(t *testing.T) {
	h, err := app.NewHeadless(colApp{dir: widget.DirRTL}, app.Config{
		Size: geom.Size{W: 200, H: 100}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if got := rectOf(t, h, "top").Min.Y; got != 0 {
		t.Errorf("RTL top.Y = %v, want 0", got)
	}
}

type padApp struct{ dir widget.Direction }

func (a padApp) Build(widget.Ctx) widget.Widget {
	return widget.Directionality{Dir: a.dir, Child: widget.Align{X: 0, Y: 0,
		Child: widget.Padding{
			Start: 16, End: 4,
			Insets: geom.Insets{Top: 2},
			Child:  widget.Semantics{Label: "body", Child: widget.Sized{W: 20, H: 20}},
		},
	}}
}

// Start/End swap with the reading direction; Insets stay literal.
func TestDirectionalPadding(t *testing.T) {
	ltr, err := app.NewHeadless(padApp{dir: widget.DirLTR}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	ltr.Render()
	if got := rectOf(t, ltr, "body").Min.X; got != 16 {
		t.Errorf("LTR body.X = %v, want 16 (Start inset on the left)", got)
	}

	rtl, err := app.NewHeadless(padApp{dir: widget.DirRTL}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	rtl.Render()
	if got := rectOf(t, rtl, "body").Min.X; got != 4 {
		t.Errorf("RTL body.X = %v, want 4 (End inset on the left)", got)
	}
	// The vertical inset is not directional and must be untouched.
	if got := rectOf(t, rtl, "body").Min.Y; got != 2 {
		t.Errorf("RTL body.Y = %v, want 2", got)
	}
}

type alignApp struct {
	dir         widget.Direction
	directional bool
}

func (a alignApp) Build(widget.Ctx) widget.Widget {
	// Align fills the bounded constraints it is given — here the whole
	// 200-wide surface — so the leading edge is the surface's own edge.
	return widget.Directionality{Dir: a.dir,
		Child: widget.Align{X: 0, Y: 0, Directional: a.directional,
			Child: widget.Semantics{Label: "chip", Child: widget.Sized{W: 20, H: 20}}},
	}
}

func TestDirectionalAlign(t *testing.T) {
	// Directional: X=0 is the leading edge, which in RTL is the right.
	rtl, err := app.NewHeadless(alignApp{dir: widget.DirRTL, directional: true}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	rtl.Render()
	if got := rectOf(t, rtl, "chip").Min.X; got != 180 {
		t.Errorf("directional RTL chip.X = %v, want 180 (right edge of a 200pt surface)", got)
	}

	// Not directional: X=0 still means the left edge, in every direction.
	plain, err := app.NewHeadless(alignApp{dir: widget.DirRTL}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	plain.Render()
	if got := rectOf(t, plain, "chip").Min.X; got != 0 {
		t.Errorf("plain RTL chip.X = %v, want 0", got)
	}
}

// With no Directionality installed, everything behaves exactly as before.
func TestDefaultsToLTR(t *testing.T) {
	h, err := app.NewHeadless(bareRowApp{}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if got := rectOf(t, h, "first").Min.X; got != 0 {
		t.Errorf("undeclared direction first.X = %v, want 0", got)
	}
}

type bareRowApp struct{}

func (bareRowApp) Build(widget.Ctx) widget.Widget {
	return widget.Align{X: 0, Y: 0, Child: widget.Row(
		widget.Semantics{Label: "first", Child: widget.Sized{W: 30, H: 20}},
		widget.Semantics{Label: "second", Child: widget.Sized{W: 40, H: 20}},
	)}
}
