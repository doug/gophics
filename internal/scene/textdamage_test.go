package scene_test

import (
	"bytes"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/paint"
)

// Text damage has to cover where the run really paints, not just its metrics
// box: an accented capital rises above the ascender and the run cache blits
// the ink with a pad around it. Damage sized from the metrics alone left the
// tops of the accents stale on an incremental replay.
func TestDamageReplayCoversTextInkAboveTheAscender(t *testing.T) {
	frame := func(c paint.Canvas, s string) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.RGB(1, 1, 1))
		c.TextIn("", s, geom.Pt{X: 20, Y: 80}, 40, paint.RGB(0, 0, 0))
	}
	var a, b scene.List
	frame(a.Recorder(), "ǺǺǺǺ")
	frame(b.Recorder(), "OOOO")
	damage, changed := b.Diff(&a, measurer(t))
	if !changed {
		t.Fatal("changed text must report change")
	}
	direct := render(t, func(c paint.Canvas) { frame(c, "OOOO") })
	incremental := renderWith(t, func(c paint.Canvas, m scene.Measurer) {
		a.Replay(c)
		c.PushClip(damage)
		b.ReplayDamage(c, damage, m)
		c.PopClip()
	})
	if !bytes.Equal(direct, incremental) {
		t.Errorf("incremental replay after replacing accented text diverged from a full repaint: damage %v leaves the accents stale", damage)
	}
}
