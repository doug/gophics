package klondike

import (
	"math/rand"
	"testing"
)

// census counts every card in play, ignoring face-up state.
func census(g *Game) map[[2]int]int {
	m := map[[2]int]int{}
	add := func(cs []Card) {
		for _, c := range cs {
			m[[2]int{int(c.Suit), int(c.Rank)}]++
		}
	}
	add(g.Stock())
	add(g.Waste())
	for i := 0; i < 4; i++ {
		add(g.Foundation(i))
	}
	for i := 0; i < 7; i++ {
		add(g.Tableau(i))
	}
	return m
}

// assertFullDeck verifies the 52 distinct cards are all present exactly once —
// the core "no card lost or duplicated" invariant.
func assertFullDeck(t *testing.T, g *Game) {
	t.Helper()
	m := census(g)
	if len(m) != 52 {
		t.Fatalf("have %d distinct cards, want 52", len(m))
	}
	for k, n := range m {
		if n != 1 {
			t.Fatalf("card %v appears %d times", k, n)
		}
	}
}

func TestDealLayout(t *testing.T) {
	g := New(42, 1)
	assertFullDeck(t, g)
	for i := 0; i < 7; i++ {
		col := g.Tableau(i)
		if len(col) != i+1 {
			t.Fatalf("tableau %d has %d cards, want %d", i, len(col), i+1)
		}
		for j, c := range col {
			if want := j == i; c.Up != want {
				t.Fatalf("tableau %d card %d Up=%v, want %v", i, j, c.Up, want)
			}
		}
	}
	if len(g.Stock()) != 24 {
		t.Fatalf("stock has %d cards, want 24", len(g.Stock()))
	}
	for _, c := range g.Stock() {
		if c.Up {
			t.Fatalf("stock card should be face down: %v", c)
		}
	}
	if len(g.Waste()) != 0 {
		t.Fatalf("waste should start empty")
	}
}

func TestFoundationRules(t *testing.T) {
	g := New(1, 1)
	up := func(s Suit, r uint8) []Card { return []Card{{s, r, true}} }

	if !g.canPlace(up(Spade, 1), Pile{Foundation, 0}) {
		t.Fatal("empty foundation must accept an Ace")
	}
	if g.canPlace(up(Spade, 2), Pile{Foundation, 0}) {
		t.Fatal("empty foundation must reject a non-Ace")
	}
	g.found[0] = up(Club, 1) // Ace of clubs down
	if !g.canPlace(up(Club, 2), Pile{Foundation, 0}) {
		t.Fatal("foundation must accept next rank, same suit")
	}
	if g.canPlace(up(Spade, 2), Pile{Foundation, 0}) {
		t.Fatal("foundation must reject a different suit")
	}
	if g.canPlace(up(Club, 3), Pile{Foundation, 0}) {
		t.Fatal("foundation must reject a rank gap")
	}
	if g.canPlace([]Card{{Club, 2, true}, {Club, 3, true}}, Pile{Foundation, 0}) {
		t.Fatal("foundation must reject a multi-card run")
	}
}

func TestTableauRules(t *testing.T) {
	g := New(1, 1)
	g.tab[0] = nil // clear the dealt column to test the empty-column rule
	if !g.canPlace([]Card{{Spade, 13, true}}, Pile{Tableau, 0}) {
		t.Fatal("empty column must accept a King")
	}
	if g.canPlace([]Card{{Spade, 12, true}}, Pile{Tableau, 0}) {
		t.Fatal("empty column must reject a non-King")
	}
	g.tab[0] = []Card{{Spade, 7, true}} // black 7
	if !g.canPlace([]Card{{Heart, 6, true}}, Pile{Tableau, 0}) {
		t.Fatal("must accept red 6 on black 7")
	}
	if g.canPlace([]Card{{Spade, 6, true}}, Pile{Tableau, 0}) {
		t.Fatal("must reject same-color 6")
	}
	if g.canPlace([]Card{{Heart, 5, true}}, Pile{Tableau, 0}) {
		t.Fatal("must reject a rank gap")
	}
	// A valid run (red6, black5) lands by its bottom card (red 6) on black 7.
	if !g.canPlace([]Card{{Heart, 6, true}, {Spade, 5, true}}, Pile{Tableau, 0}) {
		t.Fatal("must accept a valid run by its bottom card")
	}
}

