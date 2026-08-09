package scene_test

import (
	"image"
	"image/color"
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

// sliceImage is a struct-typed (non-pointer) image.Image whose dynamic type
// is NOT comparable — comparing two of them with == panics at runtime. The
// diff must handle it via identity keys, never interface equality.
type sliceImage struct {
	pix []uint8 // slice field: makes the type non-comparable
	w   int
}

func (s sliceImage) ColorModel() color.Model { return color.RGBAModel }
func (s sliceImage) Bounds() image.Rectangle { return image.Rect(0, 0, s.w, s.w) }
func (s sliceImage) At(x, y int) color.Color {
	return color.RGBA{R: s.pix[0], A: 255}
}

// TestStructImageDiffsWithoutPanic guards H4: recording and diffing a
// non-comparable struct-typed image must not panic. Such images have no cheap
// identity, so they always diff as changed (a repaint, never a crash).
func TestStructImageDiffsWithoutPanic(t *testing.T) {
	m := measurer(t)
	img := sliceImage{pix: []uint8{200, 10, 10, 255}, w: 4}
	var a, b scene.List
	a.Recorder().Image(img, geom.RectXYWH(10, 10, 40, 40))
	b.Recorder().Image(img, geom.RectXYWH(10, 10, 40, 40))

	damage, changed := b.Diff(&a, m) // must not panic
	if !changed {
		t.Fatal("a non-pointer image has no identity: it must always diff as changed")
	}
	if want := geom.RectXYWH(10, 10, 40, 40); damage != want {
		t.Fatalf("struct-image damage = %v, want %v", damage, want)
	}

	// Same for a sprite atlas.
	var c, d scene.List
	sp := paint.Sprite{Src: image.Rect(0, 0, 2, 2), Dst: geom.RectXYWH(0, 0, 8, 8)}
	c.Recorder().DrawSprite(img, sp)
	d.Recorder().DrawSprite(img, sp)
	if _, changed := d.Diff(&c, m); !changed {
		t.Fatal("a non-pointer atlas must always diff as changed")
	}
}

// TestPointerImageDiffsByIdentity: the same pointer-typed image value across
// frames compares equal (no damage); a different image value at the same
// destination diffs as changed.
func TestPointerImageDiffsByIdentity(t *testing.T) {
	m := measurer(t)
	dst := geom.RectXYWH(10, 10, 40, 40)
	var a, b scene.List
	a.Recorder().Image(testImage, dst)
	b.Recorder().Image(testImage, dst)
	if _, changed := b.Diff(&a, m); changed {
		t.Fatal("same image pointer must not diff as changed")
	}
	other := image.NewRGBA(image.Rect(0, 0, 8, 8))
	var c scene.List
	c.Recorder().Image(other, dst)
	if _, changed := c.Diff(&a, m); !changed {
		t.Fatal("a different image value must diff as changed")
	}
}

// TestDiffOpacityAlphaChangeDamagesGroupBounds guards the M4 layer-bounds
// damage: an animating opacity group (alpha change only) must damage just its
// content bounds, not the whole surface — and must not set HasLayers.
func TestDiffOpacityAlphaChangeDamagesGroupBounds(t *testing.T) {
	m := measurer(t)
	frame := func(c paint.Canvas, alpha float32) {
		c.FillRect(geom.RectXYWH(0, 0, 400, 400), paint.RGB(0, 0, 0))
		c.PushOpacity(alpha)
		c.FillRRect(geom.RectXYWH(100, 100, 60, 40), 4, paint.RGB(1, 1, 1))
		c.PopOpacity()
		c.FillRect(geom.RectXYWH(0, 380, 400, 20), paint.RGB(0.2, 0.2, 0.2))
	}
	var a, b scene.List
	frame(a.Recorder(), 0.3)
	frame(b.Recorder(), 0.7)
	if b.HasLayers() {
		t.Fatal("an opacity group must not force full-surface repaint (HasLayers)")
	}
	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("alpha change must report change")
	}
	group := geom.RectXYWH(100, 100, 60, 40)
	if damage.Intersect(group) != group {
		t.Fatalf("damage %v must cover the group content %v", damage, group)
	}
	if damage.Dy() > 100 || damage.Dx() > 200 {
		t.Fatalf("damage %v should be near the group's bounds, not the surface", damage)
	}
}

// TestDiffContentChangeInsideOpacityGroupIsTight: unchanged group boundaries
// with changed content inside diff to the content's own bounds.
func TestDiffContentChangeInsideOpacityGroupIsTight(t *testing.T) {
	m := measurer(t)
	frame := func(c paint.Canvas, col paint.Color) {
		c.FillRect(geom.RectXYWH(0, 0, 400, 400), paint.RGB(0, 0, 0))
		c.PushOpacity(0.5)
		c.FillRect(geom.RectXYWH(100, 100, 60, 40), col)
		c.PopOpacity()
	}
	var a, b scene.List
	frame(a.Recorder(), paint.RGB(1, 0, 0))
	frame(b.Recorder(), paint.RGB(0, 1, 0))
	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("content change must report change")
	}
	if want := geom.RectXYWH(100, 100, 60, 40); damage != want {
		t.Fatalf("damage = %v, want tight %v", damage, want)
	}
}

// TestDiffTransformInsideOpacityGroupIsUnbounded: a transform nested in a
// changed opacity group records content in another coordinate space, so the
// group's bounds are unknowable — fall back to unbounded damage.
func TestDiffTransformInsideOpacityGroupIsUnbounded(t *testing.T) {
	m := measurer(t)
	frame := func(c paint.Canvas, alpha float32) {
		c.PushOpacity(alpha)
		c.PushTransform(paint.Transform{TX: 10})
		c.FillRect(geom.RectXYWH(0, 0, 10, 10), paint.RGB(1, 0, 0))
		c.PopTransform()
		c.PopOpacity()
	}
	var a, b scene.List
	frame(a.Recorder(), 0.3)
	frame(b.Recorder(), 0.7)
	if !b.HasLayers() {
		t.Fatal("a transform must still set HasLayers")
	}
	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("must change")
	}
	if damage.Dx() < 1e6 {
		t.Fatalf("transform inside changed group should be unbounded damage, got %v", damage)
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
