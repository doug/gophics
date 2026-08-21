package main

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Everything below draws into a single widget.Canvas — the escape hatch beside
// the widget-built panel. Nothing here is retained between frames except the
// simulation state itself; the matrix is re-recorded each tick, which at 256
// cells is a few hundred fills.

func (s *lum) draw(c paint.Canvas, sz geom.Size) {
	// A square matrix, centred in whatever space the panel left over.
	side := sz.W
	if sz.H < side {
		side = sz.H
	}
	step := side / cols
	if step < 6 {
		return
	}
	side = step * cols
	s.step = step
	s.area = geom.RectXYWH((sz.W-side)/2, (sz.H-side)/2, side, side)

	s.drawField(c, step)
	s.drawRipples(c, step)
	s.drawCells(c, step)
	s.drawCrawlers(c, step)
}

// drawField is the unlit board: a faint dot per cell and a lighter rule every
// four, which is the only thing giving the eye a beat to count against.
func (s *lum) drawField(c paint.Canvas, step float32) {
	for i := 0; i <= cols; i += 4 {
		x := s.area.Min.X + float32(i)*step
		y := s.area.Min.Y + float32(i)*step
		c.FillRect(geom.RectXYWH(x-0.5, s.area.Min.Y, 1, s.area.Dy()), gridLine)
		c.FillRect(geom.RectXYWH(s.area.Min.X, y-0.5, s.area.Dx(), 1), gridLine)
	}
	r := step * 0.055
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			if s.grid[y][x] != empty {
				continue
			}
			cx, cy := s.center(x, y, step)
			c.FillRRect(geom.RectXYWH(cx-r, cy-r, 2*r, 2*r), r, gridDot)
		}
	}
}

func (s *lum) center(x, y int, step float32) (float32, float32) {
	return s.area.Min.X + (float32(x)+0.5)*step, s.area.Min.Y + (float32(y)+0.5)*step
}

// drawRipples draws each strike's expanding ring. The ring thins as it grows
// and fades on a squared curve, so it reads as light spreading out rather than
// a circle being scaled up.
func (s *lum) drawRipples(c paint.Canvas, step float32) {
	for _, rp := range s.ripples {
		rad := step * (0.3 + rp.t*2.4)
		a := (1 - rp.t) * (1 - rp.t) * 0.75 * rp.col.A
		if a < 0.01 {
			continue
		}
		w := step * 0.09 * (1 - rp.t*0.7)
		box := geom.RectXYWH(s.area.Min.X+rp.x*step-rad, s.area.Min.Y+rp.y*step-rad, 2*rad, 2*rad)
		c.StrokeRRect(box, rad, w, rp.col.WithAlpha(a))
	}
}

func (s *lum) drawCells(c paint.Canvas, step float32) {
	for y := 0; y < rows; y++ {
		for x := 0; x < cols; x++ {
			switch s.grid[y][x] {
			case node:
				s.drawNode(c, x, y, step)
			case turnCW:
				s.drawTurn(c, x, y, step, true)
			case turnCCW:
				s.drawTurn(c, x, y, step, false)
			}
		}
	}
}

// drawNode draws a lamp: a dim disc at rest, and while lit a brighter core
// inside a halo, so a struck node blooms without changing size.
func (s *lum) drawNode(c paint.Canvas, x, y int, step float32) {
	cx, cy := s.center(x, y, step)
	f := s.flash[y][x]
	if f > 0 {
		hr := step * (0.32 + 0.22*f)
		c.FillRRect(geom.RectXYWH(cx-hr, cy-hr, 2*hr, 2*hr), hr, nodeCol.WithAlpha(0.22*f))
	}
	r := step * 0.19
	disc(c, cx, cy, r, mix(nodeCol.WithAlpha(0.55), paint.RGB(1, 1, 1), f*0.8))
}

// drawTurn stamps the rotation glyph: an open arc with a head on it, clockwise
// or not. It is handedness, not heading — a turn cell rotates whichever way a
// crawler happens to enter, which is what lets four of them close any
// rectangle into a loop.
//
// The glyph is authored once in a unit square and mapped onto each cell with a
// transform, rather than rebuilt per cell per frame. Paths are retained by the
// display list, so a single mutated path would leave every glyph drawing
// whatever shape was written last; caching two of them sidesteps that and the
// per-frame allocation at the same time.
func (s *lum) drawTurn(c paint.Canvas, x, y int, step float32, cw bool) {
	s.buildGlyphs(step)
	i := 0
	col := cwCol
	if !cw {
		i, col = 1, ccwCol
	}
	cell := geom.RectXYWH(s.area.Min.X+float32(x)*step, s.area.Min.Y+float32(y)*step, step, step)
	c.PushTransform(paint.MapRect(geom.RectXYWH(0, 0, 1, 1), cell))
	c.StrokePath(s.glyphArc[i], glyphStroke, col)
	c.FillPath(s.glyphHead[i], col)
	c.PopTransform()
}

