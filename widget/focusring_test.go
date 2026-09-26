package widget_test

import (
	"testing"

	"github.com/doug/gophics/widget"
)

// focusRing mirrors theme.Field: a stateful wrapper that keeps a focused flag
// from the field's OnFocus and styles itself by it — the documented way to
// draw a focus ring.
type focusRing struct {
	disabled bool
	log      *[]bool // the focused value each Build saw
}

func (r focusRing) CreateState() widget.State { return &focusRingState{} }

type focusRingState struct {
	widget.StateBase[focusRing]
	focused bool
}

func (s *focusRingState) Build(ctx widget.Ctx) widget.Widget {
	*s.W().log = append(*s.W().log, s.focused)
	return widget.Decorated{BorderWidth: 2, Child: widget.Sized{W: 300, Child: widget.TextField{
		Value: "x", Autofocus: true, Disabled: s.W().disabled,
		OnFocus: func(v bool) { s.SetState(func() { s.focused = v }) },
	}}}
}

// Disabling a focused field from a parent's rebuild releases its focus from
// inside the wrapper's own update, and the SetState the wrapper makes in
// OnFocus(false) has to survive that: the wrapper's Build already ran with
// focused == true, so it must run again, or the ring stays painted on a
// field that can no longer be typed into.
func TestFocusRingClearsWhenFieldDisabled(t *testing.T) {
	var log []bool
	disabled := false
	root, m := newMutable(func(widget.Ctx) widget.Widget {
		return focusRing{disabled: disabled, log: &log}
	})
	h := headless(t, root, 320, 240)
	h.Render()
	if len(log) == 0 || !log[len(log)-1] {
		t.Fatalf("builds after mount saw %v, want the last focused", log)
	}
	disabled = true
	m.Rebuild()
	h.Render()
	h.Render()
	if h.Owner().NeedsBuild() {
		t.Fatal("a rebuild is still pending after two frames")
	}
	if log[len(log)-1] {
		t.Errorf("the wrapper's last build saw focused=true on a disabled field (builds: %v)", log)
	}
}
