package widget_test

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// lifecycleRow is a stateful row that records its Init and Dispose, so a
// remount is visible.
type lifecycleRow struct{ log *[]string }

func (r lifecycleRow) CreateState() widget.State { return &lifecycleRowState{} }

type lifecycleRowState struct {
	widget.StateBase[lifecycleRow]
}

func (s *lifecycleRowState) Init(widget.Ctx) { *s.W().log = append(*s.W().log, "init") }
func (s *lifecycleRowState) Dispose()        { *s.W().log = append(*s.W().log, "dispose") }
func (s *lifecycleRowState) Build(widget.Ctx) widget.Widget {
	return widget.Sized{W: 300, H: 40}
}

// Pressing a Reorderable row keeps the row's content mounted. The dragged
// row is translated by a wrapper, and the wrapper used to appear on press
// and go on release — a different widget type at the same position, which
// the reconciler treats as a different widget — so a tap remounted the row
// twice, dropping its state, and a field inside a row was focused by the
// press and unmounted by the next frame.
func TestReorderableRowSurvivesPress(t *testing.T) {
	var log []string
	var focus []bool
	h := headless(t, widget.Reorderable{Count: 2, ItemExtent: 40, Build: func(i int) widget.Widget {
		if i == 0 {
			return widget.Sized{W: 300, H: 40, Child: widget.TextField{Value: "row0",
				OnFocus: func(v bool) { focus = append(focus, v) }}}
		}
		return lifecycleRow{log: &log}
	}}, 320, 240)
	h.Render()
	log = log[:0]

	// Press, frame, release, frame: the sequence a real tap runs through.
	h.Press(geom.Pt{X: 150, Y: 60})
	h.Render()
	h.Release(geom.Pt{X: 150, Y: 60})
	h.Render()
	if len(log) != 0 {
		t.Errorf("a tap remounted the row's content: %v", log)
	}

	h.Press(geom.Pt{X: 150, Y: 20})
	h.Render()
	h.Release(geom.Pt{X: 150, Y: 20})
	h.Render()
	if len(focus) != 1 || !focus[0] {
		t.Errorf("field inside a tapped row has focus log %v, want [true]", focus)
	}
	if h.Owner().KeyboardTarget == nil {
		t.Error("the field inside the tapped row does not hold keyboard focus")
	}
}
