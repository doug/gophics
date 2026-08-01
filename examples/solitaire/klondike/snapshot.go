package klondike

// Snapshot is a serializable capture of a game — plain exported data (all
// fields JSON-safe), the basis for save/resume. Card, Pile and Move are already
// exported, so a Snapshot round-trips through encoding/json unchanged.
type Snapshot struct {
	Stock       []Card
	Waste       []Card
	Foundations [4][]Card
	Tableaus    [7][]Card
	History     []Move
	DrawN       int
}

// Save captures the current game as a Snapshot (deep-copied, so later play does
// not mutate it).
func (g *Game) Save() Snapshot {
	s := Snapshot{DrawN: g.drawN}
	s.Stock = append([]Card(nil), g.stock...)
	s.Waste = append([]Card(nil), g.waste...)
	for i := range g.found {
		s.Foundations[i] = append([]Card(nil), g.found[i]...)
	}
	for i := range g.tab {
		s.Tableaus[i] = append([]Card(nil), g.tab[i]...)
	}
	s.History = append([]Move(nil), g.history...)
	return s
}

// Restore rebuilds a game from a Snapshot.
func Restore(s Snapshot) *Game {
	g := &Game{drawN: s.DrawN}
	if g.drawN != 3 {
		g.drawN = 1
	}
	g.stock = append([]Card(nil), s.Stock...)
	g.waste = append([]Card(nil), s.Waste...)
	for i := range s.Foundations {
		g.found[i] = append([]Card(nil), s.Foundations[i]...)
	}
	for i := range s.Tableaus {
		g.tab[i] = append([]Card(nil), s.Tableaus[i]...)
	}
	g.history = append([]Move(nil), s.History...)
	return g
}

// CardTotal is the number of cards currently in play across every pile — 52 for
// any valid game, so a loader can reject a corrupt or truncated save.
func (g *Game) CardTotal() int {
	n := len(g.stock) + len(g.waste)
	for i := range g.found {
		n += len(g.found[i])
	}
	for i := range g.tab {
		n += len(g.tab[i])
	}
	return n
}
