package layout

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// TestFlexChildLaidOutWhenMainUnbounded guards against flexed children being
// skipped when the main axis is unbounded (e.g. an Expanded inside a scrolling
// Column): they must fall back to their natural size, not collapse to zero.
func TestFlexChildLaidOutWhenMainUnbounded(t *testing.T) {
	child := &countingLeaf{w: 30, h: 40}
	f := &Flex{Axis: Vertical, Children: []FlexChild{{Box: child, Flex: 1}}}

	f.Layout(Loose(geom.Size{W: 100, H: Inf})) // unbounded main axis

	if child.layouts == 0 {
		t.Fatal("flex child was never laid out under an unbounded main axis")
	}
	if child.Size() != sz(30, 40) {
		t.Fatalf("flex child size = %v, want 30x40 (natural size)", child.Size())
	}
}
