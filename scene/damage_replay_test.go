package scene_test

import (
	"bytes"
	"image/png"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/scene"
)

// renderWith renders a draw func that also receives the painter as a Measurer,
// so tests can exercise ReplayDamage.
func renderWith(t *testing.T, draw func(paint.Canvas, scene.Measurer)) []byte {
	t.Helper()
	p := paint.NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	c := p.BeginOffscreen(geom.Size{W: 200, H: 160}, 2)
	draw(c, p)
	var buf bytes.Buffer
	if err := png.Encode(&buf, p.Image()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestDamageReplayKeepsTransformedContent guards the ReplayDamage fix: content
// authored off-surface but brought on-surface by a transform must not be culled
// (opBounds is in record space and can't be compared to surface-space damage
// while a transform is active).
func TestDamageReplayKeepsTransformedContent(t *testing.T) {
	draw := func(c paint.Canvas) {
		c.Clear(paint.RGB(0.1, 0.1, 0.12))
		// Author at negative x (off-surface), then map onto the surface.
		c.PushTransform(paint.MapRect(geom.RectXYWH(-120, 0, 40, 20), geom.RectXYWH(60, 60, 80, 40)))
		c.FillRRect(geom.RectXYWH(-120, 0, 40, 20), 4, paint.RGB(0.9, 0.5, 0.2))
		c.PopTransform()
	}
	direct := render(t, draw) // ground truth: painted directly

	var list scene.List
	draw(list.Recorder())

	surface := geom.RectXYWH(0, 0, 200, 160)
	damaged := renderWith(t, func(c paint.Canvas, m scene.Measurer) {
		c.PushClip(surface)
		list.ReplayDamage(c, surface, m)
		c.PopClip()
	})

	if !bytes.Equal(direct, damaged) {
		t.Fatal("ReplayDamage dropped transform-mapped content that is on-surface")
	}
}

// TestDamageReplayOpacityIncrementalMatchesFull is the M4 correctness oracle:
// after fully replaying frame A (alpha 0.3), partially replaying frame B
// (alpha 0.7) clipped to B.Diff(A) damage must be pixel-identical to a fresh
// full render of frame B. Guards the layer-bounds damage path for animating
// opacity groups.
func TestDamageReplayOpacityIncrementalMatchesFull(t *testing.T) {
	frame := func(c paint.Canvas, alpha float32) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.RGB(0.1, 0.1, 0.12))
		c.FillRRect(geom.RectXYWH(10, 10, 180, 140), 8, paint.RGB(0.2, 0.25, 0.3))
		c.PushOpacity(alpha)
		c.FillRRect(geom.RectXYWH(60, 50, 80, 60), 6, paint.RGB(1, 1, 1))
		c.TextIn("", "fade", geom.Pt{X: 70, Y: 85}, 14, paint.RGB(0.9, 0.2, 0.2))
		c.PopOpacity()
		c.FillRect(geom.RectXYWH(0, 150, 200, 10), paint.RGB(0.4, 0.4, 0.1))
	}

	var a, b scene.List
	frame(a.Recorder(), 0.3)
	frame(b.Recorder(), 0.7)
	if a.HasLayers() || b.HasLayers() {
		t.Fatal("opacity-only frames must allow partial replay")
	}

	direct := render(t, func(c paint.Canvas) { frame(c, 0.7) })

	incremental := renderWith(t, func(c paint.Canvas, m scene.Measurer) {
		a.Replay(c) // frame A in full: the retained surface
		damage, changed := b.Diff(&a, m)
		if !changed {
			t.Fatal("alpha change must report change")
		}
		surface := geom.RectXYWH(0, 0, 200, 160)
		if damage == surface || damage.Dx() >= 200 {
			t.Fatalf("damage %v should be the group's bounds, not the surface", damage)
		}
		c.PushClip(damage)
		b.ReplayDamage(c, damage, m)
		c.PopClip()
	})

	if !bytes.Equal(direct, incremental) {
		t.Fatal("incremental opacity replay diverged from full repaint")
	}
}
