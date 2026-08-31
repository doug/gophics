// Package renderref is a shared reference scene for the render-correctness
// harness: one widget that exercises every paint primitive, with content
// deliberately reaching all four quadrants (including the bottom-right corner)
// and inside clip/opacity/transform groups. It backs two checks:
//
//   - CPU scale-consistency (app/rendermatrix_test.go): the same logical scene
//     rendered at 1×/2×/3× must be structurally identical once resolution is
//     normalized — this catches HiDPI-dependent bugs (e.g. an opacity layer
//     sized to logical instead of physical pixels, which silently dropped
//     bottom/right content at 2×).
//   - GPU==CPU parity (app/gpu_equiv_test.go): both backends agree per pixel.
//
// The scene is deterministic and fixed to the SceneSize logical dimensions.
package renderref

import (
	"image"
	"image/color"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// SceneSize is the logical size the scene is authored for.
var SceneSize = geom.Size{W: 320, H: 460}

// Background is the scene's clear color.
func Background() paint.Color { return paint.RGB(0.97, 0.97, 0.98) }

// Scene returns the reference widget.
func Scene() widget.Widget { return widget.Canvas{Draw: draw} }

var atlas = buildAtlas()

func draw(c paint.Canvas, sz geom.Size) {
	ink := paint.RGB(0.10, 0.11, 0.14)
	c.Clear(Background())

	// --- Row 1: solid + rounded fills, gradients (both axes), stroke ---
	c.FillRect(geom.RectXYWH(12, 12, 64, 44), paint.RGB(0.86, 0.24, 0.28))
	c.FillRRect(geom.RectXYWH(84, 12, 64, 44), 12, paint.RGB(0.24, 0.46, 0.88))
	c.FillRRectGradient(geom.RectXYWH(156, 12, 70, 44), 10,
		paint.RGB(0.20, 0.85, 0.90), paint.RGB(0.90, 0.30, 0.55), true)
	c.FillRRectGradient(geom.RectXYWH(234, 12, 70, 44), 10,
		paint.RGB(0.95, 0.80, 0.25), paint.RGB(0.30, 0.75, 0.45), false)
	c.StrokeRRect(geom.RectXYWH(12, 64, 292, 30), 10, 3, paint.RGB(0.15, 0.6, 0.3))

	// --- Row 2: lines, filled path, stroked path ---
	c.Line(geom.Pt{X: 12, Y: 104}, geom.Pt{X: 304, Y: 104}, 2, paint.RGB(0.5, 0.5, 0.6))
	tri := paint.NewPath()
	tri.MoveTo(geom.Pt{X: 12, Y: 170}).LineTo(geom.Pt{X: 76, Y: 170}).
		LineTo(geom.Pt{X: 44, Y: 116}).Close()
	c.FillPath(tri, paint.RGB(0.95, 0.55, 0.12))
	zig := paint.NewPath()
	zig.MoveTo(geom.Pt{X: 90, Y: 168}).LineTo(geom.Pt{X: 130, Y: 120}).
		LineTo(geom.Pt{X: 170, Y: 168}).LineTo(geom.Pt{X: 210, Y: 120})
	c.StrokePath(zig, 5, paint.RGB(0.15, 0.5, 0.72))

	// --- Row 2b: image + sprites (plain / flip / tint / rotate) ---
	c.Image(atlas, geom.RectXYWH(230, 116, 32, 32))
	c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(0, 0, 8, 8), Dst: geom.RectXYWH(230, 152, 16, 16), Nearest: true})
	c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(8, 0, 16, 8), Dst: geom.RectXYWH(250, 152, 16, 16), Nearest: true, FlipX: true})
	c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(0, 8, 8, 16), Dst: geom.RectXYWH(270, 152, 16, 16), Nearest: true, Tint: paint.RGB(0.6, 0.6, 1)})
	c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(8, 8, 16, 16), Dst: geom.RectXYWH(288, 150, 18, 18), Nearest: true, Rotation: 0.6})

	// --- Row 3: nested clips (rrect ∩ rect) ---
	c.PushClipRRect(geom.RectXYWH(12, 186, 140, 66), 18)
	c.PushClip(geom.RectXYWH(12, 186, 100, 66))
	c.FillRRectGradient(geom.RectXYWH(12, 186, 140, 66), 0,
		paint.RGB(0.55, 0.30, 0.75), paint.RGB(0.20, 0.55, 0.85), false)
	c.PopClip()
	c.PopClip()

	// --- Row 3b: transform (rotate+scale about a pivot) ---
	c.PushTransform(paint.Transform{TX: 0, TY: 0, SX: 1.1, SY: 1.1, Rotation: 0.3, PivotX: 232, PivotY: 219})
	c.FillRRect(geom.RectXYWH(206, 193, 52, 52), 8, paint.RGB(0.90, 0.45, 0.20))
	c.DrawSprite(atlas, paint.Sprite{Src: image.Rect(8, 8, 16, 16), Dst: geom.RectXYWH(218, 205, 28, 28), Nearest: true})
	c.PopTransform()

	// --- Text at three sizes ---
	c.TextIn("", "The quick brown fox 0123", geom.Pt{X: 12, Y: 274}, 12, ink)
	c.TextIn("", "Render matrix", geom.Pt{X: 12, Y: 296}, 18, ink)
	c.TextIn("bold", "Sharp?", geom.Pt{X: 12, Y: 322}, 24, ink)

	// --- Opacity tiles covering the whole bottom band, INCLUDING the
	// bottom-right corner. This is the HiDPI-clipping tripwire: a per-layer
	// pixmap sized to logical pixels loses tiles past the top-left region. ---
	cols, rows := 5, 2
	tw, th := float32(56), float32(52)
	x0, y0 := float32(12), float32(340)
	for r := range rows {
		for col := range cols {
			x := x0 + float32(col)*(tw+2)
			y := y0 + float32(r)*(th+4)
			base := paint.RGB(0.2+0.13*float32(col), 0.35+0.1*float32(r), 0.85-0.09*float32(col))
			c.PushOpacity(0.55)
			c.FillRRect(geom.RectXYWH(x, y, tw, th), 10, base)
			c.PopOpacity()
			// A second opacity group nested-in-sequence per tile (a distinct
			// glyph), so each tile carries two layer pushes like real UIs.
			c.PushOpacity(0.9)
			s := float32(9)
			cx, cy := x+tw/2, y+th/2
			glyph := paint.NewPath()
			glyph.MoveTo(geom.Pt{X: cx, Y: cy - s}).LineTo(geom.Pt{X: cx + s, Y: cy}).
				LineTo(geom.Pt{X: cx, Y: cy + s}).LineTo(geom.Pt{X: cx - s, Y: cy}).Close()
			c.FillPath(glyph, paint.RGB(1, 1, 1))
			c.PopOpacity()
		}
	}
}

