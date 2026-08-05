package geom

import "testing"

func TestPtMulIn(t *testing.T) {
	if got := (Pt{2, -3}).Mul(2.5); got != (Pt{5, -7.5}) {
		t.Errorf("Pt.Mul = %v", got)
	}
	r := RectXYWH(0, 0, 10, 10)
	if !(Pt{0, 0}).In(r) { // Min corner is inside (half-open)
		t.Error("Min corner should be In")
	}
	if (Pt{10, 10}).In(r) { // Max corner is outside
		t.Error("Max corner should not be In")
	}
	if (Pt{-1, 5}).In(r) {
		t.Error("point left of rect should not be In")
	}
}

func TestLerps(t *testing.T) {
	if got := LerpFloat(10, 20, 0.25); got != 12.5 {
		t.Errorf("LerpFloat = %v", got)
	}
	if got := (Size{10, 20}).Lerp(Size{20, 40}, 0.5); got != (Size{15, 30}) {
		t.Errorf("Size.Lerp = %v", got)
	}
	a, b := RectXYWH(0, 0, 10, 10), RectXYWH(10, 10, 10, 10)
	if got := a.Lerp(b, 0.5); got != (Rect{Min: Pt{5, 5}, Max: Pt{15, 15}}) {
		t.Errorf("Rect.Lerp = %v", got)
	}
	i := (Insets{2, 4, 6, 8}).Lerp(Insets{4, 8, 12, 16}, 0.5)
	if i != (Insets{3, 6, 9, 12}) {
		t.Errorf("Insets.Lerp = %v", i)
	}
	// Endpoints are exact.
	if got := LerpFloat(3, 7, 0); got != 3 {
		t.Errorf("LerpFloat t=0 = %v", got)
	}
	if got := LerpFloat(3, 7, 1); got != 7 {
		t.Errorf("LerpFloat t=1 = %v", got)
	}
}

func TestSizeHelpers(t *testing.T) {
	if !(Size{0, 10}).IsEmpty() || !(Size{10, 0}).IsEmpty() || !(Size{-1, 5}).IsEmpty() {
		t.Error("zero/negative dimension sizes should be empty")
	}
	if (Size{1, 1}).IsEmpty() {
		t.Error("positive size should not be empty")
	}
	if got := (Size{3, 4}).Pt(); got != (Pt{3, 4}) {
		t.Errorf("Size.Pt = %v", got)
	}
}

func TestRectHelpers(t *testing.T) {
	if got := RectFromSize(Size{5, 8}); got != (Rect{Max: Pt{5, 8}}) {
		t.Errorf("RectFromSize = %v", got)
	}
	r := RectXYWH(1, 2, 3, 4).Translate(Pt{10, 20})
	if r != RectXYWH(11, 22, 3, 4) {
		t.Errorf("Rect.Translate = %v", r)
	}
	if r.Dx() != 3 || r.Dy() != 4 { // dimensions preserved by translate
		t.Errorf("translate changed size: %v", r)
	}
}

func TestRadiusAndRRect(t *testing.T) {
	if got := RadiusCircular(6); got != (Radius{6, 6}) {
		t.Errorf("RadiusCircular = %v", got)
	}
	r := RectXYWH(0, 0, 20, 10)
	rr := RRectUniform(r, 4)
	c := Radius{4, 4}
	if rr.Rect != r || rr.TL != c || rr.TR != c || rr.BR != c || rr.BL != c {
		t.Errorf("RRectUniform = %+v", rr)
	}
}

func TestInsetsSymmetric(t *testing.T) {
	i := InsetsSymmetric(10, 20) // horizontal=10, vertical=20
	if i != (Insets{Top: 20, Right: 10, Bottom: 20, Left: 10}) {
		t.Errorf("InsetsSymmetric = %+v", i)
	}
	if i.Horizontal() != 20 || i.Vertical() != 40 {
		t.Errorf("Horizontal/Vertical = %v/%v", i.Horizontal(), i.Vertical())
	}
}

func TestIdentityAndTranslateAffine(t *testing.T) {
	id := Identity()
	p := Pt{3, 7}
	if got := id.Apply(p); got != p {
		t.Errorf("Identity.Apply changed point: %v", got)
	}
	tr := Translate(Pt{5, -2})
	if got := tr.Apply(p); got != (Pt{8, 5}) {
		t.Errorf("Translate.Apply = %v", got)
	}
	// Composing with identity is a no-op either way.
	m := Scale(2, 3)
	if m.Mul(id) != m || id.Mul(m) != m {
		t.Error("identity is not a multiplicative no-op")
	}
}
