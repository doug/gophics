package layoutbox

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// "Offset is clamped during layout" has to hold on a skip-cache hit too: a
// clean re-layout with the same constraints returned before clamping, so an
// Offset written between layouts could scroll past the end of the content.
func TestViewportClampsOffsetOnASkippedLayout(t *testing.T) {
	v := &Viewport{Offset: 500, Child: &leaf{w: 50, h: 300}}
	cs := layout.Tight(geom.Size{W: 50, H: 100})
	v.Layout(cs)
	if v.Offset != 200 {
		t.Fatalf("first layout clamped Offset to %v, want 200", v.Offset)
	}
	v.Offset = 900
	v.Layout(cs) // same constraints, nothing dirty: the skip path
	if v.Offset != 200 {
		t.Errorf("a skipped layout left Offset at %v, want 200", v.Offset)
	}
	if pt := v.scrollPt(); pt.Y != -200 {
		t.Errorf("content scrolled to %v, want -200", pt.Y)
	}
	v.Offset = -30
	v.Layout(cs)
	if v.Offset != 0 {
		t.Errorf("a negative Offset survived a skipped layout: %v", v.Offset)
	}
}
