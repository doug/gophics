package main

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/examples/solitaire/klondike"
	"github.com/doug/gossamer/geom"
)

var testSize = geom.Size{W: 800, H: 600}

func mount(t *testing.T, seed int64) (*app.Headless, *gameState) {
	t.Helper()
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
	for i := 0; i < stock0; i++ {
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
