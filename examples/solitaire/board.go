package main

import (
	"slices"

	"github.com/doug/gophics/examples/solitaire/klondike"
	"github.com/doug/gophics/geom"
)

// Board is the pure geometry of a Klondike layout for a given surface size and
// game — every card's rectangle, shared by rendering and hit-testing so they
// can never disagree. It holds no game state; recompute it each frame (cheap).
type Board struct {
	CardW, CardH float32
	Stock, Waste geom.Rect
	Foundations  [4]geom.Rect
	Slot         [7]geom.Rect   // base rect of each tableau column (empty slot / first card)
	Tableaus     [7][]geom.Rect // one rect per card, fanned down
}

// Layout computes the board for size and the current game.
func Layout(size geom.Size, g *klondike.Game) Board {
	const margin, gapFrac, fanUpFrac, fanDownFrac = 14, 0.16, 0.30, 0.11
	usable := size.W - 2*margin
	cardW := usable / (7 + 6*gapFrac)
	gap := cardW * gapFrac
	cardH := cardW * 1.4
	colX := func(i int) float32 { return margin + float32(i)*(cardW+gap) }
	rect := func(x, y float32) geom.Rect { return geom.RectXYWH(x, y, cardW, cardH) }

	var b Board
	b.CardW, b.CardH = cardW, cardH
	topY := float32(margin)
	b.Stock = rect(colX(0), topY)
	b.Waste = rect(colX(1), topY)
	for i := range 4 {
		b.Foundations[i] = rect(colX(3+i), topY)
	}

	tableTop := topY + cardH + gap*1.4
	fanUp, fanDown := cardH*fanUpFrac, cardH*fanDownFrac
	for j := range 7 {
		x := colX(j)
		b.Slot[j] = rect(x, tableTop)
		col := g.Tableau(j)
		y := tableTop
		for k := range col {
			b.Tableaus[j] = append(b.Tableaus[j], rect(x, y))
			if col[k].Up {
				y += fanUp
			} else {
				y += fanDown
			}
		}
	}
	return b
}

// Hit returns the pile and — for a tableau — the index of the topmost card at p.
// For waste/foundation/stock the returned index is not meaningful (use the top).
// idx == -1 means an empty tableau slot.
func (b Board) Hit(p geom.Pt) (pile klondike.Pile, idx int, ok bool) {
	switch {
	case b.Stock.Contains(p):
		return klondike.Pile{Kind: klondike.Stock}, 0, true
	case b.Waste.Contains(p):
		return klondike.Pile{Kind: klondike.Waste}, 0, true
	}
	for i := range 4 {
		if b.Foundations[i].Contains(p) {
			return klondike.Pile{Kind: klondike.Foundation, Index: i}, 0, true
		}
	}
	for j := range 7 {
		rects := b.Tableaus[j]
		for k, rect := range slices.Backward(rects) {
			if rect.Contains(p) {
				return klondike.Pile{Kind: klondike.Tableau, Index: j}, k, true
			}
		}
		if len(rects) == 0 && b.Slot[j].Contains(p) {
			return klondike.Pile{Kind: klondike.Tableau, Index: j}, -1, true
		}
	}
	return klondike.Pile{}, 0, false
}

// DropTarget is a candidate landing spot for a dragged run.
type DropTarget struct {
	Pile klondike.Pile
	Rect geom.Rect
}

// DropTargets returns the foundations and the landing rect of each tableau
// column (its top card, or the empty slot), for overlap-based drop resolution.
func (b Board) DropTargets(g *klondike.Game) []DropTarget {
	out := make([]DropTarget, 0, 11)
	for i := range 4 {
		out = append(out, DropTarget{klondike.Pile{Kind: klondike.Foundation, Index: i}, b.Foundations[i]})
	}
	for j := range 7 {
		r := b.Slot[j]
		if n := len(b.Tableaus[j]); n > 0 {
			r = b.Tableaus[j][n-1]
		}
		out = append(out, DropTarget{klondike.Pile{Kind: klondike.Tableau, Index: j}, r})
	}
	return out
}

// overlapArea is the area of the intersection of a and b (0 if disjoint).
func overlapArea(a, c geom.Rect) float32 {
	x := min(a.Max.X, c.Max.X) - max(a.Min.X, c.Min.X)
	y := min(a.Max.Y, c.Max.Y) - max(a.Min.Y, c.Min.Y)
	if x <= 0 || y <= 0 {
		return 0
	}
	return x * y
}
