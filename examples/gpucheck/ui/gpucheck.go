// Package gpucheck is a diagnostic scene for verifying the mobile GPU present
// path on a real device. Everything about it is meant to be read at a glance:
//
//   - the four color swatches confirm solid fills render the right colors;
//   - the gradient bar confirms GPU gradients;
//   - text at three sizes confirms glyph compositing (the LoadOpClear-wipe risk);
//   - the sprite trio confirms DrawSprite (plain / tinted / rotated);
//   - the spinning square + rising frame counter confirm per-frame present;
//   - tapping drops a marker and bumps the tap counter, confirming touch input.
//
// If the surface renders, animates, and responds to touch, the mobile GPU path
// works. Compare a device screenshot against the desktop-GPU golden.
package gpucheck

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Root is the scene widget; Background is its clear color.
func Root() widget.Widget     { return Check{} }
func Background() paint.Color { return paint.RGB(0.06, 0.07, 0.10) }

// Config is the bring-up scene's own configuration, so a mobile build can swap
// this in for the real app's without either side hand-rolling an app.Config.
func Config() app.Config {
	return app.Config{
		Title:      "gophics · gpucheck",
		Background: Background(),
		Font:       goregular.TTF,
	}
}

type Check struct{}

func (Check) CreateState() widget.State { return &checkState{} }

type checkState struct {
	widget.StateBase[Check]
	ctx     widget.Ctx
	atlas   *image.RGBA
	t       float64
	frames  int
	taps    int
	lastTap geom.Pt
}

func (s *checkState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.atlas = buildAtlas()
	ctx.AddTicker(tick{s})
}

type tick struct{ s *checkState }

func (t tick) Tick(dt float64) bool {
	t.s.t += dt
	t.s.frames++
	t.s.ctx.Invalidate()
	return true
}

func (s *checkState) Build(_ widget.Ctx) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{OnPress: func(p geom.Pt) {
			s.taps++
			s.lastTap = p
			s.ctx.Invalidate()
		}},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
}

var (
	ink  = paint.RGB(0.90, 0.92, 0.96)
	dim  = paint.RGB(0.55, 0.60, 0.68)
	good = paint.RGB(0.30, 0.80, 0.45)
)

