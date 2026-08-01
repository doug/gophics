// Package klondike is the pure rules engine for Klondike solitaire: deck,
// piles, legal moves, an O(1) undo journal, and win detection — with no
// rendering, no framework imports, and no global state, so it is exhaustively
// unit-testable with plain `go test`. The example's UI (a widget.Canvas board)
// is built on top of this.
package klondike

import "fmt"

// Suit of a card. The foundation and tableau rules care only about a suit's
// color (red = Diamond/Heart), never its identity.
type Suit uint8

const (
	Club Suit = iota
	Diamond
	Heart
	Spade
)

// Red reports whether the suit is red (Diamond or Heart).
func (s Suit) Red() bool { return s == Diamond || s == Heart }

func (s Suit) String() string {
	switch s {
	case Club:
		return "C"
	case Diamond:
		return "D"
	case Heart:
		return "H"
	default:
		return "S"
	}
}

// Card is one playing card. Rank is 1 (Ace) through 13 (King). Up reports
// whether the card is face up (visible and playable).
type Card struct {
	Suit Suit
	Rank uint8
	Up   bool
}

func (c Card) String() string {
	r := map[uint8]string{1: "A", 11: "J", 12: "Q", 13: "K"}[c.Rank]
	if r == "" {
		r = fmt.Sprintf("%d", c.Rank)
	}
	s := r + c.Suit.String()
	if !c.Up {
		return "(" + s + ")"
	}
	return s
}

// standardDeck returns the 52 distinct cards, face down, in suit-then-rank order.
func standardDeck() []Card {
	d := make([]Card, 0, 52)
	for s := Club; s <= Spade; s++ {
		for r := uint8(1); r <= 13; r++ {
			d = append(d, Card{Suit: s, Rank: r})
		}
	}
	return d
}
