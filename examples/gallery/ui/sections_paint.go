package ui

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Paint: gradients, blend modes, measured text ----------------------------

// blendShown is the subset of paint.BlendMode the sheet draws, in the order the
// enum groups them: separable, non-separable, then Porter-Duff. It is a list
// rather than a loop to numBlendModes for the reason the icon sheet is one —
// the sheet exists to be compared, and thirty tiles is a wall, not a
// comparison. These are the modes an app actually reaches for.
var blendShown = []paint.BlendMode{
	paint.BlendNormal,
	paint.BlendMultiply,
	paint.BlendScreen,
	paint.BlendOverlay,
	paint.BlendDarken,
	paint.BlendLighten,
	paint.BlendColorDodge,
	paint.BlendSoftLight,
	paint.BlendDifference,
	paint.BlendExclusion,
	paint.BlendHue,
	paint.BlendColor,
	paint.BlendLuminosity,
	paint.BlendPlus,
	paint.BlendDstOut,
	paint.BlendSrcAtop,
}

// paintSection is the custom-drawing sheet: the three things a widget can do
// on a paint.Canvas that no higher-level widget wraps for it.
//
// It is one section rather than three because they are one question — "what can
// I draw in here" — and because a blend mode needs something underneath it to
// blend against, which the gradient above it supplies.
type paintSection struct{}

func (paintSection) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	return sectionColumn(
		groupLabel("Gradients"),
		theme.Body("Multi-stop linear, radial and sweep ramps. Drawn through "+
			"paint.FillGradient, which uses the canvas's own gradient support "+
			"where it has any and approximates the ramp in bands where it does "+
			"not — so the same call works on every backend."),
		widget.Sized{H: 10},
		widget.Canvas{H: 132, Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
			drawGradientTriptych(c, size, th)
		}},

		groupLabel("Blend modes"),
		theme.Body("Each tile paints the same two swatches over the same ramp, "+
			"changing only the mode. Blending is an opt-in capability: it needs a "+
			"destination to read, so unlike drawing it cannot be approximated, and "+
			"paint.Blends returns nil on a canvas that cannot do it."),
		widget.Sized{H: 10},
		widget.Canvas{H: blendSheetHeight, Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
			drawBlendSheet(c, size, th)
		}},

		groupLabel("Measured text"),
		theme.Body("A canvas measures the text it is about to draw, from the same "+
			"Painter that will shape it — so the box always matches the glyphs. "+
			"The outer rule is the advance box (ascent, descent and advance width); "+
			"the inner one is where the run actually puts ink."),
		widget.Sized{H: 10},
		widget.Canvas{H: 112, Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
			drawMeasuredText(c, size, th)
		}},
		widget.Sized{H: 4},
	)
}

// drawGradientTriptych draws the three gradient kinds side by side, each
// labelled. The panels are sized from the surface rather than fixed, so the
// sheet works at phone width and in a wide window alike.
func drawGradientTriptych(c paint.Canvas, size geom.Size, th theme.Theme) {
	const gap, labelH = 8, 18
	panelW := (size.W - 2*gap) / 3
	panelH := size.H - labelH
	if panelW <= 0 || panelH <= 0 {
		return
	}

	// Stops come from the theme's chart palette, so a re-theme restyles the
	// ramps along with everything else on the page.
	stops := paint.EvenStops(th.Chart[0], th.Chart[2], th.Chart[4])

	for i, name := range [...]string{"Linear", "Radial", "Sweep"} {
		x := float32(i) * (panelW + gap)
		r := geom.Rect{Min: geom.Pt{X: x}, Max: geom.Pt{X: x + panelW, Y: panelH}}
		mid := geom.Pt{X: x + panelW/2, Y: panelH / 2}

		var g *paint.Gradient
		switch i {
		case 0:
			g = paint.LinearGradient(r.Min, geom.Pt{X: r.Max.X, Y: r.Max.Y}, stops...)
		case 1:
			g = paint.RadialGradient(mid, panelW/2, stops...)
		default:
			// A full turn, so the seam where the last stop meets the first is
			// visible — it is the thing to get right in a sweep.
			g = paint.SweepGradient(mid, 0, 2*3.14159265, stops...)
		}
		paint.FillGradient(c, r, 10, g)
		centeredLabel(c, name, geom.Pt{X: x + panelW/2, Y: panelH + 13}, panelW, th)
	}
}

// blendSheetHeight is four rows of tiles plus their labels. It is a constant
// because the Canvas needs a height before Draw runs, and a sheet that sized
// itself from the tile count would have to be laid out twice.
const blendSheetHeight = 4 * (blendTileH + blendLabelH)

const (
	blendTileH   = 56
	blendLabelH  = 16
	blendTileGap = 6
)

