package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// A drag repaints on every move, so the carried preview follows the pointer.
//
// update set the position and then returned early whenever the hovered target
// had not changed. The preview is positioned from that position, so it moved
// only when something else happened to repaint: crossing into a *different*
// drop target moved it, moving within one did not, and moving across empty
// space did not either. On a phone the ghost sat where the last target change
// had left it while the finger carried on without it.
//
// This drives the session directly because that is the only way to see it. A
// test that renders after each move rebuilds the tree whatever the session
// did, which is exactly why the earlier ghost test passed while the ghost was
// standing still on a device.
func TestDragSessionRepaintsOnEveryMove(t *testing.T) {
	var repaints int
	s := &DragSession{repaint: func() { repaints++ }}
	s.begin("payload", geom.Pt{X: 0, Y: 0})

	// No drop targets at all: the case where the early return used to skip
	// every repaint, since the hovered target is nil throughout.
	start := repaints
	for _, p := range []geom.Pt{{X: 10}, {X: 20}, {X: 30}, {X: 30, Y: 5}} {
		s.update(p)
	}
	if got := repaints - start; got < 4 {
		t.Errorf("four pointer moves produced %d repaints; the preview only follows if each one does", got)
	}

	// A move to where it already is changes nothing and should not repaint:
	// the fix must not turn into a repaint on every event regardless.
	n := repaints
	s.update(geom.Pt{X: 30, Y: 5})
	if repaints != n {
		t.Errorf("a move to the same position repainted %d extra times", repaints-n)
	}
}
