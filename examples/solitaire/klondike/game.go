package klondike

import (
	"math/rand"
	"slices"
)

// PileKind identifies a family of stacks in the Klondike layout.
type PileKind uint8

const (
	Stock      PileKind = iota // the face-down draw pile
	Waste                      // cards drawn from the stock, face up
	Foundation                 // four suit stacks, built up Ace→King
	Tableau                    // seven columns, built down by alternating color
)

// Pile addresses a specific stack. Index selects the foundation (0..3) or
// tableau (0..6); it is unused for Stock/Waste.
type Pile struct {
	Kind  PileKind
	Index int
}

// Move is one applied action, recorded so it can be undone exactly.
type Move struct {
	From, To Pile
	Count    int  // cards moved (a tableau run may be >1)
	Flipped  bool // the move exposed and flipped up a face-down source card
	Draw     int  // this move drew Draw cards stock→waste (0 if not a draw)
	Recycle  bool // this move recycled the waste back into the stock
}

// Game is a Klondike deal in progress. All state is plain data; copy the piles
// out through the accessors to render them.
type Game struct {
	stock   []Card
	waste   []Card
	found   [4][]Card
	tab     [7][]Card
	history []Move
	drawN   int
}

// New deals a game from seed (deterministic) drawing drawN cards at a time
// (1 or 3; anything else means 1).
func New(seed int64, drawN int) *Game {
	if drawN != 3 {
		drawN = 1
	}
	deck := standardDeck()
	rand.New(rand.NewSource(seed)).Shuffle(len(deck), func(i, j int) {
		deck[i], deck[j] = deck[j], deck[i]
	})
	g := &Game{drawN: drawN}
	k := 0
	for col := range 7 {
		for row := 0; row <= col; row++ {
			c := deck[k]
			k++
			c.Up = row == col // only the last card in each column is face up
			g.tab[col] = append(g.tab[col], c)
		}
	}
	for ; k < len(deck); k++ {
		g.stock = append(g.stock, deck[k]) // face down
	}
	return g
}

// Accessors (read-only views for rendering and tests).
func (g *Game) DrawCount() int          { return g.drawN }
func (g *Game) Stock() []Card           { return g.stock }
func (g *Game) Waste() []Card           { return g.waste }
func (g *Game) Foundation(i int) []Card { return g.found[i] }
func (g *Game) Tableau(i int) []Card    { return g.tab[i] }
func (g *Game) MoveCount() int          { return len(g.history) }

func (g *Game) pile(p Pile) *[]Card {
	switch p.Kind {
	case Stock:
		return &g.stock
	case Waste:
		return &g.waste
	case Foundation:
		return &g.found[p.Index]
	case Tableau:
		return &g.tab[p.Index]
	}
	return nil
}

// Draw turns the next drawN cards from the stock onto the waste, or — when the
// stock is empty — recycles the waste back into a fresh stock. Reports whether
// anything happened (false only when stock and waste are both empty).
func (g *Game) Draw() bool {
	if len(g.stock) == 0 {
		if len(g.waste) == 0 {
			return false
		}
		n := len(g.waste)
		for i := n - 1; i >= 0; i-- {
			c := g.waste[i]
			c.Up = false
			g.stock = append(g.stock, c)
		}
		g.waste = g.waste[:0]
		g.history = append(g.history, Move{Recycle: true, Count: n})
		return true
	}
	n := min(g.drawN, len(g.stock))
	for i := 0; i < n; i++ {
		c := g.stock[len(g.stock)-1]
		g.stock = g.stock[:len(g.stock)-1]
		c.Up = true
		g.waste = append(g.waste, c)
	}
	g.history = append(g.history, Move{Draw: n})
	return true
}

// canPlace reports whether the run `moving` (its first element is the card that
// will touch the destination) may legally land on `to`.
func (g *Game) canPlace(moving []Card, to Pile) bool {
	if len(moving) == 0 {
		return false
	}
	bottom := moving[0]
	switch to.Kind {
	case Foundation:
		if len(moving) != 1 {
			return false
		}
		f := g.found[to.Index]
		if len(f) == 0 {
			return bottom.Rank == 1 // an Ace starts a foundation
		}
		t := f[len(f)-1]
		return bottom.Suit == t.Suit && bottom.Rank == t.Rank+1
	case Tableau:
		d := g.tab[to.Index]
		if len(d) == 0 {
			return bottom.Rank == 13 // only a King may start an empty column
		}
		t := d[len(d)-1]
		return t.Up && t.Suit.Red() != bottom.Suit.Red() && bottom.Rank == t.Rank-1
	default:
		return false // nothing may be placed onto the stock or waste
	}
}