// buildAtlas is a 16×16 image with four solid 8×8 color quadrants.
func buildAtlas() *image.RGBA {
	a := image.NewRGBA(image.Rect(0, 0, 16, 16))
	quad := [4]color.RGBA{
		{220, 70, 70, 255}, {70, 170, 220, 255}, {80, 200, 120, 255}, {230, 200, 80, 255},
	}
	for y := range 16 {
		for x := range 16 {
			a.SetRGBA(x, y, quad[(y/8)*2+(x/8)])
		}
	}
	return a
}

// The performance corpus.
//
// Three scenes beside Scene(), chosen because the tier table in that plan's
// §1.1 predicts they diverge: strokes and curved fills are claimed to land in
// the stencil tier, text in the glyph tiers. If they do not diverge, the tier
// model is wrong, and that is the most useful thing the corpus could report.
//
// They live here rather than in a benchmark package so that correctness and
// performance are measured on the same content: TestGPUMatchesCPU and
// TestRenderScaleConsistency pick these up for free, which means a scene
// written to be fast cannot quietly stop being correct.
//
// All three are authored to SceneSize and are deterministic.

// StrokeHeavy is dividers, borders, focus rings and chart lines — the shapes
// §1.2 claims reach tier 2b unconditionally, because preTessellateStroke sets
// EvenOdd and preTessellateFill skips the convex test for EvenOdd.
func StrokeHeavy() widget.Widget { return widget.Canvas{Draw: drawStrokeHeavy} }

func drawStrokeHeavy(c paint.Canvas, sz geom.Size) {
	c.Clear(Background())
	ink := paint.RGB(0.35, 0.38, 0.45)

	// Dividers: the single commonest stroke in a real UI.
	for i := range 24 {
		y := float32(14 + i*8)
		c.Line(geom.Pt{X: 10, Y: y}, geom.Pt{X: 310, Y: y}, 1, ink)
	}
	// Borders and focus rings.
	for i := range 8 {
		f := float32(i)
		c.StrokeRRect(geom.RectXYWH(12+f*4, 214+f*4, 180-f*8, 90-f*8), 8, 2,
			paint.RGB(0.20+f*0.08, 0.45, 0.85-f*0.06))
	}
	// Chart lines: open polylines, which is the case Phase C2 argues can reach
	// tier 2a once the EvenOdd gate stops rejecting it untested.
	for s := range 6 {
		p := paint.NewPath()
		base := float32(320 + s*20)
		p.MoveTo(geom.Pt{X: 14, Y: base})
		for i := 1; i < 20; i++ {
			x := 14 + float32(i)*15
			y := base - float32((i*7+s*11)%18)
			p.LineTo(geom.Pt{X: x, Y: y})
		}
		c.StrokePath(p, 2, paint.RGB(0.85-float32(s)*0.1, 0.30, 0.35+float32(s)*0.09))
	}
}

