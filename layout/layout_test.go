package layout

import (
	"math"
	"testing"

	"github.com/doug/gophics/geom"
)

// Constrain is the protocol's one load-bearing arithmetic: every Box's
// returned size passes through it, so an off-by-anything here misplaces every
// widget at once.
func TestConstrain(t *testing.T) {
	c := Constraints{Min: geom.Size{W: 10, H: 20}, Max: geom.Size{W: 100, H: 200}}
	cases := []struct{ in, want geom.Size }{
		{geom.Size{W: 50, H: 50}, geom.Size{W: 50, H: 50}},     // inside: untouched
		{geom.Size{W: 5, H: 5}, geom.Size{W: 10, H: 20}},       // under: raised to min
		{geom.Size{W: 500, H: 500}, geom.Size{W: 100, H: 200}}, // over: capped at max
		{geom.Size{W: 5, H: 500}, geom.Size{W: 10, H: 200}},    // mixed per-axis
	}
	for _, tc := range cases {
		if got := c.Constrain(tc.in); got != tc.want {
			t.Errorf("Constrain(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

// An unbounded axis passes any size through; Inf must never leak out as a
// "nearest satisfying size" for a finite request.
func TestConstrainUnbounded(t *testing.T) {
	c := Unbounded()
	got := c.Constrain(geom.Size{W: 12345, H: 1})
	if got != (geom.Size{W: 12345, H: 1}) {
		t.Errorf("unbounded Constrain changed the size: %v", got)
	}
	if !math.IsInf(float64(c.Max.W), 1) || c.BoundedW() || c.BoundedH() {
		t.Error("Unbounded() reports bounded axes")
	}
}

func TestTightAndLoose(t *testing.T) {
	s := geom.Size{W: 30, H: 40}
	tight := Tight(s)
	if tight.Constrain(geom.Size{}) != s || tight.Constrain(geom.Size{W: 999, H: 999}) != s {
		t.Error("Tight admits a size other than its own")
	}
	loose := Loose(s)
	if loose.Constrain(geom.Size{}) != (geom.Size{}) {
		t.Error("Loose raised a zero size; it must have no minimum")
	}
	if loose.Constrain(geom.Size{W: 999, H: 999}) != s {
		t.Error("Loose admitted a size over its maximum")
	}
}

// Deflate is how padding constrains a child. The clamp at zero is the part
// worth pinning: padding wider than the available space must produce a zero
// constraint, not a negative one — a negative Max makes Constrain return
// negative sizes and the child paints outside its parent, mirrored.
func TestDeflateClampsAtZero(t *testing.T) {
	c := Constraints{Min: geom.Size{W: 10, H: 10}, Max: geom.Size{W: 30, H: 30}}
	d := c.Deflate(geom.InsetsAll(8))
	want := Constraints{Max: geom.Size{W: 14, H: 14}}
	if d.Max != want.Max {
		t.Errorf("Deflate max = %v, want %v", d.Max, want.Max)
	}
	over := c.Deflate(geom.InsetsAll(50))
	if over.Max != (geom.Size{}) || over.Min != (geom.Size{}) {
		t.Errorf("insets wider than the space gave %+v, want all-zero constraints", over)
	}
}

// Deflating an unbounded axis must keep it unbounded: Inf minus any inset is
// still Inf, and a Deflate that turned Inf into a huge finite number would
// make scroll content suddenly clamp.
func TestDeflateKeepsUnbounded(t *testing.T) {
	d := Unbounded().Deflate(geom.InsetsAll(16))
	if !math.IsInf(float64(d.Max.W), 1) || !math.IsInf(float64(d.Max.H), 1) {
		t.Errorf("Deflate bounded an unbounded axis: %v", d.Max)
	}
}

func TestLoosenDropsOnlyMinimums(t *testing.T) {
	c := Constraints{Min: geom.Size{W: 10, H: 10}, Max: geom.Size{W: 30, H: 40}}
	l := c.Loosen()
	if l.Min != (geom.Size{}) {
		t.Errorf("Loosen kept a minimum: %v", l.Min)
	}
	if l.Max != c.Max {
		t.Errorf("Loosen changed the maximum: %v", l.Max)
	}
}
