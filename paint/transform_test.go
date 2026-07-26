package paint

import (
	"testing"

	"github.com/doug/gossamer/geom"
)

func TestMapRect(t *testing.T) {
	src := geom.RectXYWH(0, 0, 100, 40)
	dst := geom.RectXYWH(120, 120, 200, 80)
	tr := MapRect(src, dst)

	if tr.SX != 2 || tr.SY != 2 {
		t.Fatalf("scale = %v,%v want 2,2", tr.SX, tr.SY)
	}
	if tr.TX != 120 || tr.TY != 120 {
		t.Fatalf("translate = %v,%v want 120,120", tr.TX, tr.TY)
	}
	if tr.PivotX != 0 || tr.PivotY != 0 {
		t.Fatalf("pivot = %v,%v want 0,0 (src origin)", tr.PivotX, tr.PivotY)
	}
}

func TestMapRectZeroSourceIsSafe(t *testing.T) {
	// A degenerate source must not divide by zero — scale falls back to 1.
	tr := MapRect(geom.RectXYWH(10, 10, 0, 0), geom.RectXYWH(0, 0, 50, 50))
	if tr.SX != 1 || tr.SY != 1 {
		t.Fatalf("zero-source scale = %v,%v want 1,1", tr.SX, tr.SY)
	}
}
