package app

import (
	"strings"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
	"golang.org/x/image/font/gofont/goregular"
)

func semLabelsHas(h *Headless, substr string) bool {
	for _, n := range layout.FlattenSemantics(h.Core.Semantics()) {
		if strings.Contains(n.Label, substr) {
			return true
		}
	}
	return false
}

// tallColumn is a stretch of text taller than any viewport.
func tallColumn() widget.Widget {
	var kids []widget.Widget
	for range 40 {
		kids = append(kids, widget.Text{S: "LINE", Size: 14})
	}
	return widget.Column(kids...)
}

func rowStretch(kids ...widget.Widget) widget.Flex {
	r := widget.Row(kids...)
	r.CrossAlign = layout.CrossStretch
	return r
}

func colStretch(kids ...widget.Widget) widget.Flex {
	c := widget.Column(kids...)
	c.CrossAlign = layout.CrossStretch
	return c
}

func mountFlex(t *testing.T, root widget.Widget) *Headless {
	t.Helper()
	h, err := NewHeadless(root, Config{Size: geom.Size{W: 900, H: 640}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

// The scroll content ("LINE") must render in each arrangement. The nested case
// (Row with an Expand'd Scroll beside a fixed Sized, itself inside a Column's
// Expand inside an outer Row's Expand) is the one the notes app hit.
func TestFlexNestedExpandScroll(t *testing.T) {
	scroll := widget.Scroll{Child: tallColumn()}
	side := widget.Sized{W: 200, Child: widget.Text{S: "SIDE"}}

	t.Run("top-level row", func(t *testing.T) {
		h := mountFlex(t, rowStretch(widget.Expand(scroll), side))
		if !semLabelsHas(h, "LINE") {
			t.Errorf("scroll content missing at top level")
		}
	})

	t.Run("nested like notes", func(t *testing.T) {
		inner := rowStretch(widget.Expand(widget.Scroll{Child: tallColumn()}), side)
		pane := colStretch(widget.Sized{H: 40, Child: widget.Text{S: "BAR"}}, widget.Expand(inner))
		root := rowStretch(widget.Sized{W: 240, Child: widget.Text{S: "NAV"}}, widget.Expand(pane))
		h := mountFlex(t, root)
		if !semLabelsHas(h, "LINE") {
			t.Errorf("scroll content missing when nested")
		}
	})

	// The notes outline was itself a Scroll inside the fixed Sized — two Scrolls
	// in one Row. Bisect toward that.
	t.Run("nested, scroll in the side too", func(t *testing.T) {
		scrollSide := widget.Sized{W: 200, Child: widget.Scroll{Child: tallColumn()}}
		inner := rowStretch(widget.Expand(widget.Scroll{Child: tallColumn()}), scrollSide)
		pane := colStretch(widget.Sized{H: 40, Child: widget.Text{S: "BAR"}}, widget.Expand(inner))
		root := rowStretch(widget.Sized{W: 240, Child: widget.Text{S: "NAV"}}, widget.Expand(pane))
		h := mountFlex(t, root)
		if !semLabelsHas(h, "LINE") {
			t.Errorf("scroll content missing with a scroll in the side")
		}
	})

	t.Run("nested, with divider like notes", func(t *testing.T) {
		div := widget.Sized{W: 1, Child: widget.Text{S: "DIV"}}
		scrollSide := widget.Sized{W: 200, Child: widget.Scroll{Child: tallColumn()}}
		inner := rowStretch(widget.Expand(widget.Scroll{Child: tallColumn()}), div, scrollSide)
		pane := colStretch(widget.Sized{H: 40, Child: widget.Text{S: "BAR"}}, widget.Expand(inner))
		root := rowStretch(widget.Sized{W: 240, Child: widget.Text{S: "NAV"}}, widget.Expand(pane))
		h := mountFlex(t, root)
		if !semLabelsHas(h, "LINE") {
			t.Errorf("scroll content missing with divider + scroll side")
		}
	})

	// The exact notes shape: the Row is returned from a LayoutBuilder (which is
	// itself the flex child). This is what actually broke.
	t.Run("nested, row from a LayoutBuilder", func(t *testing.T) {
		body := widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
			return rowStretch(
				widget.Expand(widget.Scroll{Child: tallColumn()}),
				widget.Sized{W: 1, Child: widget.Text{S: "DIV"}},
				widget.Sized{W: 200, Child: widget.Text{S: "SIDE"}},
			)
		}}
		pane := colStretch(widget.Sized{H: 40, Child: widget.Text{S: "BAR"}}, widget.Expand(body))
		root := rowStretch(widget.Sized{W: 240, Child: widget.Text{S: "NAV"}}, widget.Expand(pane))
		h := mountFlex(t, root) // one Render — LayoutBuilder now settles same-frame
		if !semLabelsHas(h, "LINE") {
			t.Errorf("scroll content missing after one frame (LayoutBuilder should settle in-frame)")
		}
	})
}
