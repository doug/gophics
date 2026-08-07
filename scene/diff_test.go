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
	a.Recorder().TextIn("", "old", geom.Pt{X: 20, Y: 50}, 14, paint.RGB(1, 1, 1))
	b.Recorder().TextIn("", "new longer text", geom.Pt{X: 20, Y: 50}, 14, paint.RGB(1, 1, 1))

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

// TestDiffMutatedPathUnionsOldAndNewBounds guards the record-time path bounds:
// a retained *paint.Path is mutated in place across frames, so the damage rect
// for a path that moved must cover BOTH where it was and where it is — reading
// live bounds would report only the new location and leave the old region
// uncleared.
func TestDiffMutatedPathUnionsOldAndNewBounds(t *testing.T) {
	m := measurer(t)
	tri := func(p *paint.Path, x, y float32) {
		p.MoveTo(geom.Pt{X: x, Y: y}).
			LineTo(geom.Pt{X: x + 20, Y: y}).
			LineTo(geom.Pt{X: x + 10, Y: y + 20}).Close()
	}
	path := paint.NewPath()
	tri(path, 10, 10) // old frame: near the top-left
	var a, b scene.List
	a.Recorder().FillPath(path, paint.RGB(1, 0, 0))
	path.Reset()
	tri(path, 100, 100) // new frame: same pointer, moved far away
	b.Recorder().FillPath(path, paint.RGB(1, 0, 0))

	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("a moved path must report change")
	}
	// Damage must span from the old triangle (~10,10) to the new one (~120,120).
	if damage.Min.X > 10 || damage.Min.Y > 10 || damage.Max.X < 120 || damage.Max.Y < 120 {
		t.Fatalf("damage %v does not cover both old (10,10) and new (120,120) regions", damage)
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
