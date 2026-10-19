package scene_test

import (
	"bytes"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/paint"
)

// ramp is the gradient every test here paints: multi-stop, so the band
// fallback would need many bands and the differ would have many ops to walk.
func ramp() *paint.Gradient {
	return paint.LinearGradient(
		geom.Pt{X: 40, Y: 40}, geom.Pt{X: 160, Y: 120},
		paint.EvenStops(
			paint.RGB(0.95, 0.15, 0.1),
			paint.RGB(0.1, 0.8, 0.3),
			paint.RGB(0.1, 0.2, 0.95),
		)...)
}

func gradientFrame(c paint.Canvas, g *paint.Gradient, y float32) {
	c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.RGB(0.1, 0.1, 0.12))
	paint.FillGradient(c, geom.RectXYWH(30, y, 140, 100), 10, g)
}

// TestRecorderRecordsAGradientAsOneOp is the cost claim. On a plain Canvas,
// FillGradient approximates the ramp with up to 256 solid bands; the recorder
// implements paint.GradientFiller so the list holds the gradient itself, and
// the differ then compares one op per frame instead of a few hundred.
func TestRecorderRecordsAGradientAsOneOp(t *testing.T) {
	g := ramp()
	var list scene.List
	paint.FillGradient(list.Recorder(nil), geom.RectXYWH(30, 20, 140, 100), 10, g)
	if list.Len() != 1 {
		t.Fatalf("recorded %d ops for one gradient, want 1 — the recorder took "+
			"the band fallback", list.Len())
	}
}

// TestRecordedGradientReplaysThroughTheExactBrush pins what the single op is
// worth: replayed onto the painter canvas it must produce the same pixels as
// calling FillGradient on that canvas directly, which is the analytic
// per-pixel brush rather than any band approximation. A list that replayed as
// bands would differ visibly, and at 2x the band count would be wrong as well.
func TestRecordedGradientReplaysThroughTheExactBrush(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, g := range []*paint.Gradient{
			ramp(),
			paint.RadialGradient(geom.Pt{X: 100, Y: 70}, 70,
				paint.EvenStops(paint.RGB(1, 0.9, 0.2), paint.RGB(0.6, 0.1, 0.5))...),
			paint.SweepGradient(geom.Pt{X: 100, Y: 70}, 0, 6.2831853,
				paint.EvenStops(paint.RGB(1, 0, 0), paint.RGB(0, 1, 0), paint.RGB(0, 0, 1), paint.RGB(1, 0, 0))...),
		} {
			direct := renderAt(t, scale, func(c paint.Canvas) { gradientFrame(c, g, 20) })

			var list scene.List
			gradientFrame(list.Recorder(nil), g, 20)
			replayed := renderAt(t, scale, func(c paint.Canvas) { list.Replay(c) })

			if !bytes.Equal(direct, replayed) {
				t.Errorf("at %gx: a recorded gradient replayed differently from "+
					"a direct FillGradient", scale)
			}
		}
	}
}

// TestGradientDiffsByIdentity is the whole reason Gradient is immutable after
// construction: the same pointer across two frames is the same ramp, so the
// op compares equal without walking the stop list, exactly as *Path and
// *Marks ops do.
func TestGradientDiffsByIdentity(t *testing.T) {
	m := measurer(t)
	g := ramp()

	var a, b scene.List
	gradientFrame(a.Recorder(nil), g, 20)
	gradientFrame(b.Recorder(nil), g, 20)
	if _, changed := b.Diff(&a, m); changed {
		t.Fatal("the same *Gradient at the same rect must diff as unchanged")
	}

	// An equal but distinct gradient is a new pointer, so it repaints. That is
	// the deliberate cost of rebuilding one per frame rather than keeping it,
	// and the alternative — comparing stop lists — buys a caller who already
	// has the cheap answer available nothing.
	var c scene.List
	gradientFrame(c.Recorder(nil), ramp(), 20)
	if _, changed := c.Diff(&a, m); !changed {
		t.Fatal("a freshly built *Gradient must diff as changed")
	}

	// A moved rect with the same gradient changes too: the op carries the rect.
	var d scene.List
	gradientFrame(d.Recorder(nil), g, 26)
	damage, changed := d.Diff(&a, m)
	if !changed {
		t.Fatal("a moved gradient rect must report change")
	}
	if want := geom.RectXYWH(30, 20, 140, 106); damage != want {
		t.Fatalf("damage = %v, want both rects' union %v", damage, want)
	}
}