// CanMove reports whether the cards from fromIdx to the top of `from` may move
// onto `to`.
func (g *Game) CanMove(from Pile, fromIdx int, to Pile) bool {
	src := g.pile(from)
	if src == nil || from.Kind == Stock || from == to {
		return false
	}
	if fromIdx < 0 || fromIdx >= len(*src) {
		return false
	}
	// Only a tableau exposes a multi-card run; elsewhere only the top card moves.
	if from.Kind != Tableau && fromIdx != len(*src)-1 {
		return false
	}
	moving := (*src)[fromIdx:]
	for _, c := range moving {
		if !c.Up {
			return false
		}
	}
	return g.canPlace(moving, to)
}

// Move applies the move if legal (flipping a newly exposed source card face up)
// and returns whether it did.
func (g *Game) Move(from Pile, fromIdx int, to Pile) bool {
	if !g.CanMove(from, fromIdx, to) {
		return false
	}
	src := g.pile(from)
	dst := g.pile(to)
	moving := (*src)[fromIdx:]
	n := len(moving)
	run := make([]Card, n)
	copy(run, moving)
	*dst = append(*dst, run...)
	*src = (*src)[:fromIdx]

	flipped := false
	if from.Kind == Tableau && len(*src) > 0 && !(*src)[len(*src)-1].Up {
		(*src)[len(*src)-1].Up = true
		flipped = true
	}
	g.history = append(g.history, Move{From: from, To: to, Count: n, Flipped: flipped})
	return true
}

// AutoToFoundation moves the top card of p onto a legal foundation if one
// accepts it. This is the single-tap / double-click convenience.
func (g *Game) AutoToFoundation(p Pile) bool {
	src := g.pile(p)
	if src == nil || p.Kind == Stock || len(*src) == 0 {
		return false
	}
	idx := len(*src) - 1
	if !(*src)[idx].Up {
		return false
	}
	card := (*src)[idx]
	for i := range 4 {
		if g.canPlace([]Card{card}, Pile{Foundation, i}) {
			return g.Move(p, idx, Pile{Foundation, i})
		}
	}
	return false
}

// Undo reverses the most recent move exactly, including re-hiding a card that
// the move had flipped up — the classic Klondike undo bug, made explicit by the
// Move.Flipped bit. Reports whether there was anything to undo.
func (g *Game) Undo() bool {
	if len(g.history) == 0 {
		return false
	}
	m := g.history[len(g.history)-1]
	g.history = g.history[:len(g.history)-1]
	switch {
	case m.Recycle:
		// The recycle emptied the waste into the stock; put it all back.
		for _, c := range slices.Backward(g.stock) {

			c.Up = true
			g.waste = append(g.waste, c)
		}
		g.stock = g.stock[:0]
	case m.Draw > 0:
		for i := 0; i < m.Draw; i++ {
			c := g.waste[len(g.waste)-1]
			g.waste = g.waste[:len(g.waste)-1]
			c.Up = false
			g.stock = append(g.stock, c)
		}
	default:
		src := g.pile(m.From)
		dst := g.pile(m.To)
		if m.Flipped && len(*src) > 0 {
			(*src)[len(*src)-1].Up = false // re-hide before the run lands back on it
		}
		moving := (*dst)[len(*dst)-m.Count:]
		run := make([]Card, m.Count)
		copy(run, moving)
		*dst = (*dst)[:len(*dst)-m.Count]
		*src = append(*src, run...)
	}
	return true
}

// Won reports whether all 52 cards have reached the foundations.
func (g *Game) Won() bool {
	n := 0
	for i := range 4 {
		n += len(g.found[i])
	}
	return n == 52
}

// Action is a concrete legal move (source run → destination).
type Action struct {
	From    Pile
	FromIdx int
	To      Pile
}

// LegalActions lists every card move available right now (not including drawing
// from the stock). Used by hints, the auto-player, and the fuzz test.
func (g *Game) LegalActions() []Action {
	var out []Action
	dests := make([]Pile, 0, 11)
	for i := range 4 {
		dests = append(dests, Pile{Foundation, i})
	}
	for i := range 7 {
		dests = append(dests, Pile{Tableau, i})
	}
	consider := func(from Pile, idx int) {
		for _, to := range dests {
			if g.CanMove(from, idx, to) {
				out = append(out, Action{from, idx, to})
			}
		}
	}
	if len(g.waste) > 0 {
		consider(Pile{Waste, 0}, len(g.waste)-1)
	}
	for i := range 4 {
		if len(g.found[i]) > 0 {
			consider(Pile{Foundation, i}, len(g.found[i])-1)
		}
	}
	for i := range 7 {
		t := g.tab[i]
		for j := range t {
			if t[j].Up {
				consider(Pile{Tableau, i}, j)
			}
		}
	}
	return out
}