// drawBlendSheet draws one tile per mode in blendShown: a gradient backdrop,
// then two overlapping swatches painted inside a PushBlend/PopBlend pair.
//
// The swatches overlap each other as well as the ramp, which is deliberate —
// the mode applies per shape rather than to the group, so the overlap is where
// that shows.
func drawBlendSheet(c paint.Canvas, size geom.Size, th theme.Theme) {
	const cols = 4
	tileW := (size.W - (cols-1)*blendTileGap) / cols
	if tileW <= 0 {
		return
	}

	b := paint.Blends(c)
	backdrop := paint.EvenStops(th.Chart[1], th.Chart[3])

	for i, m := range blendShown {
		col, row := i%cols, i/cols
		x := float32(col) * (tileW + blendTileGap)
		y := float32(row) * (blendTileH + blendLabelH)
		tile := geom.Rect{Min: geom.Pt{X: x, Y: y}, Max: geom.Pt{X: x + tileW, Y: y + blendTileH}}

		paint.FillGradient(c, tile, 8, paint.LinearGradient(
			tile.Min, geom.Pt{X: tile.Max.X, Y: tile.Max.Y}, backdrop...))

		// Clipped to the tile so a mode that spreads (Plus, DstOut) cannot
		// paint over its neighbour and make the sheet unreadable.
		c.PushClipRRect(tile, 8)
		if b != nil {
			b.PushBlend(m)
		}
		var d float32 = blendTileH * 0.46
		c.FillRRect(geom.Rect{
			Min: geom.Pt{X: tile.Min.X + tileW*0.5 - d, Y: tile.Min.Y + blendTileH*0.5 - d*0.82},
			Max: geom.Pt{X: tile.Min.X + tileW*0.5 + d*0.1, Y: tile.Min.Y + blendTileH*0.5 + d*0.82},
		}, d, th.Chart[5])
		c.FillRRect(geom.Rect{
			Min: geom.Pt{X: tile.Min.X + tileW*0.5 - d*0.1, Y: tile.Min.Y + blendTileH*0.5 - d*0.82},
			Max: geom.Pt{X: tile.Min.X + tileW*0.5 + d, Y: tile.Min.Y + blendTileH*0.5 + d*0.82},
		}, d, th.OnPrimary)
		if b != nil {
			b.PopBlend()
		}
		c.PopClip()

		// String() names the mode, so the sheet cannot drift out of step with
		// the enum the way a parallel table of names would.
		centeredLabel(c, m.String(), geom.Pt{X: x + tileW/2, Y: tile.Max.Y + 12}, tileW, th)
	}
}

// drawMeasuredText draws a run with its advance box and its ink box, which is
// the whole point of canvas measurement: the two are different, and a custom
// painter that assumes otherwise clips its own descenders.
func drawMeasuredText(c paint.Canvas, size geom.Size, th theme.Theme) {
	const (
		sample = "Hamburgefonstiv jpq"
		fsize  = 30
	)
	m := c.MetricsIn("", fsize)
	w := c.MeasureWidthIn("", sample, fsize)

	// Centred using the measurement, which is also the simplest demonstration
	// of why a painter wants it: there is no other way to place a run you have
	// not drawn yet.
	origin := geom.Pt{X: (size.W - w) / 2, Y: m.Ascent + 26}
	if origin.X < 0 {
		origin.X = 0
	}

	advance := geom.Rect{
		Min: geom.Pt{X: origin.X, Y: origin.Y - m.Ascent},
		Max: geom.Pt{X: origin.X + w, Y: origin.Y + m.Descent},
	}
	c.StrokeRRect(advance, 2, 1, th.Border)

	if ink, ok := c.InkBoundsIn("", sample, fsize); ok {
		c.StrokeRRect(geom.Rect{
			Min: geom.Pt{X: origin.X + ink.Min.X, Y: origin.Y + ink.Min.Y},
			Max: geom.Pt{X: origin.X + ink.Max.X, Y: origin.Y + ink.Max.Y},
		}, 0, 1, th.Primary)
	}

	// The baseline, so the two boxes can be read against the thing they are
	// both measured from.
	c.Line(geom.Pt{X: advance.Min.X, Y: origin.Y}, geom.Pt{X: advance.Max.X, Y: origin.Y}, 1, th.Muted)
	c.TextIn("", sample, origin, fsize, th.Text)

	centeredLabel(c, "advance box · ink box · baseline",
		geom.Pt{X: size.W / 2, Y: advance.Max.Y + 22}, size.W, th)
}

// centeredLabel draws s centred on x, measuring it to find the left edge. It
// exists because every sheet above needs it, and because centring text on a
// canvas is exactly the case Measurer is for.
func centeredLabel(c paint.Canvas, s string, at geom.Pt, maxW float32, th theme.Theme) {
	const fsize = 10
	w := c.MeasureWidthIn("", s, fsize)
	if w > maxW {
		// Too long to centre honestly: left-align rather than bleed into the
		// neighbouring tile.
		c.TextIn("", s, geom.Pt{X: at.X - maxW/2, Y: at.Y}, fsize, th.Muted)
		return
	}
	c.TextIn("", s, geom.Pt{X: at.X - w/2, Y: at.Y}, fsize, th.Muted)
}
