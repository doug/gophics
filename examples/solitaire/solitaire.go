package main

import (
	"encoding/json"
	"math/rand"
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/examples/solitaire/klondike"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
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

	// Deal: a fresh game flies its tableau cards in from the stock, staggered.
	dealing  bool
	dealCtrl *anim.Controller

	// Win cascade: on a win, the foundation cards fountain off and bounce down
	// the felt, leaving streaks — the classic finale.
	size        geom.Size // last drawn surface size (physics bounds)
	cascading   bool
	cascadeTick *cascadeAnim
	cascade     []fallCard   // cards currently in flight
	stamps      []stamp      // trail left behind (bounded)
	launch      []launchItem // cards waiting to fountain, top-first
	launchT     float32      // countdown to the next launch
	rng         *rand.Rand
}

// fallCard is a bouncing card during the win cascade.
type fallCard struct {
	card     klondike.Card
	pos, vel geom.Pt
}

// stamp is one frame of a card's trail, drawn cheaply so streaks are affordable.
type stamp struct {
	card klondike.Card
	pos  geom.Pt
}

// launchItem is a foundation card queued to fountain, with its source pile.
type launchItem struct {
	card  klondike.Card
	found int
}

func (s *gameState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.deal = s.W().Seed
	s.store = makeStore()
	g, resumed := s.loadOrNew()
	s.g = g
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
	s.dealCtrl = &anim.Controller{Duration: 650 * time.Millisecond, Curve: anim.Linear, OnChange: func() {
		s.SetState(nil)
		if s.dealCtrl.Value() >= 1 {
			s.dealing = false
		}
	}}
	ctx.AddTicker(s.dealCtrl)
	s.rng = rand.New(rand.NewSource(s.deal + 1))
	s.cascadeTick = &cascadeAnim{s}
	ctx.AddTicker(s.cascadeTick)
	if !resumed {
		s.startDeal() // animate a fresh deal, but not a resumed game
	}
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *gameState) Dispose() {
	s.ctx.RemoveTicker(s.snapCtrl)
	s.ctx.RemoveTicker(s.dealCtrl)
	s.ctx.RemoveTicker(s.cascadeTick)
}

func (s *gameState) startDeal() {
	s.dealing = true
	s.dealCtrl.Jump(0)
	s.dealCtrl.Forward()
	s.ctx.Invalidate()
}

// maybeWin refreshes the win flag and kicks the cascade off exactly on the
// losing→won transition (and cancels it if an undo takes the win back).
func (s *gameState) maybeWin() {
	won := s.g.Won()
	switch {
	case won && !s.won:
		s.startCascade()
	case !won:
		s.stopCascade()
	}
	s.won = won
}

// startCascade queues every foundation card (top of each pile first, dealt
// round-robin across suits) to fountain off and bounce down the felt.
func (s *gameState) startCascade() {
	s.cascading = true
	s.cascade, s.stamps, s.launch, s.launchT = nil, nil, nil, 0
	maxLen := 0
	for i := range 4 {
		if l := len(s.g.Foundation(i)); l > maxLen {
			maxLen = l
		}
	}
	for row := 0; row < maxLen; row++ {
		for i := range 4 {
			f := s.g.Foundation(i)
			if idx := len(f) - 1 - row; idx >= 0 {
				s.launch = append(s.launch, launchItem{f[idx], i})
			}
		}
	}
	s.ctx.Invalidate()
}

func (s *gameState) stopCascade() {
	s.cascading = false
	s.cascade, s.stamps, s.launch = nil, nil, nil
}