// glyphStroke is the arc's width in the unit square the glyph is authored in;
// the cell transform scales it up with everything else.
const glyphStroke = 0.058

// buildGlyphs (re)builds the two rotation glyphs. They are scale-free, so this
// runs once — the guard is only here because the state starts empty.
func (s *lum) buildGlyphs(float32) {
	if s.glyphArc[0] != nil {
		return
	}
	for i, dir := range []float64{1, -1} {
		const (
			segs    = 22
			radius  = 0.24
			sweep   = 4.36 // 250°, leaving room for the head
			headLen = 0.15
		)
		// The two glyphs are exact mirrors, which means the anticlockwise one
		// starts from the mirrored angle (x → −x maps a → π − a) rather than
		// from the same place; sweeping the other way from a shared start would
		// leave the pair with their gaps in different corners.
		start := -0.5 // radians; puts the gap across the top
		if dir < 0 {
			start = math.Pi + 0.5
		}
		arc := paint.NewPath()
		var tip geom.Pt
		for j := 0; j <= segs; j++ {
			a := start + dir*sweep*float64(j)/segs
			tip = geom.Pt{X: 0.5 + float32(radius*math.Cos(a)), Y: 0.5 + float32(radius*math.Sin(a))}
			if j == 0 {
				arc.MoveTo(tip)
			} else {
				arc.LineTo(tip)
			}
		}
		s.glyphArc[i] = arc

		// The head is a barb on the arc's end, pointing the way the arc travels.
		head := start + dir*sweep + dir*math.Pi/2
		p := paint.NewPath().MoveTo(geom.Pt{
			X: tip.X + float32(headLen*math.Cos(head)),
			Y: tip.Y + float32(headLen*math.Sin(head)),
		})
		for _, spread := range []float64{2.3, -2.3} {
			a := head + spread
			p.LineTo(geom.Pt{
				X: tip.X + float32(headLen*0.95*math.Cos(a)),
				Y: tip.Y + float32(headLen*0.95*math.Sin(a)),
			})
		}
		s.glyphHead[i] = p.Close()
	}
}

// drawCrawlers draws each agent partway between the cell it left and the cell
// it is on, so motion is continuous even though the simulation is a grid step.
// The interpolation runs backwards from the current cell along the direction it
// arrived by, which also makes an edge wrap enter from off-board instead of
// streaking across the width of the matrix.
func (s *lum) drawCrawlers(c paint.Canvas, step float32) {
	frac := float32(1)
	if d := s.stepDur(); s.playing && d > 0 {
		frac = float32(s.acc / d)
		if frac > 1 {
			frac = 1
		}
	}
	for _, cr := range s.crawlers {
		d := delta[cr.from]
		fx := float32(cr.x) + 0.5 - float32(d[0])*(1-frac)
		fy := float32(cr.y) + 0.5 - float32(d[1])*(1-frac)
		cx := s.area.Min.X + fx*step
		cy := s.area.Min.Y + fy*step

		// A short comet tail behind it, three stamps fading back along the step.
		for i := 3; i >= 1; i-- {
			t := float32(i) * 0.26
			tx := cx - float32(d[0])*step*t
			ty := cy - float32(d[1])*step*t
			disc(c, tx, ty, step*(0.12-0.02*float32(i)), cr.col.WithAlpha(0.22-0.05*float32(i)))
		}
		disc(c, cx, cy, step*0.30, cr.col.WithAlpha(0.16)) // halo
		disc(c, cx, cy, step*0.135, cr.col)
	}
}

// disc fills a circle — a rounded rect whose radius is its half-width, which is
// how paint spells a circle.
func disc(c paint.Canvas, cx, cy, r float32, col paint.Color) {
	c.FillRRect(geom.RectXYWH(cx-r, cy-r, 2*r, 2*r), r, col)
}

// mix blends two colours, keeping a's alpha channel.
func mix(a, b paint.Color, t float32) paint.Color {
	return paint.Color{
		R: a.R + (b.R-a.R)*t,
		G: a.G + (b.G-a.G)*t,
		B: a.B + (b.B-a.B)*t,
		A: a.A + (b.A-a.A)*t,
	}
}