// TestDamagedGradientReplayMatchesFullRepaint is the damage oracle for the new
// op, at both scales. A gradient op is bounds-culled like any other fill, and
// the brush it replays through samples in physical pixels, so this is where a
// 1x/2x mistake in the op would surface.
func TestDamagedGradientReplayMatchesFullRepaint(t *testing.T) {
	g := ramp()
	for _, scale := range []float32{1, 2} {
		var a, b scene.List
		gradientFrame(a.Recorder(nil), g, 20)
		gradientFrame(b.Recorder(nil), g, 26)

		direct := renderAt(t, scale, func(c paint.Canvas) { gradientFrame(c, g, 26) })
		incremental := renderWithAt(t, scale, func(c paint.Canvas, m scene.Measurer) {
			a.Replay(c)
			damage, changed := b.Diff(&a, m)
			if !changed {
				t.Fatal("a moved gradient must report change")
			}
			c.PushClip(damage)
			b.ReplayDamage(c, damage, m)
			c.PopClip()
		})
		if !bytes.Equal(direct, incremental) {
			t.Errorf("at %gx: damaged replay of a gradient diverged from a full "+
				"repaint", scale)
		}
	}
}

// TestFillRRectGradientStillRecordsItsOwnOp: the original two-stop
// axis-aligned Canvas call predates all of this and must be untouched by it —
// same op, same identity-free comparison by value, same pixels.
func TestFillRRectGradientStillRecordsItsOwnOp(t *testing.T) {
	frame := func(c paint.Canvas, to paint.Color) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.RGB(0.1, 0.1, 0.12))
		c.FillRRectGradient(geom.RectXYWH(30, 20, 140, 100), 10,
			paint.RGB(0.2, 0.4, 1), to, false)
	}

	var a scene.List
	frame(a.Recorder(nil), paint.RGB(1, 0.4, 0.2))
	if a.Len() != 2 {
		t.Fatalf("recorded %d ops, want the background plus one gradient op", a.Len())
	}

	m := measurer(t)
	var same scene.List
	frame(same.Recorder(nil), paint.RGB(1, 0.4, 0.2))
	if _, changed := same.Diff(&a, m); changed {
		t.Fatal("an identical two-stop gradient must diff as unchanged; it " +
			"compares by value, not by identity")
	}

	var b scene.List
	frame(b.Recorder(nil), paint.RGB(0.2, 1, 0.4))
	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("a changed end colour must report change")
	}
	if want := geom.RectXYWH(30, 20, 140, 100); damage != want {
		t.Fatalf("damage = %v, want the gradient rect %v", damage, want)
	}

	direct := render(t, func(c paint.Canvas) { frame(c, paint.RGB(0.2, 1, 0.4)) })
	replayed := render(t, func(c paint.Canvas) { b.Replay(c) })
	if !bytes.Equal(direct, replayed) {
		t.Fatal("FillRRectGradient no longer replays to the same pixels")
	}
}

// TestGradientBlendsTogether: the two features meet when a gradient is painted
// under a blend mode, which is the obvious use (a multiplied sheen). Both are
// recorded ops now, so this checks the op stream keeps them in the right order
// and that the pixels survive a damaged replay.
func TestGradientBlendsTogether(t *testing.T) {
	g := ramp()
	frame := func(c paint.Canvas, y float32) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), dstColor())
		c.FillRect(geom.RectXYWH(20, y, 160, 60), srcColor())
		b := paint.Blends(c)
		if b == nil {
			t.Fatalf("%T does not implement paint.Blender", c)
		}
		b.PushBlend(paint.BlendMultiply)
		paint.FillGradient(c, geom.RectXYWH(30, 30, 140, 100), 10, g)
		b.PopBlend()
	}

	var a, b scene.List
	frame(a.Recorder(nil), 40)
	frame(b.Recorder(nil), 48)

	direct := render(t, func(c paint.Canvas) { frame(c, 48) })
	incremental := renderWith(t, func(c paint.Canvas, m scene.Measurer) {
		a.Replay(c)
		damage, changed := b.Diff(&a, m)
		if !changed {
			t.Fatal("a moved rect under a blended gradient must report change")
		}
		c.PushClip(damage)
		b.ReplayDamage(c, damage, m)
		c.PopClip()
	})
	if !bytes.Equal(direct, incremental) {
		t.Fatal("a damaged replay of a blended gradient diverged from a full repaint")
	}
}