// stepCascade advances the cascade physics by dt seconds: launch the next card
// on a fixed cadence, integrate gravity + floor bounce, and record a trail.
func (s *gameState) stepCascade(dt float32) {
	if s.size.W == 0 {
		return // no frame drawn yet — no bounds to bounce within
	}
	if dt > 0.05 {
		dt = 0.05 // clamp long stalls so the integration stays stable
	}
	for s.launchT -= dt; s.launchT <= 0 && len(s.launch) > 0; s.launchT += 0.11 {
		it := s.launch[0]
		s.launch = s.launch[1:]
		vx := (s.rng.Float32()*2 - 1) // [-1,1]
		if vx > -0.4 && vx < 0.4 {    // ensure a decent sideways throw
			if vx < 0 {
				vx -= 0.4
			} else {
				vx += 0.4
			}
		}
		s.cascade = append(s.cascade, fallCard{
			card: it.card,
			pos:  s.board.Foundations[it.found].Min,
			vel:  geom.Pt{X: vx * 340, Y: -(220 + s.rng.Float32()*180)},
		})
	}
	const gravity = 2100
	floor := s.size.H - s.board.CardH
	alive := s.cascade[:0]
	for _, fc := range s.cascade {
		fc.vel.Y += gravity * dt
		fc.pos.X += fc.vel.X * dt
		fc.pos.Y += fc.vel.Y * dt
		if fc.pos.Y >= floor {
			fc.pos.Y = floor
			if fc.vel.Y = -fc.vel.Y * 0.78; fc.vel.Y > -90 {
				fc.vel.Y = 0 // too slow to rebound — slide off along the floor
			}
		}
		s.stamps = append(s.stamps, stamp{fc.card, fc.pos})
		if fc.pos.X > -s.board.CardW && fc.pos.X < s.size.W {
			alive = append(alive, fc)
		}
	}
	s.cascade = alive
	if n := len(s.stamps); n > 900 { // bound the trail (perf)
		s.stamps = append(s.stamps[:0], s.stamps[n-900:]...)
	}
	if len(s.cascade) == 0 && len(s.launch) == 0 {
		s.cascading = false
	}
}

// cascadeAnim drives the win cascade's per-frame physics.
type cascadeAnim struct{ s *gameState }

func (a *cascadeAnim) Tick(dt float64) bool {
	if !a.s.cascading {
		return false
	}
	a.s.stepCascade(float32(dt))
	a.s.SetState(nil)
	a.s.ctx.Invalidate()
	return a.s.cascading
}

// loadOrNew resumes the saved game (resumed=true), or deals a fresh one if
// there's no valid save.
func (s *gameState) loadOrNew() (g *klondike.Game, resumed bool) {
	if s.store != nil {
		if data, ok := s.store.load(); ok {
			var snap klondike.Snapshot
			if json.Unmarshal(data, &snap) == nil {
				if g := klondike.Restore(snap); fullDeck(g) {
					return g, true
				}
			}
		}
	}
	return klondike.New(s.deal, 1), false
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
		Gestures: widget.Gestures{
			OnPress: func(p geom.Pt) {
				if s.dealing { // ignore board input while the deal animates in
					s.pressOK = false
					return
				}
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
					s.maybeWin()
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
				s.maybeWin()
				s.persist()
				s.SetState(nil)
			},
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}

	// The board fills the window (so board coordinates are window coordinates);
	// the controls float over the felt at the bottom-right, on top of it.
	var items []widget.Widget
	if s.g.CanAutoComplete() {
		items = append(items, chip("Finish", s.finish), widget.Sized{W: 8})
	}
	items = append(items, chip("Undo", s.undo), widget.Sized{W: 8}, chip("New", s.newGame))
	controls := widget.Row(items...)
	controls.CrossAlign = layout.CrossCenter
	return widget.Stack{Children: []widget.Widget{
		board,
		widget.Align{X: 1, Y: 1, Child: widget.Padding{All: 14, Child: controls}},
	}}
}

func chip(label string, onTap func()) widget.Widget {
	return widget.Interactive{
		Gestures: widget.Gestures{OnTap: onTap},
		Child: widget.Decorated{Color: colBack2, Radius: 8, Child: widget.Padding{
			Insets: geom.InsetsSymmetric(14, 7),
			Child:  widget.Text{S: label, Size: 14, Color: colFace},
		}},
	}
}

func (s *gameState) undo() {
	s.cancelInteraction()
	s.g.Undo()
	s.maybeWin()
	s.persist()
	s.SetState(nil)
}

