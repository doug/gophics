package main

import (
	"fmt"

	"github.com/doug/gossamer/examples/solitaire/klondike"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

var (
	colFelt   = paint.RGB(0.10, 0.44, 0.30)
	colFeltHi = paint.RGB(0.13, 0.50, 0.34)
	colFeltLo = paint.RGB(0.06, 0.33, 0.22)
	colFace   = paint.RGB(0.99, 0.99, 0.98)
	colEdge   = paint.Color{R: 0, G: 0, B: 0, A: 0.10} // subtle card outline
	colShadow = paint.Color{R: 0, G: 0, B: 0, A: 0.28}
	colRed    = paint.RGB(0.79, 0.13, 0.17)
	colBlack  = paint.RGB(0.11, 0.12, 0.15)
	colBack1  = paint.RGB(0.28, 0.42, 0.72)
	colBack2  = paint.RGB(0.12, 0.22, 0.46)
	colBack3  = paint.RGB(0.40, 0.54, 0.82) // back motif
	colSlot   = paint.Color{R: 1, G: 1, B: 1, A: 0.14}
)

func suitColor(s klondike.Suit) paint.Color {
	if s.Red() {
		return colRed
	}
	return colBlack
}

// suitGlyph returns the Unicode pip; goregular includes all four.
func suitGlyph(s klondike.Suit) string {
	switch s {
	case klondike.Club:
		return "♣"
	case klondike.Diamond:
		return "♦"
	case klondike.Heart:
		return "♥"
	default:
		return "♠"
	}
}

func rankLabel(r uint8) string {
	switch r {
	case 1:
		return "A"
	case 11:
		return "J"
	case 12:
		return "Q"
	case 13:
		return "K"
	default:
		return fmt.Sprintf("%d", r)
	}
}

// drawCard paints one card in r (face up, or a gradient back), with a soft
// drop shadow for depth.
func drawCard(c paint.Canvas, r geom.Rect, card klondike.Card) {
	sz := r.Dx()
	paint.DropShadow(c, r, sz*0.08, geom.Pt{Y: sz * 0.02}, sz*0.05, colShadow)
	drawCardBody(c, r, card)
}

// drawCardBody paints the card without a shadow (used for the many win-cascade
// trail stamps, where per-card shadows would be too costly).
func drawCardBody(c paint.Canvas, r geom.Rect, card klondike.Card) {
	sz := r.Dx()
	rad := sz * 0.08
	if !card.Up {
		c.FillRRectGradient(r, rad, colBack1, colBack2, false)
		// A centered diamond motif inside a hairline frame.
		cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
		c.PushTransform(paint.Transform{Rotation: 0.7853982, PivotX: cx, PivotY: cy})
		c.FillRect(geom.RectXYWH(cx-sz*0.15, cy-sz*0.15, sz*0.30, sz*0.30), colBack3)
		c.PopTransform()
		return
	}
	c.FillRRect(r, rad, colFace)
	c.StrokeRRect(r, rad, 1, colEdge)

	col := suitColor(card.Suit)
	glyph := suitGlyph(card.Suit)
	rl := rankLabel(card.Rank)

	// Two opposing corner indices (top-left, and bottom-right rotated 180°),
	// like a real deck — the second one shows on face-up tops (waste/foundation).
	drawCorner(c, r, rl, glyph, col)
	cx, cy := r.Min.X+sz/2, r.Min.Y+r.Dy()/2
	c.PushTransform(paint.Transform{Rotation: pi, PivotX: cx, PivotY: cy})
	drawCorner(c, r, rl, glyph, col)
	c.PopTransform()

	switch {
	case card.Rank >= 2 && card.Rank <= 10:
		// The traditional pip arrangement: N symbols laid out in the standard
		// grid, with the lower-half pips rotated 180° as on a printed card.
		xs := [3]float32{r.Min.X + sz*0.30, cx, r.Max.X - sz*0.30}
		ps := sz * 0.19
		for _, p := range pipLayout[card.Rank] {
			y := r.Min.Y + r.Dy()*p.y
			pip(c, glyph, xs[p.col], y, ps, col, p.y > 0.5)
		}
	case card.Rank == 1:
		// Ace: one large central pip.
		pip(c, glyph, cx, cy, sz*0.5, col, false)
	default:
		// Court cards: a large rank letter over its suit.
		centerGlyph(c, rl, cx, r.Min.Y+r.Dy()*0.46, sz*0.5, col)
		centerGlyph(c, glyph, cx, r.Min.Y+r.Dy()*0.72, sz*0.26, col)
	}
}

// drawCorner paints the top-left rank index over a small pip. Kept compact so a
// fanned card still reveals it (the fan offset is ~0.42·sz — see Layout).
func drawCorner(c paint.Canvas, r geom.Rect, rl, glyph string, col paint.Color) {
	sz := r.Dx()
	centerGlyph(c, rl, r.Min.X+sz*0.15, r.Min.Y+sz*0.19, sz*0.20, col)
	centerGlyph(c, glyph, r.Min.X+sz*0.15, r.Min.Y+sz*0.36, sz*0.15, col)
}

// pip draws a suit symbol centered at (ax, ay), optionally rotated 180° (as the
// lower-half pips are printed on a real card).
func pip(c paint.Canvas, glyph string, ax, ay, size float32, col paint.Color, flip bool) {
	if flip {
		c.PushTransform(paint.Transform{Rotation: pi, PivotX: ax, PivotY: ay})
		centerGlyph(c, glyph, ax, ay, size, col)
		c.PopTransform()
		return
	}
	centerGlyph(c, glyph, ax, ay, size, col)
}

// centerGlyph draws s centered on (ax, ay). The Canvas has no measure API, so it
// uses fixed fractions calibrated for goregular's near-square suit glyphs and
// digits (baseline-left positioning; pos.Y is the baseline).
func centerGlyph(c paint.Canvas, s string, ax, ay, size float32, col paint.Color) {
	c.Text(s, geom.Pt{X: ax - size*0.30, Y: ay + size*0.36}, size, col)
}

const pi = 3.14159265

// pipPos is a suit-symbol slot: column (0=left, 1=center, 2=right) and a
// vertical fraction of the card height. Lower-half slots (y>0.5) render rotated.
type pipPos struct {
	col int
	y   float32
}

// pipLayout is the standard printed arrangement of N suit symbols for ranks
// 2–10.
var pipLayout = map[uint8][]pipPos{
	2:  {{1, 0.20}, {1, 0.80}},
	3:  {{1, 0.20}, {1, 0.50}, {1, 0.80}},
	4:  {{0, 0.20}, {2, 0.20}, {0, 0.80}, {2, 0.80}},
	5:  {{0, 0.20}, {2, 0.20}, {1, 0.50}, {0, 0.80}, {2, 0.80}},
	6:  {{0, 0.20}, {2, 0.20}, {0, 0.50}, {2, 0.50}, {0, 0.80}, {2, 0.80}},
	7:  {{0, 0.20}, {2, 0.20}, {1, 0.35}, {0, 0.50}, {2, 0.50}, {0, 0.80}, {2, 0.80}},
	8:  {{0, 0.20}, {2, 0.20}, {1, 0.35}, {0, 0.50}, {2, 0.50}, {1, 0.65}, {0, 0.80}, {2, 0.80}},
	9:  {{0, 0.20}, {2, 0.20}, {0, 0.40}, {2, 0.40}, {1, 0.50}, {0, 0.60}, {2, 0.60}, {0, 0.80}, {2, 0.80}},
	10: {{0, 0.20}, {2, 0.20}, {1, 0.30}, {0, 0.40}, {2, 0.40}, {0, 0.60}, {2, 0.60}, {1, 0.70}, {0, 0.80}, {2, 0.80}},
}

// drawStamp paints a cheap card for the win-cascade trail: just the face, a
// hairline edge, and the top-left index — enough to read as a streaking card
// without the cost of a full pip layout across hundreds of stamps per frame.
func drawStamp(c paint.Canvas, r geom.Rect, card klondike.Card) {
	sz := r.Dx()
	c.FillRRect(r, sz*0.08, colFace)
	c.StrokeRRect(r, sz*0.08, 1, colEdge)
	col := suitColor(card.Suit)
	drawCorner(c, r, rankLabel(card.Rank), suitGlyph(card.Suit), col)
}

// drawEmpty paints a ghost slot where a pile can be placed.
func drawEmpty(c paint.Canvas, r geom.Rect) {
	c.StrokeRRect(r, r.Dx()*0.08, 1.5, colSlot)
}
