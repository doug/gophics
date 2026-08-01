package main

import (
	"fmt"
	"math"

	"github.com/doug/gossamer/examples/solitaire/klondike"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

var (
	colFelt   = paint.RGB(0.09, 0.42, 0.28)
	colFeltHi = paint.RGB(0.11, 0.48, 0.32)
	colFace   = paint.RGB(0.98, 0.98, 0.97)
	colEdge   = paint.RGB(0.78, 0.80, 0.82)
	colRed    = paint.RGB(0.82, 0.16, 0.20)
	colBlack  = paint.RGB(0.12, 0.13, 0.16)
	colBack1  = paint.RGB(0.20, 0.34, 0.62)
	colBack2  = paint.RGB(0.12, 0.22, 0.44)
	colBackHi = paint.Color{R: 1, G: 1, B: 1, A: 0.14}
	colSlot   = paint.Color{R: 1, G: 1, B: 1, A: 0.14}
)

func suitColor(s klondike.Suit) paint.Color {
	if s.Red() {
		return colRed
	}
	return colBlack
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

// drawCard paints one card in r (face up or the patterned back).
func drawCard(c paint.Canvas, r geom.Rect, card klondike.Card) {
	rad := r.Dx() * 0.09
	if !card.Up {
		c.FillRRectGradient(r, rad, colBack1, colBack2, false)
		c.StrokeRRect(r, rad, 1, colEdge)
		inset := r.Dx() * 0.14
		inner := geom.Rect{Min: r.Min.Add(geom.Pt{X: inset, Y: inset}), Max: r.Max.Sub(geom.Pt{X: inset, Y: inset})}
		c.StrokeRRect(inner, rad*0.6, 1, colBackHi)
		return
	}
	c.FillRRect(r, rad, colFace)
	c.StrokeRRect(r, rad, 1, colEdge)

	col := suitColor(card.Suit)
	sz := r.Dx()
	label := rankLabel(card.Rank)
	// Corner rank (baseline-left) + a small pip beneath it.
	c.Text(label, geom.Pt{X: r.Min.X + sz*0.12, Y: r.Min.Y + sz*0.30}, sz*0.30, col)
	drawPip(c, r.Min.X+sz*0.20, r.Min.Y+sz*0.46, sz*0.16, card.Suit, col)
	// Large center pip.
	drawPip(c, r.Min.X+sz*0.5, r.Min.Y+r.Dy()*0.60, sz*0.42, card.Suit, col)
}

// drawEmpty paints a ghost slot where a pile can be placed.
func drawEmpty(c paint.Canvas, r geom.Rect) {
	c.StrokeRRect(r, r.Dx()*0.09, 1.5, colSlot)
}

func circle(c paint.Canvas, cx, cy, rad float32, col paint.Color) {
	c.FillRRect(geom.RectXYWH(cx-rad, cy-rad, rad*2, rad*2), rad, col)
}

// diamond draws a square of side s rotated 45° about (cx,cy) — a diamond of
// height ~s*√2.
func diamond(c paint.Canvas, cx, cy, s float32, col paint.Color) {
	c.PushTransform(paint.Transform{Rotation: math.Pi / 4, PivotX: cx, PivotY: cy})
	c.FillRect(geom.RectXYWH(cx-s/2, cy-s/2, s, s), col)
	c.PopTransform()
}

// drawPip draws the suit glyph centered at (cx,cy) roughly sz tall, built from
// circles, rotated squares and stems (no font dependency on ♠♥♦♣).
func drawPip(c paint.Canvas, cx, cy, sz float32, s klondike.Suit, col paint.Color) {
	switch s {
	case klondike.Diamond:
		diamond(c, cx, cy, sz*0.72, col)
	case klondike.Heart:
		r := sz * 0.27
		circle(c, cx-r*0.85, cy-r*0.55, r, col)
		circle(c, cx+r*0.85, cy-r*0.55, r, col)
		diamond(c, cx, cy+r*0.35, sz*0.66, col)
	case klondike.Club:
		r := sz * 0.24
		circle(c, cx, cy-r*1.05, r, col)
		circle(c, cx-r*0.95, cy+r*0.45, r, col)
		circle(c, cx+r*0.95, cy+r*0.45, r, col)
		c.FillRect(geom.RectXYWH(cx-sz*0.06, cy+r*0.2, sz*0.12, sz*0.42), col)
	default: // Spade
		r := sz * 0.24
		diamond(c, cx, cy-sz*0.18, sz*0.62, col) // pointed top
		circle(c, cx-r*0.9, cy+r*0.5, r, col)
		circle(c, cx+r*0.9, cy+r*0.5, r, col)
		c.FillRect(geom.RectXYWH(cx-sz*0.06, cy+r*0.2, sz*0.12, sz*0.42), col)
	}
}
