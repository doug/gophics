package main

import (
	"fmt"

	"github.com/doug/gossamer/examples/solitaire/klondike"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

var (
	colFelt   = paint.RGB(0.09, 0.42, 0.28)
	colFeltHi = paint.RGB(0.11, 0.48, 0.32)
	colFace   = paint.RGB(0.99, 0.99, 0.98)
	colEdge   = paint.Color{R: 0, G: 0, B: 0, A: 0.12} // subtle card outline
	colRed    = paint.RGB(0.80, 0.14, 0.18)
	colBlack  = paint.RGB(0.11, 0.12, 0.15)
	colBack1  = paint.RGB(0.24, 0.38, 0.66)
	colBack2  = paint.RGB(0.13, 0.23, 0.46)
	colBack3  = paint.RGB(0.34, 0.48, 0.76) // back motif
	colSlot   = paint.Color{R: 1, G: 1, B: 1, A: 0.13}
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

// drawCard paints one card in r (face up, or a plain gradient back).
func drawCard(c paint.Canvas, r geom.Rect, card klondike.Card) {
	rad := r.Dx() * 0.08
	if !card.Up {
		c.FillRRectGradient(r, rad, colBack1, colBack2, false)
		// A simple centered diamond motif — no inset rectangles.
		sz := r.Dx()
		cx, cy := r.Min.X+r.Dx()/2, r.Min.Y+r.Dy()/2
		c.PushTransform(paint.Transform{Rotation: 0.7853982, PivotX: cx, PivotY: cy})
		c.FillRect(geom.RectXYWH(cx-sz*0.16, cy-sz*0.16, sz*0.32, sz*0.32), colBack3)
		c.PopTransform()
		c.StrokeRRect(r, rad, 1, colEdge)
		return
	}
	c.FillRRect(r, rad, colFace)
	c.StrokeRRect(r, rad, 1, colEdge)

	col := suitColor(card.Suit)
	glyph := suitGlyph(card.Suit)
	sz := r.Dx()
	// Corner: rank over a small pip, kept compact in the top-left so a fanned
	// card reveals the whole corner (the fan offset is ~0.42·sz — see Layout).
	c.Text(rankLabel(card.Rank), geom.Pt{X: r.Min.X + sz*0.10, Y: r.Min.Y + sz*0.25}, sz*0.24, col)
	c.Text(glyph, geom.Pt{X: r.Min.X + sz*0.12, Y: r.Min.Y + sz*0.41}, sz*0.15, col)
	// Center: one large pip (roughly centered; suit glyphs are near-square).
	c.Text(glyph, geom.Pt{X: r.Min.X + sz*0.28, Y: r.Min.Y + r.Dy()*0.64}, sz*0.46, col)
}

// drawEmpty paints a ghost slot where a pile can be placed.
func drawEmpty(c paint.Canvas, r geom.Rect) {
	c.StrokeRRect(r, r.Dx()*0.08, 1.5, colSlot)
}