func (s *gameState) newGame() {
	s.cancelInteraction()
	s.deal++
	s.g = klondike.New(s.deal, 1)
	s.stopCascade()
	s.won = false
	s.persist()
	s.startDeal()
	s.SetState(nil)
}

// finish auto-plays the rest of the game to the foundations (shown only when
// s.g.CanAutoComplete()).
func (s *gameState) finish() {
	s.cancelInteraction()
	s.g.AutoComplete()
	s.maybeWin()
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
	s.size = size
	b := s.board
	// A subtle felt gradient (lighter top → darker bottom) for depth.
	c.FillRRectGradient(geom.RectXYWH(0, 0, size.W, size.H), 0, colFeltHi, colFeltLo, false)

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
	for i := range 4 {
		f := s.g.Foundation(i)
		hiding := s.hidingRun() && s.dragPile.Kind == klondike.Foundation && s.dragPile.Index == i
		if len(f) > 0 && !hiding {
			drawCard(c, b.Foundations[i], f[len(f)-1])
		} else {
			drawEmpty(c, b.Foundations[i])
		}
	}
	total := 0
	for j := range 7 {
		total += len(s.g.Tableau(j))
	}
	di := 0
	for j := range 7 {
		col := s.g.Tableau(j)
		if len(col) == 0 {
			drawEmpty(c, b.Slot[j])
		}
		for k := range col {
			idx := di
			di++
			if s.hidingRun() && s.dragPile.Kind == klondike.Tableau && s.dragPile.Index == j && k >= s.dragIdx {
				break // being dragged / snapped
			}
			if s.dealing {
				if lt, flying := dealProgress(s.dealCtrl.Value(), idx, total); lt < 0 {
					continue // still in the deck (drawn as the stock back)
				} else if flying {
					x := b.Stock.Min.X + (b.Tableaus[j][k].Min.X-b.Stock.Min.X)*lt
					y := b.Stock.Min.Y + (b.Tableaus[j][k].Min.Y-b.Stock.Min.Y)*lt
					drawCard(c, geom.RectXYWH(x, y, b.CardW, b.CardH), col[k])
					continue
				}
			}
			// A card with another on top of it shows only a strip, so it is
			// drawn without the back's inset frame -- see drawCardFanned.
			if k < len(col)-1 {
				drawCardFanned(c, b.Tableaus[j][k], col[k])
			} else {
				drawCard(c, b.Tableaus[j][k], col[k])
			}
		}
	}

	// The active run (dragged, or gliding home), on top.
	if rx, ry, ok := s.runOrigin(); ok {
		fan := b.CardH * 0.30
		for i, card := range s.dragCards {
			drawCard(c, geom.RectXYWH(rx, ry+float32(i)*fan, b.CardW, b.CardH), card)
		}
	}
	// Win cascade: the trail streaks under the live bouncing cards.
	for _, st := range s.stamps {
		drawStamp(c, geom.RectXYWH(st.pos.X, st.pos.Y, b.CardW, b.CardH), st.card)
	}
	for _, fc := range s.cascade {
		drawCard(c, geom.RectXYWH(fc.pos.X, fc.pos.Y, b.CardW, b.CardH), fc.card)
	}

	// The banner lands once the cascade has played out.
	if s.won && !s.cascading {
		c.FillRRect(geom.RectXYWH(size.W*0.5-size.W*0.22, size.H*0.42, size.W*0.44, size.H*0.14), 16, colFeltHi)
		c.TextIn("bold", "You win!", geom.Pt{X: size.W*0.5 - size.W*0.16, Y: size.H * 0.52}, size.W*0.08, colFace)
	}
}

// dealProgress maps the global deal timeline t (0..1) to card idx's flight:
// lt < 0 means still in the deck, 0..1 means in flight, and flying is true only
// during that window (lt >= 1 means arrived — draw it at its final spot).
func dealProgress(t float32, idx, total int) (lt float32, flying bool) {
	if total < 1 {
		total = 1
	}
	const fly = 0.5
	lt = (t - float32(idx)*(0.5/float32(total))) / fly
	switch {
	case lt < 0:
		return lt, false
	case lt >= 1:
		return 1, false
	default:
		return lt, true
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
