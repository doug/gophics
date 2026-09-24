package widget_test

import (
	"testing"

	"github.com/doug/gophics/widget"
)

// Tab does not reach a field on a page the Navigator has pushed another page
// over.
//
// The walk took every focusable in the element tree, and an offstage page is
// still mounted — laid out, not painted, not hit-testable — so Tab from the
// top page's field landed on the home page's, where no caret can be seen.
func TestTabSkipsOffstageNavigatorPages(t *testing.T) {
	var nav widget.Nav
	var homeFocus, topFocus []bool
	home := builder{func(ctx widget.Ctx) widget.Widget {
		nav = ctx.MustOf[widget.Nav]()
		return widget.Sized{W: 300, Child: widget.TextField{Value: "home",
			OnFocus: func(v bool) { homeFocus = append(homeFocus, v) }}}
	}}
	top := builder{func(ctx widget.Ctx) widget.Widget {
		return widget.Sized{W: 300, Child: widget.TextField{Value: "top", Autofocus: true,
			OnFocus: func(v bool) { topFocus = append(topFocus, v) }}}
	}}
	h := headless(t, widget.Navigator{Home: home}, 320, 240)
	h.Render()
	nav.Push(top)
	for range 6 {
		h.Step(0.1)
		h.Render()
	}
	if nav.Depth() != 2 || len(topFocus) == 0 || !topFocus[len(topFocus)-1] {
		t.Fatalf("depth=%d top page focus log %v; the pushed page did not settle focused", nav.Depth(), topFocus)
	}

	if h.Owner().MoveFocus(true) {
		t.Error("Tab moved focus away from the only field on the visible page")
	}
	if len(homeFocus) > 0 && homeFocus[len(homeFocus)-1] {
		t.Error("Tab focused the field on the offstage home page")
	}

	// Popping brings the home page back on stage, and back into the cycle.
	nav.Pop()
	for range 6 {
		h.Step(0.1)
		h.Render()
	}
	if !h.Owner().MoveFocus(true) {
		t.Error("Tab found nothing to focus on the home page once it was back on stage")
	}
	if len(homeFocus) == 0 || !homeFocus[len(homeFocus)-1] {
		t.Errorf("home page focus log %v; want focus after Tab", homeFocus)
	}
}

// An overlay entry with fields is its own Tab cycle: a dialog keeps focus
// among its fields rather than letting Tab wander into the page beneath its
// scrim. An entry with nothing to focus does not capture Tab.
func TestTabStaysWithinTheTopmostFocusableOverlayEntry(t *testing.T) {
	var ov widget.Overlay
	var pageFocus, dialogFocus []bool
	page := builder{func(ctx widget.Ctx) widget.Widget {
		ov = ctx.MustOf[widget.Overlay]()
		return widget.Column(
			widget.Sized{W: 300, H: 20, Child: widget.TextField{Value: "a",
				OnFocus: func(v bool) { pageFocus = append(pageFocus, v) }}},
			widget.Sized{W: 300, H: 20, Child: widget.TextField{Value: "b",
				OnFocus: func(v bool) { pageFocus = append(pageFocus, v) }}},
		)
	}}
	h := headless(t, widget.OverlayHost{Child: page}, 320, 240)
	h.Render()
	if !h.Owner().MoveFocus(true) || len(pageFocus) != 1 {
		t.Fatalf("Tab on the bare page: moved=%v log=%v", len(pageFocus) == 1, pageFocus)
	}

	// A snackbar-like entry with nothing to focus leaves the page's cycle alone.
	note := ov.Show(widget.Text{Value: "saved"})
	h.Render()
	if !h.Owner().MoveFocus(true) || len(pageFocus) != 3 {
		t.Fatalf("Tab under a non-focusable overlay entry: log=%v, want the page's second field focused", pageFocus)
	}
	note.Dismiss()

	// A dialog with two fields takes the cycle.
	dialog := ov.Show(widget.Column(
		widget.Sized{W: 300, H: 20, Child: widget.TextField{Value: "x", Autofocus: true,
			OnFocus: func(v bool) { dialogFocus = append(dialogFocus, v) }}},
		widget.Sized{W: 300, H: 20, Child: widget.TextField{Value: "y",
			OnFocus: func(v bool) { dialogFocus = append(dialogFocus, v) }}},
	))
	h.Render()
	pageFocus = pageFocus[:0]
	for range 3 {
		if !h.Owner().MoveFocus(true) {
			t.Fatal("Tab found nothing to move to inside the dialog")
		}
	}
	if len(pageFocus) != 0 {
		t.Errorf("Tab reached the page under the dialog: page focus log %v", pageFocus)
	}
	if len(dialogFocus) < 4 {
		t.Errorf("dialog focus log %v; Tab did not cycle between the dialog's fields", dialogFocus)
	}

	// Dismissing the dialog hands the cycle back to the page.
	dialog.Dismiss()
	h.Render()
	if !h.Owner().MoveFocus(true) || len(pageFocus) == 0 || !pageFocus[len(pageFocus)-1] {
		t.Errorf("Tab after the dialog closed: page focus log %v, want a page field focused", pageFocus)
	}
}
