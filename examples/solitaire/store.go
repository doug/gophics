package main

import (
	"github.com/doug/gophics/examples/solitaire/klondike"
	"github.com/doug/gophics/shell"
)

// store persists the current game between runs.
//
// There is no platform split here, and that is the point. This used to be two
// build-tagged files — localStorage on web, a JSON file under the user's config
// directory on desktop — which is precisely what shell.Preferences already is
// on every platform gophics runs on, mobile included. An app writing that split
// itself is reimplementing a capability the framework ships.
//
// makeStore is a var so tests can substitute an in-memory slot.
type store interface {
	save(data []byte)
	load() ([]byte, bool)
}

var makeStore = newPrefsStore

// prefKey names this game's save. The app name is part of the key because the
// shell's own prefix ("gophics.pref.") namespaces the framework, not the app,
// and every demo on gophics.com shares one origin and therefore one
// localStorage.
const prefKey = "solitaire.game"

// prefsStore autosaves the game through the Preferences capability.
type prefsStore struct{ p shell.Preferences }

// newPrefsStore returns nil where the platform has no preference store — a
// sandboxed browser context, say. Callers already treat a nil store as "do not
// persist", so a game that cannot be saved still deals and plays.
func newPrefsStore(p shell.Preferences) store {
	if p == nil {
		return nil
	}
	return prefsStore{p}
}

func (s prefsStore) save(data []byte) { _ = s.p.Set(prefKey, string(data)) }

func (s prefsStore) load() ([]byte, bool) {
	v, ok := s.p.Get(prefKey)
	if !ok {
		return nil, false
	}
	return []byte(v), true
}

// fullDeck reports whether g is a complete, non-corrupt game (52 cards) — used
// to reject a bad save and start fresh instead.
func fullDeck(g *klondike.Game) bool { return g.CardTotal() == 52 }
