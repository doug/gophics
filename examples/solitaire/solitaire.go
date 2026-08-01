package main

import (
	"encoding/json"
	"time"

	"github.com/doug/gossamer/anim"
	"github.com/doug/gossamer/examples/solitaire/klondike"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
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
	store store
	board Board
	deal  int64 // current deal's seed; bumped by New game
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

	// Snap-back: an illegal drop glides the run home instead of vanishing.
	snapping         bool
	snapCtrl         *anim.Controller
	snapFrom, snapTo geom.Pt
}

func (s *gameState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.deal = s.W().Seed
	s.store = makeStore()
	s.g = s.loadOrNew()
	s.won = s.g.Won()
	s.snapCtrl = &anim.Controller{Duration: 170 * time.Millisecond, Curve: anim.EaseOut, OnChange: func() {
		s.SetState(nil)
		// Finalize on completion (Value hits 1) — not on the initial Jump(0),
		// which also leaves the controller not-Running but at Value 0.
		if s.snapping && s.snapCtrl.Value() >= 1 {
			s.snapping, s.dragCards = false, nil
		}
	}}
	ctx.AddTicker(s.snapCtrl)
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *gameState) Dispose() { s.ctx.RemoveTicker(s.snapCtrl) }

// loadOrNew resumes the saved game, or deals a fresh one if there's no valid save.
func (s *gameState) loadOrNew() *klondike.Game {
	if s.store != nil {
		if data, ok := s.store.load(); ok {
			var snap klondike.Snapshot
			if json.Unmarshal(data, &snap) == nil {
				if g := klondike.Restore(snap); fullDeck(g) {
					return g
				}
			}
		}
	}
	return klondike.New(s.deal, 1)
}

// persist autosaves the current game (called after every state change).
func (s *gameState) persist() {
	if s.store == nil {
		return
	}
	if data, err := json.Marshal(s.g.Save()); err == nil {
		s.store.save(data)
	}
}

func (s *gameState) Build(ctx widget.Ctx) widget.Widget {
	board := widget.Interactive{
		Handler: widget.Handler{
			OnPress: func(p geom.Pt) {
				s.pressHit, s.pressIdx, s.pressOK = s.board.Hit(p)
				s.pressStart, s.dragging = p, false
			},
			OnDrag: func(pos, _ geom.Pt) {
				if !s.pressOK {
					return
				}
				if !s.dragging {
					if s.snapping || !s.grab(s.pressHit, s.pressIdx, s.pressStart) {
						s.pressOK = false
						return
					}
					s.dragging = true
				}
				s.pointer = pos
				s.SetState(nil)
			},
			OnRelease: func() {
				if !s.dragging {
					return
				}
				if s.tryDrop() {
					s.dragging, s.dragCards = false, nil
					s.won = s.g.Won()
					s.persist()
				} else {
					s.startSnapBack()
				}
				s.SetState(nil)
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
				s.persist()
				s.SetState(nil)
			},
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}

	// The board fills the window (so board coordinates are window coordinates);
	// the controls float over the felt at the bottom-right, on top of it.
	controls := widget.Row(chip("Undo", s.undo), widget.Sized{W: 8}, chip("New", s.newGame))
	controls.CrossAlign = layout.CrossCenter
	return widget.Stack{Children: []widget.Widget{
		board,
		widget.Align{X: 1, Y: 1, Child: widget.Padding{All: 14, Child: controls}},
	}}
}

func chip(label string, onTap func()) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{OnTap: onTap},
		Child: widget.Decorated{Color: colBack2, Radius: 8, Child: widget.Padding{
			Insets: geom.InsetsSymmetric(14, 7),
			Child:  widget.Text{S: label, Size: 14, Color: colFace},
		}},
	}
}

func (s *gameState) undo() {
	s.cancelInteraction()
	s.g.Undo()
	s.won = s.g.Won()
	s.persist()
	s.SetState(nil)
}

func (s *gameState) newGame() {
	s.cancelInteraction()
	s.deal++
	s.g = klondike.New(s.deal, 1)
	s.won = false
	s.persist()
	s.SetState(nil)
}

