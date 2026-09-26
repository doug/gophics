package widget_test

import (
	"testing"

	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// A Modal on a Navigator page under the top one stays mounted but does not
// take Escape: the key belongs to the page the user can see. Before this the
// registration order alone decided, so a page holding a Modal swallowed
// Escape for every page pushed over it.
func TestOffstagePageModalDeclinesEscape(t *testing.T) {
	var nav widget.Nav
	escapes := 0
	home := builder{func(ctx widget.Ctx) widget.Widget {
		nav = ctx.MustOf[widget.Nav]()
		return widget.Modal{OnEscape: func() { escapes++ }, Child: widget.Sized{W: 300, H: 20}}
	}}
	top := builder{func(ctx widget.Ctx) widget.Widget {
		return widget.Sized{W: 300, H: 20, Child: widget.TextField{Value: "hello", Autofocus: true}}
	}}
	h := headless(t, widget.Navigator{Home: home}, 320, 240)
	h.Render()
	h.Key(shell.KeyEscape)
	if escapes != 1 {
		t.Fatalf("Escape on the home page reached its modal %d times, want 1", escapes)
	}

	settle := func() {
		for range 8 {
			h.Step(0.1)
			h.Render()
		}
	}
	nav.Push(top)
	settle()
	if nav.Depth() != 2 {
		t.Fatal("push did not settle")
	}
	if h.Owner().TopModal() != nil {
		t.Error("the covered page's modal is still the top modal")
	}
	h.Key(shell.KeyEscape)
	if escapes != 1 {
		t.Errorf("Escape on the pushed page reached the covered page's modal (%d)", escapes)
	}

	// Popping back puts the page — and its modal — within reach again.
	nav.Pop()
	settle()
	h.Key(shell.KeyEscape)
	if escapes != 2 {
		t.Errorf("Escape after popping back reached the home modal %d times, want 2", escapes)
	}
}
