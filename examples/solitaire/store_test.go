package main

import (
	"strings"
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/examples/solitaire/klondike"
	"github.com/doug/gophics/shell"
)

// The save round-trips through the preference store, under a key that names the
// app. The gallery serves every demo from one origin, so the shell's own
// "gophics.pref." prefix does not separate them — two demos both saving under
// "game" would overwrite each other, and the symptom would be a solitaire deal
// that resumes as something else entirely.
func TestPrefsStoreRoundTrip(t *testing.T) {
	p := apptest.NewPrefs(nil)
	s := newPrefsStore(p)
	if _, ok := s.load(); ok {
		t.Fatal("loaded a save from an empty preference store")
	}

	s.save([]byte(`{"deal":7}`))
	got, ok := s.load()
	if !ok {
		t.Fatal("save then load returned nothing")
	}
	if string(got) != `{"deal":7}` {
		t.Errorf("loaded %q, want the bytes that were saved", got)
	}
	if keys := p.Keys(); len(keys) != 1 || keys[0] != prefKey {
		t.Errorf("wrote keys %v, want exactly [%s]", keys, prefKey)
	}
	if !strings.Contains(prefKey, "solitaire") {
		t.Errorf("prefKey is %q and does not name the app: the shell prefix namespaces the framework, not this game, so a bare key like \"game\" collides with the other demos sharing this origin", prefKey)
	}
}

// No preference store means no persistence, not a crash. Preferences() is nil
// wherever the platform cannot persist at all (a sandboxed browser context),
// and the game has to stay playable there: resume, load and persist all treat
// a nil store as "just deal".
func TestNoPreferencesStillPlays(t *testing.T) {
	var p shell.Preferences // nil
	if s := newPrefsStore(p); s != nil {
		t.Fatalf("newPrefsStore(nil) = %v, want a nil store", s)
	}
	s := &gameState{g: klondike.New(1, 1)}
	if s.resume(p) {
		t.Error("resume with no Preferences reported a resumed game")
	}
	if g, ok := s.load(); ok || g != nil {
		t.Errorf("load with no store = %v, %v; want nothing", g, ok)
	}
	s.persist() // must not panic
}
