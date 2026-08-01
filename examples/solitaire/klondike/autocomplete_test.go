package klondike

import "testing"

// winnableFaceUp builds an end-state where every card is face up on the tableaus
// (stock/waste empty): four columns, each a full suit stacked King→Ace so the
// Ace is on top and each column unwinds straight to its foundation.
func winnableFaceUp() *Game {
	g := &Game{drawN: 1}
	for s := Club; s <= Spade; s++ {
		for r := 13; r >= 1; r-- {
			g.tab[int(s)] = append(g.tab[int(s)], Card{Suit: s, Rank: uint8(r), Up: true})
		}
	}
	return g
}

func TestAutoComplete(t *testing.T) {
	g := winnableFaceUp()
	if !g.CanAutoComplete() {
		t.Fatal("an all-face-up, empty-stock game should be auto-completable")
	}
	g.AutoComplete()
	if !g.Won() {
		t.Fatalf("auto-complete did not win; foundations hold %d cards", g.CardTotal()-tableauCards(g))
	}
	if g.CardTotal() != 52 {
		t.Fatalf("cards lost/duplicated: total %d", g.CardTotal())
	}
	if g.CanAutoComplete() {
		t.Fatal("a won game is not auto-completable")
	}
}

func tableauCards(g *Game) int {
	n := 0
	for i := range g.tab {
		n += len(g.tab[i])
	}
	return n
}

func TestCannotAutoCompleteWithHiddenCards(t *testing.T) {
	g := New(1, 1) // a fresh deal still has face-down cards + a full stock
	if g.CanAutoComplete() {
		t.Fatal("a fresh deal must not be auto-completable")
	}
}
