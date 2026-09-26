package widget_test

import (
	"testing"

	"github.com/doug/gophics/widget"
)

// A page pushed over a focused field takes the keyboard with it: the covered
// page stays mounted, but its field lets go of focus (OnFocus(false), which
// is also what hides the soft keyboard) so keystrokes on the new page do not
// edit a field the user can no longer see.
func TestPushReleasesFocusOnCoveredPage(t *testing.T) {
	var nav widget.Nav
	var changes []string
	var focus []bool
	home := builder{func(ctx widget.Ctx) widget.Widget {
		nav = ctx.MustOf[widget.Nav]()
		return widget.Sized{W: 300, H: 20, Child: widget.TextField{Value: "", Autofocus: true,
			OnChange: func(s string) { changes = append(changes, s) },
			OnFocus:  func(v bool) { focus = append(focus, v) }}}
	}}
	top := builder{func(ctx widget.Ctx) widget.Widget {
		return widget.Text{Value: "detail page"}
	}}
	h := headless(t, widget.Navigator{Home: home}, 320, 240)
	h.Render()
	if len(focus) != 1 || !focus[0] {
		t.Fatalf("home field focus log %v before the push, want [true]", focus)
	}
	nav.Push(top)
	for range 8 {
		h.Step(0.1)
		h.Render()
	}
	if nav.Depth() != 2 {
		t.Fatal("push did not settle")
	}
	if h.Owner().KeyboardTarget != nil {
		t.Error("keyboard focus is still held after the page holding it went offstage")
	}
	if len(focus) != 2 || focus[1] {
		t.Errorf("home field focus log %v after the push, want [true false]", focus)
	}
	h.Type("abc")
	h.Render()
	if len(changes) != 0 {
		t.Errorf("typing on the pushed page edited the offstage home field: %q", changes)
	}
}
