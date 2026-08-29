package scene_test

import (
	"image"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/paint"
)

// recordFrame records a representative ~300-op frame: fills, gradients,
// strokes, lines, text, images, sprites, clips, and one opacity group.
func recordFrame(c paint.Canvas, path *paint.Path) {
	c.FillRect(geom.RectXYWH(0, 0, 800, 600), paint.RGB(0.1, 0.1, 0.12))
	for i := range 30 {
		y := float32(i * 20)
		c.FillRect(geom.RectXYWH(0, y, 800, 20), paint.RGB(0.15, 0.15, 0.18))
		c.FillRRect(geom.RectXYWH(10, y+2, 100, 16), 4, paint.RGB(0.3, 0.3, 0.4))
		c.FillRRectGradient(geom.RectXYWH(120, y+2, 100, 16), 4,
			paint.RGB(0.2, 0.4, 1), paint.RGB(1, 0.4, 0.2), true)
		c.StrokeRRect(geom.RectXYWH(230, y+2, 60, 16), 4, 1, paint.RGB(1, 1, 1))
		c.Line(geom.Pt{X: 300, Y: y}, geom.Pt{X: 360, Y: y + 20}, 1, paint.RGB(1, 1, 0))
		c.TextIn("", "row label", geom.Pt{X: 370, Y: y + 14}, 12, paint.RGB(0.9, 0.9, 0.9))
		c.Image(benchImage, geom.RectXYWH(500, y+2, 16, 16))
		c.DrawSprite(benchImage, paint.Sprite{
			Src: image.Rect(0, 0, 8, 8), Dst: geom.RectXYWH(520, y+2, 16, 16), Alpha: 1,
		})
		c.PushClip(geom.RectXYWH(540, y, 60, 20))
		c.FillPath(path, paint.RGB(0.8, 0.2, 0.2))
		c.PopClip()
	}
	c.PushOpacity(0.5)
	c.FillRRect(geom.RectXYWH(300, 200, 200, 100), 8, paint.RGB(1, 1, 1))
	c.PopOpacity()
}

var benchImage = image.NewRGBA(image.Rect(0, 0, 8, 8))

// BenchmarkRecordFrame measures re-recording a ~300-op frame into a Reset
// list — the steady-state per-frame cost. Allocs/op should be ~0: ops are
// tagged values appended into a capacity-reused slice, not boxed interfaces.
func BenchmarkRecordFrame(b *testing.B) {
	path := paint.NewPath()
	path.MoveTo(geom.Pt{X: 545, Y: 5}).LineTo(geom.Pt{X: 595, Y: 5}).
		LineTo(geom.Pt{X: 570, Y: 15}).Close()

	var list scene.List
	rec := list.Recorder()
	recordFrame(rec, path) // size the op slice
	b.Logf("ops per frame: %d", list.Len())
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		list.Reset()
		recordFrame(rec, path)
	}
}

// BenchmarkDiffUnchanged measures diffing two identical ~300-op frames — the
// per-frame cost of the skip-detection path.
func BenchmarkDiffUnchanged(b *testing.B) {
	path := paint.NewPath()
	path.MoveTo(geom.Pt{X: 545, Y: 5}).LineTo(geom.Pt{X: 595, Y: 5}).
		LineTo(geom.Pt{X: 570, Y: 15}).Close()
	var a, c scene.List
	recordFrame(a.Recorder(), path)
	recordFrame(c.Recorder(), path)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, changed := c.Diff(&a, nil); changed {
			b.Fatal("identical frames must not diff as changed")
		}
	}
}