func (s *checkState) draw(c paint.Canvas, size geom.Size) {
	c.Clear(Background())
	x, y := float32(20), float32(24)

	c.TextIn("bold", "GPU CHECK", geom.Pt{X: x, Y: y + 8}, 30, ink)
	c.TextIn("", fmt.Sprintf("frames %d   taps %d", s.frames, s.taps), geom.Pt{X: x, Y: y + 36}, 15, dim)
	y += 64

	// Color swatches — fills + color correctness.
	for i, col := range []paint.Color{
		paint.RGB(0.86, 0.24, 0.26), paint.RGB(0.26, 0.72, 0.40),
		paint.RGB(0.28, 0.50, 0.92), paint.RGB(0.95, 0.95, 0.97),
	} {
		c.FillRRect(geom.RectXYWH(x+float32(i)*66, y, 56, 40), 8, col)
	}
	y += 56

	// Gradient bar — GPU gradients.
	c.FillRRectGradient(geom.RectXYWH(x, y, 4*66-10, 30), 8,
		paint.RGB(0.20, 0.85, 0.90), paint.RGB(0.90, 0.30, 0.60), true)
	y += 46

	// Text at three sizes — glyph compositing.
	c.TextIn("", "The quick brown fox 0123", geom.Pt{X: x, Y: y + 12}, 12, ink)
	c.TextIn("", "The quick brown fox", geom.Pt{X: x, Y: y + 34}, 18, ink)
	c.TextIn("bold", "Sharp?", geom.Pt{X: x, Y: y + 64}, 26, ink)
	y += 84

	// Sprite trio — DrawSprite plain / tinted / rotated.
	q := image.Rect(0, 0, 8, 8)
	c.DrawSprite(s.atlas, paint.Sprite{Src: q, Dst: geom.RectXYWH(x, y, 48, 48), Nearest: true})
	c.DrawSprite(s.atlas, paint.Sprite{Src: q, Dst: geom.RectXYWH(x+64, y, 48, 48), Nearest: true, Tint: paint.RGB(1, 0.5, 0.2)})
	c.DrawSprite(s.atlas, paint.Sprite{Src: q, Dst: geom.RectXYWH(x+128, y, 48, 48), Nearest: true, Rotation: float32(s.t)})
	c.TextIn("", "sprite · tint · rotate", geom.Pt{X: x, Y: y + 62}, 13, dim)
	y += 84

	// Filled path (triangle) + a spinning square — animation + paths.
	tri := paint.NewPath()
	tri.MoveTo(geom.Pt{X: x, Y: y + 44}).LineTo(geom.Pt{X: x + 48, Y: y + 44}).LineTo(geom.Pt{X: x + 24, Y: y}).Close()
	c.FillPath(tri, good)
	spin := paint.NewPath()
	cx, cy, r := x+120, y+24, float32(22)
	for i := 0; i <= 4; i++ {
		a := s.t + float64(i)*math.Pi/2
		p := geom.Pt{X: cx + r*float32(math.Cos(a)), Y: cy + r*float32(math.Sin(a))}
		if i == 0 {
			spin.MoveTo(p)
		} else {
			spin.LineTo(p)
		}
	}
	c.FillPath(spin.Close(), paint.RGB(0.95, 0.72, 0.30))
	c.TextIn("", "path fill · spinning = frames advancing", geom.Pt{X: x + 170, Y: y + 28}, 13, dim)
	y += 76

	// Opacity groups (GPU saveLayer). The green base must SURVIVE under the
	// half-opacity magenta overlay (the old bug lost it), the overlay must be
	// ~50% (over green → blend, over dark bg → muted), and the nested blue must
	// be ~25% (a 0.5 group inside a 0.5 group). If the base vanishes or the
	// overlay is fully opaque, GPU layer compositing is broken.
	c.TextIn("", "opacity: base survives · overlay 50% · nested 25%", geom.Pt{X: x, Y: y}, 13, dim)
	oy := y + 12
	c.FillRRect(geom.RectXYWH(x, oy, 120, 58), 8, good) // opaque green base
	c.PushOpacity(0.5)
	c.FillRRect(geom.RectXYWH(x+60, oy+14, 210, 44), 8, paint.RGB(0.95, 0.30, 0.60)) // 50% magenta
	c.PushOpacity(0.5)
	c.FillRRect(geom.RectXYWH(x+170, oy+6, 90, 52), 8, paint.RGB(0.30, 0.55, 0.95)) // 25% blue (nested)
	c.PopOpacity()
	c.PopOpacity()
	y = oy + 70

	// Backdrop blur (glass) — the right half of a colorful backdrop is frosted.
	// On the GPU path this exercises the offscreen downsample + composite; the
	// left half stays sharp for an A/B. If the right half isn't blurred, the GPU
	// backdrop path is falling back to a plain tint.
	c.TextIn("", "backdrop blur (glass): right half frosted", geom.Pt{X: x, Y: y}, 13, dim)
	by := y + 10
	c.FillRRectGradient(geom.RectXYWH(x, by, 280, 60), 10,
		paint.RGB(0.95, 0.42, 0.36), paint.RGB(0.28, 0.52, 0.95), true)
	c.FillRRect(geom.RectXYWH(x+28, by+8, 44, 44), 22, paint.RGB(0.24, 0.80, 0.42))
	c.FillRRect(geom.RectXYWH(x+150, by+6, 48, 48), 24, paint.RGB(0.96, 0.82, 0.24))
	gr := geom.RectXYWH(x+140, by, 140, 60)
	c.PushClipRRect(gr, 10)
	c.BackdropBlur(gr, 16)
	c.PopClip()
	c.FillRRect(gr, 10, paint.Color{R: 1, G: 1, B: 1, A: 0.32})
	c.StrokeRRect(gr, 10, 1, paint.Color{R: 1, G: 1, B: 1, A: 0.55})

	// Tap marker.
	if s.taps > 0 {
		c.FillRRect(geom.RectXYWH(s.lastTap.X-10, s.lastTap.Y-10, 20, 20), 10, paint.RGB(1, 0.9, 0.3))
	}
	c.TextIn("", "tap anywhere → marker + tap count (touch input)",
		geom.Pt{X: 20, Y: size.H - 20}, 13, dim)

}

// buildAtlas is a tiny 16×16 atlas with four solid color quadrants.
func buildAtlas() *image.RGBA {
	a := image.NewRGBA(image.Rect(0, 0, 16, 16))
	quad := [4]color.RGBA{{220, 70, 70, 255}, {70, 170, 220, 255}, {80, 200, 120, 255}, {230, 200, 80, 255}}
	for yy := 0; yy < 16; yy++ {
		for xx := 0; xx < 16; xx++ {
			a.SetRGBA(xx, yy, quad[(yy/8)*2+(xx/8)])
		}
	}
	return a
}
