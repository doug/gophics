package main

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/solitaire/klondike"
	"github.com/doug/gophics/geom"
)

var testSize = geom.Size{W: 800, H: 600}

// memStore is an in-memory save slot, so tests never touch the real config dir.
type memStore struct{ data []byte }

func (m *memStore) save(d []byte)        { m.data = append([]byte(nil), d...) }
func (m *memStore) load() ([]byte, bool) { return m.data, m.data != nil }

func mount(t *testing.T, seed int64) (*app.Headless, *gameState) {
	t.Helper()
	makeStore = func() store { return &memStore{} } // fresh slot → deterministic new deal
	var st *gameState
	stateHook = func(s *gameState) { st = s }
	defer func() { stateHook = nil }()
	h, err := app.NewHeadless(Solitaire{Seed: seed}, app.Config{
		Size:         testSize,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // computes the board geometry the handlers hit-test against
	if st == nil {
		t.Fatal("game state not mounted")
	}
	// Let the deal animation finish so tests interact with a settled board.
	for i := 0; i < 40 && st.dealing; i++ {
		h.Step(0.05)
		h.Render()
	}
	return h, st
}

func center(r geom.Rect) geom.Pt {
	return geom.Pt{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
}

func TestBoardHitAndLayout(t *testing.T) {
	_, st := mount(t, 1)
	b := Layout(testSize, st.g)

	if p, _, ok := b.Hit(center(b.Stock)); !ok || p.Kind != klondike.Stock {
		t.Fatalf("stock hit = %+v ok=%v", p, ok)
	}
	if p, _, ok := b.Hit(center(b.Foundations[2])); !ok || p.Kind != klondike.Foundation || p.Index != 2 {
		t.Fatalf("foundation hit = %+v ok=%v", p, ok)
	}
	// The topmost card of tableau column 6 (7 cards) is the last, face-up one.
	col := 6
	rects := b.Tableaus[col]
	p, idx, ok := b.Hit(center(rects[len(rects)-1]))
	if !ok || p.Kind != klondike.Tableau || p.Index != col || idx != len(rects)-1 {
		t.Fatalf("tableau top hit = %+v idx=%d ok=%v", p, idx, ok)
	}
}

func TestTapDrawsFromStock(t *testing.T) {
	h, st := mount(t, 1)
	b := Layout(testSize, st.g)
	before := len(st.g.Waste())
	h.Tap(center(b.Stock))
	h.Render()
	if len(st.g.Waste()) != before+1 {
		t.Fatalf("waste = %d after tapping stock, want %d", len(st.g.Waste()), before+1)
	}
}

func TestTapStockRecycles(t *testing.T) {
	h, st := mount(t, 1)
	b := Layout(testSize, st.g)
	stock0 := len(st.g.Stock())
	for range stock0 {
		h.Tap(center(b.Stock))
	}
	h.Render()
	if len(st.g.Stock()) != 0 || len(st.g.Waste()) != stock0 {
		t.Fatalf("after draining: stock=%d waste=%d", len(st.g.Stock()), len(st.g.Waste()))
	}
	h.Tap(center(b.Stock)) // recycle
	h.Render()
	if len(st.g.Stock()) != stock0 || len(st.g.Waste()) != 0 {
		t.Fatalf("after recycle: stock=%d waste=%d", len(st.g.Stock()), len(st.g.Waste()))
	}
}

// TestDragPerformsLegalMove asks the engine for a legal move from a tableau,
// then drives that move through the UI (drag from source card to target) and
// asserts it happened. Draws through the stock first to widen the options.
func TestDragPerformsLegalMove(t *testing.T) {
	h, st := mount(t, 1)

	var act klondike.Action
	found := false
	for round := 0; round < 30 && !found; round++ {
		for _, a := range st.g.LegalActions() {
			if a.From.Kind == klondike.Tableau {
				act, found = a, true
				break
			}
		}
		if !found {
			st.g.Draw()
		}
	}
	if !found {
		t.Skip("no tableau move surfaced in this deal")
	}
	h.Render() // refresh board after any draws
	b := Layout(testSize, st.g)

	from := center(b.Tableaus[act.From.Index][act.FromIdx])
	var to geom.Pt
	for _, dt := range b.DropTargets(st.g) {
		if dt.Pile == act.To {
			to = center(dt.Rect)
		}
	}
	before := st.g.MoveCount()
	h.DragTo(from, to)
	h.Release(to)
	h.Render()
	if st.g.MoveCount() <= before {
		t.Fatalf("drag did not perform the move %+v", act)
	}
}

func TestUndoAndNewGame(t *testing.T) {
	h, st := mount(t, 1)
	b := Layout(testSize, st.g)

	h.Tap(center(b.Stock)) // draw one to the waste
	h.Render()
	if len(st.g.Waste()) != 1 {
		t.Fatalf("expected a card on the waste, got %d", len(st.g.Waste()))
	}
	st.undo()
	if len(st.g.Waste()) != 0 || st.g.MoveCount() != 0 {
		t.Fatalf("undo failed: waste=%d moves=%d", len(st.g.Waste()), st.g.MoveCount())
	}

	h.Tap(center(b.Stock))
	h.Tap(center(b.Stock))
	h.Render()
	st.newGame()
	if st.g.MoveCount() != 0 || len(st.g.Stock()) != 24 || len(st.g.Waste()) != 0 {
		t.Fatalf("new game not fresh: moves=%d stock=%d waste=%d",
			st.g.MoveCount(), len(st.g.Stock()), len(st.g.Waste()))
	}
}

func TestSnapBackSettles(t *testing.T) {
	h, st := mount(t, 1)
	b := Layout(testSize, st.g)

	// Drag the face-up top of column 6 onto the stock — never a legal target.
	col := b.Tableaus[6]
	from := center(col[len(col)-1])
	to := center(b.Stock)
	before := st.g.MoveCount()
	h.DragTo(from, to)
	h.Release(to)
	h.Render()

	if st.g.MoveCount() != before {
		t.Fatalf("illegal drop changed the game (moves %d→%d)", before, st.g.MoveCount())
	}
	if !st.snapping {
		t.Fatal("an illegal drop should start a snap-back animation")
	}
	for i := 0; i < 40 && (st.snapping || st.dragging); i++ {
		h.Step(0.016)
		h.Render()
	}
	if st.snapping || st.dragging {
		t.Fatal("snap-back did not settle")
	}
}

func TestPersistResume(t *testing.T) {
	shared := &memStore{}
	makeStore = func() store { return shared }
	t.Cleanup(func() { makeStore = platformStore })

	cfg := app.Config{Size: testSize, Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF}}

	// Session 1: draw a card (autosaves to the shared store).
	var st1 *gameState
	stateHook = func(s *gameState) { st1 = s }
	h1, err := app.NewHeadless(Solitaire{Seed: 1}, cfg, 1)
	stateHook = nil
	if err != nil {
		t.Fatal(err)
	}
	h1.Render()
	for i := 0; i < 60 && st1.dealing; i++ { // let the deal finish before interacting
		h1.Step(0.05)
		h1.Render()
	}
	h1.Tap(center(Layout(testSize, st1.g).Stock))
	h1.Render()
	if len(st1.g.Waste()) != 1 {
		t.Fatalf("session 1: expected a drawn card, got waste=%d", len(st1.g.Waste()))
	}

	// Session 2 with the same store resumes the drawn state, not a fresh deal.
	var st2 *gameState
	stateHook = func(s *gameState) { st2 = s }
	_, err = app.NewHeadless(Solitaire{Seed: 1}, cfg, 1)
	stateHook = nil
	if err != nil {
		t.Fatal(err)
	}
	if len(st2.g.Waste()) != 1 || st2.g.MoveCount() != 1 {
		t.Fatalf("session 2 did not resume: waste=%d moves=%d", len(st2.g.Waste()), st2.g.MoveCount())
	}
}

func TestWinCascade(t *testing.T) {
	h, st := mount(t, 1)

	// Force a completed game: all four foundations full, A→K.
	var snap klondike.Snapshot
	snap.DrawN = 1
	for suit := range 4 {
		for rank := 1; rank <= 13; rank++ {
			snap.Foundations[suit] = append(snap.Foundations[suit],
				klondike.Card{Suit: klondike.Suit(suit), Rank: uint8(rank), Up: true})
		}
	}
	st.g = klondike.Restore(snap)
	h.Render() // record size/board so the cascade has bounds
	st.maybeWin()
	if !st.cascading || !st.won {
		t.Fatalf("a win should start the cascade: cascading=%v won=%v", st.cascading, st.won)
	}

	settled := false
	for i := 0; i < 3000 && !settled; i++ {
		h.Step(0.016)
		settled = !st.cascading
	}
	if !settled {
		t.Fatal("cascade did not settle")
	}
	if !st.won {
		t.Fatal("game should still read as won after the cascade")
	}
	// A New game clears any lingering trail.
	st.newGame()
	if st.cascading || len(st.stamps) != 0 {
		t.Fatalf("new game left cascade state: cascading=%v stamps=%d", st.cascading, len(st.stamps))
	}
}

func TestDealAnimates(t *testing.T) {
	makeStore = func() store { return &memStore{} }
	var st *gameState
	stateHook = func(s *gameState) { st = s }
	cfg := app.Config{Size: testSize, Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF}}
	h, err := app.NewHeadless(Solitaire{Seed: 1}, cfg, 1)
	stateHook = nil
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if !st.dealing {
		t.Fatal("a fresh game should animate the deal")
	}
	for i := 0; i < 60 && st.dealing; i++ {
		h.Step(0.05)
		h.Render()
	}
	if st.dealing {
		t.Fatal("deal animation did not settle")
	}
	if len(st.g.Stock()) != 24 {
		t.Fatalf("after the deal, stock = %d, want 24", len(st.g.Stock()))
	}
}