// CurveHeavy is filled curved paths — the other half of what §1.2 says reaches
// tier 2b, being neither SDF-detectable shapes nor curve-free convex polygons.
func CurveHeavy() widget.Widget { return widget.Canvas{Draw: drawCurveHeavy} }

func drawCurveHeavy(c paint.Canvas, sz geom.Size) {
	c.Clear(Background())
	for row := range 6 {
		for col := range 4 {
			x := float32(14 + col*76)
			y := float32(14 + row*74)
			p := paint.NewPath()
			// A closed blob: four quadratic arcs, so it is curved, closed and
			// single-contour — SDF shape detection cannot claim it and the
			// convex extractor rejects it on hasCurves.
			p.MoveTo(geom.Pt{X: x, Y: y + 30})
			p.QuadTo(geom.Pt{X: x, Y: y}, geom.Pt{X: x + 30, Y: y})
			p.QuadTo(geom.Pt{X: x + 62, Y: y}, geom.Pt{X: x + 62, Y: y + 30})
			p.QuadTo(geom.Pt{X: x + 62, Y: y + 60}, geom.Pt{X: x + 30, Y: y + 60})
			p.QuadTo(geom.Pt{X: x, Y: y + 60}, geom.Pt{X: x, Y: y + 30})
			p.Close()
			f := float32(row*4+col) / 24
			c.FillPath(p, paint.RGB(0.25+f*0.6, 0.45, 0.85-f*0.5))
		}
	}
}

// TextHeavy is long runs at mixed sizes, cold and warm in the glyph atlas —
// the tier-6 case, and the one where the first frame differs most from the
// rest because every glyph is rasterized once.
func TextHeavy() widget.Widget { return widget.Canvas{Draw: drawTextHeavy} }

func drawTextHeavy(c paint.Canvas, sz geom.Size) {
	c.Clear(Background())
	ink := paint.RGB(0.12, 0.13, 0.16)
	lines := []string{
		"The quick brown fox jumps over the lazy dog",
		"Sphinx of black quartz, judge my vow",
		"Pack my box with five dozen liquor jugs",
		"How vexingly quick daft zebras jump",
	}
	y := float32(20)
	for size := 9; size <= 15; size += 2 {
		for _, s := range lines {
			c.TextIn("", s, geom.Pt{X: 10, Y: y}, float32(size), ink)
			y += float32(size) + 4
		}
	}
}

// UIScreen is a realistic application screen built from the theme's own
// widgets, as opposed to the primitive-coverage scene above.
//
// It exists because the corpus needed a case that is neither a stress test nor
// a coverage fixture. Scene() reaches every paint primitive by construction,
// which makes it excellent for correctness and misleading for tier populations:
// it is dense in the rounded rectangles the SDF tier claims and thin in the
// strokes and curves the stencil tier claims. A plan that reasons about "most
// of a UI" needs a UI.
func UIScreen() widget.Widget { return uiScreen{} }

type uiScreen struct{}

func (uiScreen) Build(ctx widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, 0, 24)
	rows = append(rows,
		widget.Text{S: "Settings", Font: "bold", Size: 22},
		widget.Sized{H: 12},
	)
	for i := range 6 {
		rows = append(rows,
			theme.Card{Child: widget.Column(
				widget.Text{S: []string{
					"Appearance", "Notifications", "Privacy",
					"Storage", "Accounts", "About",
				}[i], Font: "bold", Size: 15},
				widget.Sized{H: 4},
				widget.Text{S: "Configure how this section behaves.", Size: 12},
				widget.Sized{H: 8},
				theme.Divider{},
				widget.Sized{H: 8},
				widget.Row(
					theme.Button{Label: "Edit"},
					widget.Sized{W: 8},
					theme.Button{Label: "Reset", Primary: true},
				),
			)},
			widget.Sized{H: 10},
		)
	}
	return widget.Decorated{Color: Background(),
		Child: widget.Padding{Insets: geom.InsetsAll(12), Child: widget.Column(rows...)}}
}
