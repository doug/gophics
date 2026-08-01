package klondike

// CanAutoComplete reports whether the game can be finished automatically: the
// stock and waste are empty and every tableau card is face up, so nothing is
// hidden and greedy foundation play is guaranteed to win. This is the point a
// "Finish" affordance should appear.
func (g *Game) CanAutoComplete() bool {
	if len(g.stock) != 0 || len(g.waste) != 0 || g.Won() {
		return false
	}
	for i := range g.tab {
		for _, c := range g.tab[i] {
			if !c.Up {
				return false
			}
		}
	}
	return true
}

// AutoComplete plays every remaining card to the foundations by repeatedly
// sending each tableau's top card up until the game is won or wedged. When
// CanAutoComplete was true this always reaches a win.
func (g *Game) AutoComplete() {
	for !g.Won() {
		moved := false
		for i := range g.tab {
			if g.AutoToFoundation(Pile{Kind: Tableau, Index: i}) {
				moved = true
			}
		}
		if g.AutoToFoundation(Pile{Kind: Waste}) {
			moved = true
		}
		if !moved {
			break
		}
	}
}
