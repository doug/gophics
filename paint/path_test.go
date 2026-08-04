package paint

import (
	"testing"

	"github.com/doug/gophics/geom"
)

func TestPathBounds(t *testing.T) {
	p := NewPath()
	if !p.Empty() {
		t.Fatal("new path should be empty")
	}
	p.MoveTo(geom.Pt{X: 10, Y: 20}).
		LineTo(geom.Pt{X: 100, Y: 20}).
		LineTo(geom.Pt{X: 50, Y: 80}).
		Close()
	if p.Empty() {
		t.Fatal("path should be non-empty after commands")
	}
	b := p.Bounds()
	if b.Min != (geom.Pt{X: 10, Y: 20}) || b.Max != (geom.Pt{X: 100, Y: 80}) {
		t.Fatalf("bounds = %+v, want (10,20)-(100,80)", b)
	}
}

func TestPathGenAndReset(t *testing.T) {
	p := NewPath()
	g0 := p.Gen()
	p.MoveTo(geom.Pt{X: 0, Y: 0})
	if p.Gen() == g0 {
		t.Fatal("Gen should advance on mutation")
	}
	p.LineTo(geom.Pt{X: 5, Y: 5})
	g1 := p.Gen()
	p.Reset()
	if p.Gen() == g1 || !p.Empty() {
		t.Fatalf("Reset should clear and bump gen: empty=%v", p.Empty())
	}
	if p.Bounds() != (geom.Rect{}) {
		t.Fatalf("empty path bounds = %+v, want zero", p.Bounds())
	}
}

// TestLineToWithoutMove starts a subpath implicitly.
func TestLineToWithoutMove(t *testing.T) {
	p := NewPath().LineTo(geom.Pt{X: 3, Y: 4})
	if p.Empty() {
		t.Fatal("LineTo on an empty path should start it")
	}
	if b := p.Bounds(); b.Min != (geom.Pt{X: 3, Y: 4}) {
		t.Fatalf("bounds min = %+v", b.Min)
	}
}