func (s *gameState) cancelInteraction() {
	s.dragging, s.snapping, s.dragCards, s.pressOK = false, false, nil, false
	s.snapCtrl.Jump(0)
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

// tryDrop lands the dragged run on the legal target it overlaps most and reports
// whether it moved (false → the caller snaps it back).
func (s *gameState) tryDrop() bool {
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
		return s.g.Move(s.dragPile, s.dragIdx, targets[best].Pile)
	}
	return false
}

// startSnapBack animates the dragged run from the release point back to where it
// was grabbed, then clears it (the game was never mutated).
func (s *gameState) startSnapBack() {
	s.snapFrom = geom.Pt{X: s.pointer.X - s.grabOff.X, Y: s.pointer.Y - s.grabOff.Y}
	s.snapTo = s.sourceTop()
	s.dragging, s.snapping = false, true
	s.snapCtrl.Jump(0)
	s.snapCtrl.Forward()
	s.ctx.Invalidate()
}

func (s *gameState) sourceTop() geom.Pt {
	switch s.dragPile.Kind {
	case klondike.Waste:
		return s.board.Waste.Min
	case klondike.Foundation:
		return s.board.Foundations[s.dragPile.Index].Min
	case klondike.Tableau:
		return s.board.Tableaus[s.dragPile.Index][s.dragIdx].Min
	}
	return geom.Pt{}
}

// hidingRun reports whether the source cards of the active run should be hidden
// (they are being dragged or snapped back and drawn as an overlay).
func (s *gameState) hidingRun() bool { return s.dragging || s.snapping }

func (s *gameState) draw(c paint.Canvas, size geom.Size) {
	s.board = Layout(size, s.g)
	b := s.board
	c.Clear(colFelt)

	if len(s.g.Stock()) > 0 {
		drawCard(c, b.Stock, klondike.Card{})
	} else {
		drawEmpty(c, b.Stock)
	}
	w := s.g.Waste()
	if len(w) > 0 && !(s.hidingRun() && s.dragPile.Kind == klondike.Waste) {
		drawCard(c, b.Waste, w[len(w)-1])
	} else {
		drawEmpty(c, b.Waste)
	}
	for i := 0; i < 4; i++ {
		f := s.g.Foundation(i)
		hiding := s.hidingRun() && s.dragPile.Kind == klondike.Foundation && s.dragPile.Index == i
		if len(f) > 0 && !hiding {
			drawCard(c, b.Foundations[i], f[len(f)-1])
		} else {
			drawEmpty(c, b.Foundations[i])
		}
	}
	for j := 0; j < 7; j++ {
		col := s.g.Tableau(j)
		if len(col) == 0 {
			drawEmpty(c, b.Slot[j])
		}
		for k := range col {
			if s.hidingRun() && s.dragPile.Kind == klondike.Tableau && s.dragPile.Index == j && k >= s.dragIdx {
				break // being dragged / snapped
			}
			drawCard(c, b.Tableaus[j][k], col[k])
		}
	}

	// The active run (dragged, or gliding home), on top.
	if rx, ry, ok := s.runOrigin(); ok {
		fan := b.CardH * 0.30
		for i, card := range s.dragCards {
			drawCard(c, geom.RectXYWH(rx, ry+float32(i)*fan, b.CardW, b.CardH), card)
		}
	}
	if s.won {
		c.FillRRect(geom.RectXYWH(size.W*0.5-size.W*0.22, size.H*0.42, size.W*0.44, size.H*0.14), 16, colFeltHi)
		c.TextIn("bold", "You win!", geom.Pt{X: size.W*0.5 - size.W*0.16, Y: size.H * 0.52}, size.W*0.08, colFace)
	}
}

// runOrigin returns the top-left of the active run and whether one is showing.
func (s *gameState) runOrigin() (float32, float32, bool) {
	switch {
	case s.dragging:
		return s.pointer.X - s.grabOff.X, s.pointer.Y - s.grabOff.Y, true
	case s.snapping:
		t := s.snapCtrl.Value()
		return s.snapFrom.X + (s.snapTo.X-s.snapFrom.X)*t, s.snapFrom.Y + (s.snapTo.Y-s.snapFrom.Y)*t, true
	}
	return 0, 0, false
}
