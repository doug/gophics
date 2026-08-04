package main

import "github.com/doug/gophics/examples/solitaire/klondike"

// store persists the current game between runs. Desktop uses a file
// (store_os.go); web uses localStorage (store_web.go). makeStore is a var so
// tests can substitute an in-memory store.
type store interface {
	save(data []byte)
	load() ([]byte, bool)
}

var makeStore = platformStore

// fullDeck reports whether g is a complete, non-corrupt game (52 cards) — used
// to reject a bad save and start fresh instead.
func fullDeck(g *klondike.Game) bool { return g.CardTotal() == 52 }