func TestStockDrawAndRecycle(t *testing.T) {
	g := New(3, 1)
	stock0 := len(g.Stock())
	if !g.Draw() || len(g.Waste()) != 1 || len(g.Stock()) != stock0-1 {
		t.Fatalf("draw did not move one card to the waste")
	}
	// Exhaust the stock.
	for len(g.Stock()) > 0 {
		g.Draw()
	}
	if len(g.Waste()) != stock0 {
		t.Fatalf("waste should hold all %d stock cards, has %d", stock0, len(g.Waste()))
	}
	// Next draw recycles the waste back into the (face-down) stock.
	if !g.Draw() || len(g.Stock()) != stock0 || len(g.Waste()) != 0 {
		t.Fatalf("recycle failed: stock=%d waste=%d", len(g.Stock()), len(g.Waste()))
	}
	for _, c := range g.Stock() {
		if c.Up {
			t.Fatalf("recycled stock must be face down")
		}
	}
	assertFullDeck(t, g)
}

func TestUndoRestoresFlippedCard(t *testing.T) {
	g := New(1, 1)
	// A column whose face-up Ace hides a face-down 7; move the Ace to a
	// foundation, which should flip the 7 up.
	g.tab[0] = []Card{{Diamond, 7, false}, {Spade, 1, true}}
	g.found[0] = nil

	if !g.Move(Pile{Tableau, 0}, 1, Pile{Foundation, 0}) {
		t.Fatal("Ace should move to the empty foundation")
	}
	col := g.Tableau(0)
	if len(col) != 1 || !col[0].Up {
		t.Fatalf("moving the Ace should expose and flip the 7 up, got %v", col)
	}
	last := g.history[len(g.history)-1]
	if !last.Flipped {
		t.Fatal("the move must record Flipped=true")
	}

	if !g.Undo() {
		t.Fatal("undo failed")
	}
	col = g.Tableau(0)
	if len(col) != 2 || col[0].Up || !col[1].Up {
		t.Fatalf("undo must re-hide the flipped 7 and restore the Ace, got %v", col)
	}
	if col[0].Rank != 7 || col[1].Rank != 1 {
		t.Fatalf("undo restored the wrong cards: %v", col)
	}
	if len(g.Foundation(0)) != 0 {
		t.Fatal("undo must remove the Ace from the foundation")
	}
	// (This is a crafted partial state, so the 52-card invariant is checked by
	// the fuzz test, not here.)
}

func TestAutoToFoundation(t *testing.T) {
	g := New(1, 1)
	g.waste = []Card{{Heart, 1, true}} // an Ace waiting on the waste
	g.found = [4][]Card{}
	if !g.AutoToFoundation(Pile{Waste, 0}) {
		t.Fatal("an Ace on the waste should auto-move to a foundation")
	}
	if g.AutoToFoundation(Pile{Waste, 0}) {
		t.Fatal("an empty waste has nothing to auto-move")
	}
}

// TestNoCardLostOrDuplicated fuzzes legal moves, draws and undos and asserts the
// 52-card invariant after every step.
func TestNoCardLostOrDuplicated(t *testing.T) {
	g := New(99, 3)
	r := rand.New(rand.NewSource(2024))
	for step := 0; step < 10000; step++ {
		switch {
		case r.Intn(6) == 0:
			g.Undo()
		default:
			acts := g.LegalActions()
			if len(acts) > 0 && r.Intn(3) != 0 {
				a := acts[r.Intn(len(acts))]
				if !g.Move(a.From, a.FromIdx, a.To) {
					t.Fatalf("LegalActions returned an illegal move: %+v", a)
				}
			} else {
				g.Draw()
			}
		}
		assertFullDeck(t, g)
	}
}

// TestGreedyPlayTerminates plays foundation-first with draws and asserts it
// halts (no infinite draw/recycle loop) with the deck intact.
func TestGreedyPlayTerminates(t *testing.T) {
	g := New(7, 1)
	sources := []Pile{{Waste, 0}}
	for i := 0; i < 7; i++ {
		sources = append(sources, Pile{Tableau, i})
	}
	sinceProgress := 0
	for steps := 0; steps < 200000; steps++ {
		if g.Won() {
			break
		}
		moved := false
		for _, p := range sources {
			if g.AutoToFoundation(p) {
				moved = true
				break
			}
		}
		if moved {
			sinceProgress = 0
			continue
		}
		if !g.Draw() {
			break // stock and waste exhausted
		}
		sinceProgress++
		if sinceProgress > len(g.Stock())+len(g.Waste())+1 {
			break // a full pass through the stock with no foundation move
		}
	}
	assertFullDeck(t, g)
}
