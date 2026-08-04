package scene_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/scene"
)

func measurer(t *testing.T) *paint.Painter {
	t.Helper()
	p := paint.NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDiffIdenticalNoDamage(t *testing.T) {
	m := measurer(t)
	var a, b scene.List
	drawScene(a.Recorder())
	drawScene(b.Recorder())
	if _, changed := b.Diff(&a, m); changed {
		t.Fatal("identical scenes must report no change")
	}
}

func TestDiffLocalizedChange(t *testing.T) {
	m := measurer(t)
	var a, b scene.List
	base := func(c paint.Canvas, hoverColor paint.Color) {
		c.FillRect(geom.RectXYWH(0, 0, 400, 400), paint.RGB(0, 0, 0))
		c.FillRRect(geom.RectXYWH(10, 10, 100, 40), 4, paint.RGB(0.5, 0.5, 0.5))
		c.FillRRect(geom.RectXYWH(10, 60, 100, 40), 4, hoverColor) // the change
		c.FillRRect(geom.RectXYWH(10, 110, 100, 40), 4, paint.RGB(0.5, 0.5, 0.5))
	}
	base(a.Recorder(), paint.RGB(0.5, 0.5, 0.5))
	base(b.Recorder(), paint.RGB(0.9, 0.9, 0.9))

	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("changed scene must report change")
	}
	want := geom.RectXYWH(10, 60, 100, 40)
	if damage != want {
		t.Fatalf("damage = %v, want tight %v", damage, want)
	}
}

func TestDiffTextChangeUsesMeasuredBounds(t *testing.T) {
	m := measurer(t)
	var a, b scene.List
	a.Recorder().Text("old", geom.Pt{X: 20, Y: 50}, 14, paint.RGB(1, 1, 1))
	b.Recorder().Text("new longer text", geom.Pt{X: 20, Y: 50}, 14, paint.RGB(1, 1, 1))

	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("text change must report change")
	}
	if damage.Min.X != 20 || damage.Min.Y >= 50 || damage.Max.Y <= 50 {
		t.Fatalf("text damage %v should span the baseline at y=50", damage)
	}
	wide := m.MeasureWidth("new longer text", 14)
	if damage.Max.X < 20+wide {
		t.Fatalf("damage %v narrower than new text width %v", damage, wide)
	}
}

func TestDiffClipStructureChangeIsFullDamage(t *testing.T) {
	m := measurer(t)
	var a, b scene.List
	a.Recorder().FillRect(geom.RectXYWH(0, 0, 10, 10), paint.RGB(1, 0, 0))
	rb := b.Recorder()
	rb.PushClip(geom.RectXYWH(0, 0, 5, 5))
	rb.FillRect(geom.RectXYWH(0, 0, 10, 10), paint.RGB(1, 0, 0))
	rb.PopClip()

	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("must change")
	}
	if damage.Dx() < 1e6 {
		t.Fatalf("clip structure change should be unbounded damage, got %v", damage)
	}
}
