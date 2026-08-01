package main

import (
	"github.com/doug/gossamer/examples/solitaire/klondike"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// Solitaire is the root widget: a full-screen Klondike board. Seed makes the
// deal deterministic (tests pass a fixed value; the command uses the clock).
type Solitaire struct{ Seed int64 }

func (Solitaire) CreateState() widget.State { return &gameState{} }

// stateHook lets tests observe the mounted game state.
var stateHook func(*gameState)

type gameState struct {
	widget.StateBase[Solitaire]
	ctx   widget.Ctx
	g     *klondike.Game
	board Board
	won   bool

	// Press/drag transient state.
	pressHit   klondike.Pile
	pressIdx   int
	pressOK    bool
	pressStart geom.Pt

	dragging  bool
	dragPile  klondike.Pile
	dragIdx   int
	dragCards []klondike.Card
	grabOff   geom.Pt // pointer offset within the grabbed top card
	pointer   geom.Pt // live pointer during a drag
}

func (s *gameState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.g = klondike.New(s.W().Seed, 1)
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *gameState) Build(ctx widget.Ctx) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{
			OnPress: func(p geom.Pt) {
				s.pressHit, s.pressIdx, s.pressOK = s.board.Hit(p)
				s.pressStart = p
				s.dragging = false
			},
			OnDrag: func(pos, _ geom.Pt) {
				if !s.pressOK {
					return
				}
				if !s.dragging {
					if !s.grab(s.pressHit, s.pressIdx, s.pressStart) {
						s.pressOK = false
						return
					}
					s.dragging = true
				}
				s.pointer = pos
				s.SetState(nil)
			},
			OnRelease: func() {
				if s.dragging {
					s.drop()
					s.dragging = false
					s.dragCards = nil
					s.won = s.g.Won()
					s.SetState(nil)
				}
			},
			OnTap: func() {
				if !s.pressOK || s.dragging {
					return
				}
				switch s.pressHit.Kind {
				case klondike.Stock:
					s.g.Draw()
				case klondike.Waste, klondike.Tableau, klondike.Foundation:
					s.g.AutoToFoundation(s.pressHit)
				}
				s.won = s.g.Won()
				s.SetState(nil)
			},
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
}

// grab sets up the run being dragged from pile at idx, or returns false.
func (s *gameState) grab(pile klondike.Pile, idx int, p geom.Pt) bool {
	switch pile.Kind {
	case klondike.Waste:
		w := s.g.Waste()
		if len(w) == 0 {
			return false
		}
		s.dragPile, s.dragIdx = pile, len(w)-1
		s.dragCards = []klondike.Card{w[len(w)-1]}
		s.grabOff = p.Sub(s.board.Waste.Min)
		return true
	case klondike.Foundation:
		f := s.g.Foundation(pile.Index)
		if len(f) == 0 {
			return false
		}
		s.dragPile, s.dragIdx = pile, len(f)-1
		s.dragCards = []klondike.Card{f[len(f)-1]}
		s.grabOff = p.Sub(s.board.Foundations[pile.Index].Min)
		return true
	case klondike.Tableau:
		col := s.g.Tableau(pile.Index)
		if idx < 0 || idx >= len(col) || !col[idx].Up {
			return false
		}
		s.dragPile, s.dragIdx = pile, idx
		s.dragCards = append([]klondike.Card(nil), col[idx:]...)
		s.grabOff = p.Sub(s.board.Tableaus[pile.Index][idx].Min)
		return true
	}
	return false
}

// drop lands the dragged run on the legal target it overlaps most, or leaves the
// game untouched (a snap-back, since the drag never mutated it).
func (s *gameState) drop() {
	topRect := geom.RectXYWH(s.pointer.X-s.grabOff.X, s.pointer.Y-s.grabOff.Y, s.board.CardW, s.board.CardH)
	best := -1
	var bestArea float32
	targets := s.board.DropTargets(s.g)
	for i, t := range targets {
		if !s.g.CanMove(s.dragPile, s.dragIdx, t.Pile) {
			continue
		}
		if a := overlapArea(topRect, t.Rect); a > bestArea {
			bestArea, best = a, i
		}
	}
	if best >= 0 && bestArea > 0 {
		s.g.Move(s.dragPile, s.dragIdx, targets[best].Pile)
	}
}

func (s *gameState) draw(c paint.Canvas, size geom.Size) {
	s.board = Layout(size, s.g)
	b := s.board
	c.Clear(colFelt)

	// Stock (face-down back) / empty.
	if len(s.g.Stock()) > 0 {
		drawCard(c, b.Stock, klondike.Card{})
	} else {
		drawEmpty(c, b.Stock)
	}
	// Waste top (hidden while its top card is being dragged).
	w := s.g.Waste()
	draggingWaste := s.dragging && s.dragPile.Kind == klondike.Waste
	if len(w) > 0 && !draggingWaste {
		drawCard(c, b.Waste, w[len(w)-1])
	} else {
		drawEmpty(c, b.Waste)
	}
	// Foundations.
	for i := 0; i < 4; i++ {
		f := s.g.Foundation(i)
		hiding := s.dragging && s.dragPile.Kind == klondike.Foundation && s.dragPile.Index == i
		if len(f) > 0 && !hiding {
			drawCard(c, b.Foundations[i], f[len(f)-1])
		} else {
			drawEmpty(c, b.Foundations[i])
		}
	}
	// Tableaus.
	for j := 0; j < 7; j++ {
		col := s.g.Tableau(j)
		if len(col) == 0 {
			drawEmpty(c, b.Slot[j])
		}
		for k := range col {
			if s.dragging && s.dragPile.Kind == klondike.Tableau && s.dragPile.Index == j && k >= s.dragIdx {
				break // these cards are being dragged
			}
			drawCard(c, b.Tableaus[j][k], col[k])
		}
	}
	// The dragged run, on top, following the pointer.
	if s.dragging {
		x, y := s.pointer.X-s.grabOff.X, s.pointer.Y-s.grabOff.Y
		fan := b.CardH * 0.30
		for i, card := range s.dragCards {
			drawCard(c, geom.RectXYWH(x, y+float32(i)*fan, b.CardW, b.CardH), card)
		}
	}
	if s.won {
		c.FillRRect(geom.RectXYWH(size.W*0.5-size.W*0.22, size.H*0.42, size.W*0.44, size.H*0.14), 16, colFeltHi)
		c.TextIn("bold", "You win!", geom.Pt{X: size.W*0.5 - size.W*0.16, Y: size.H * 0.52}, size.W*0.08, colFace)
	}
}
