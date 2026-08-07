package geom

import (
	"math"
	"testing"
)

func approx(a, b float32) bool {
	return math.Abs(float64(a-b)) < 1e-4
}

func ptApprox(a, b Pt) bool { return approx(a.X, b.X) && approx(a.Y, b.Y) }

func TestRectBasics(t *testing.T) {
	r := RectXYWH(10, 20, 100, 50)
	if r.Dx() != 100 || r.Dy() != 50 {
		t.Fatalf("Dx/Dy = %v/%v, want 100/50", r.Dx(), r.Dy())
	}
	if got := r.Size(); got != (Size{100, 50}) {
		t.Fatalf("Size = %v", got)
	}
	if r.IsEmpty() {
		t.Fatal("rect should not be empty")
	}
	if !r.Contains(Pt{10, 20}) {
		t.Fatal("Min corner should be inside (half-open)")
	}
	if r.Contains(Pt{110, 70}) {
		t.Fatal("Max corner should be outside (half-open)")
	}
}

func TestRectIntersectUnion(t *testing.T) {
	a := RectXYWH(0, 0, 10, 10)
	b := RectXYWH(5, 5, 10, 10)
	got := a.Intersect(b)
	want := RectXYWH(5, 5, 5, 5)
	if got != want {
		t.Fatalf("Intersect = %v, want %v", got, want)
	}

	c := RectXYWH(100, 100, 5, 5)
	if !a.Intersect(c).IsEmpty() {
		t.Fatal("disjoint intersect should be empty")
	}

	u := a.Union(b)
	if u != RectXYWH(0, 0, 15, 15) {
		t.Fatalf("Union = %v", u)
	}
	if a.Union(Rect{}) != a {
		t.Fatal("union with empty should be identity")
	}
	if (Rect{}).Union(a) != a {
		t.Fatal("union with empty should be identity (reversed)")
	}
}

func TestInsets(t *testing.T) {
	r := RectXYWH(0, 0, 100, 100)
	i := Insets{Top: 10, Right: 20, Bottom: 30, Left: 40}
	got := i.Inset(r)
	want := Rect{Pt{40, 10}, Pt{80, 70}}
	if got != want {
		t.Fatalf("Inset = %v, want %v", got, want)
	}
	if i.Horizontal() != 60 || i.Vertical() != 40 {
		t.Fatalf("Horizontal/Vertical = %v/%v", i.Horizontal(), i.Vertical())
	}

	// Over-inset collapses to center, never inverts.
	small := RectXYWH(0, 0, 10, 10)
	c := InsetsAll(20).Inset(small)
	if !c.IsEmpty() {
		t.Fatalf("over-inset should be empty, got %v", c)
	}
	if !approx(c.Min.X, 5) || !approx(c.Min.Y, 5) {
		t.Fatalf("over-inset should collapse to center, got %v", c)
	}
}

func TestPtOps(t *testing.T) {
	if got := (Pt{1, 2}).Add(Pt{3, 4}); got != (Pt{4, 6}) {
		t.Fatalf("Add = %v", got)
	}
	if got := (Pt{5, 5}).Sub(Pt{2, 3}); got != (Pt{3, 2}) {
		t.Fatalf("Sub = %v", got)
	}
	if got := (Pt{1, 2}).Lerp(Pt{3, 6}, 0.5); !ptApprox(got, Pt{2, 4}) {
		t.Fatalf("Lerp = %v", got)
	}
}
