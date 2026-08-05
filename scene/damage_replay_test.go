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
