package scene_test

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/paint"
)

// A change *behind* a backdrop blur reaches further than its own bounds: the
// blur spreads it by the radius, and a partial replay clipped to the change
// would blur the stale, already-tinted pixels outside the clip back into the
// panel, leaving a ring of the old colour. The whole panel has to repaint.
func TestDiffChangeUnderBackdropBlurDamagesThePanel(t *testing.T) {
	m := measurer(t)
	content := geom.RectXYWH(0, 0, 40, 40)
	panel := geom.RectXYWH(20, 20, 100, 100)
	glass := func(c paint.Canvas, under paint.Color) {
		c.FillRect(content, under)
		c.BackdropBlur(panel, 12)
		c.FillRRect(panel, 8, paint.Color{R: 1, G: 1, B: 1, A: 0.3})
	}
	var a, b scene.List
	glass(a.Recorder(), paint.RGB(1, 0, 0))
	glass(b.Recorder(), paint.RGB(0, 0, 1))
	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("changed scene must report change")
	}
	if want := content.Union(panel); damage != want {
		t.Fatalf("damage = %v, want %v (the changed content plus the blurred panel)", damage, want)
	}

	// A change painted *over* the panel is not sampled by the blur, so the
	// panel need not repaint for it.
	over := func(c paint.Canvas, col paint.Color) {
		c.BackdropBlur(panel, 12)
		c.FillRRect(panel, 8, paint.Color{R: 1, G: 1, B: 1, A: 0.3})
		c.FillRect(content, col)
	}
	var x, y scene.List
	over(x.Recorder(), paint.RGB(1, 0, 0))
	over(y.Recorder(), paint.RGB(0, 0, 1))
	if damage, _ := y.Diff(&x, m); damage != content {
		t.Fatalf("damage for a change above the blur = %v, want just %v", damage, content)
	}

	// A change outside the blur's reach (further than the radius from the
	// panel) leaves the panel alone.
	far := geom.RectXYWH(300, 300, 10, 10)
	farOff := func(c paint.Canvas, col paint.Color) {
		c.FillRect(far, col)
		c.BackdropBlur(panel, 12)
	}
	var u, v scene.List
	farOff(u.Recorder(), paint.RGB(1, 0, 0))
	farOff(v.Recorder(), paint.RGB(0, 0, 1))
	if damage, _ := v.Diff(&u, m); damage != far {
		t.Fatalf("damage for a change out of the blur's reach = %v, want just %v", damage, far)
	}
}
