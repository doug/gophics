package main

import (
	"errors"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/doug/gophics/shell"
)

// recorder is a shell.Handler that only remembers the events it was given.
type recorder struct{ events []shell.Event }

func (*recorder) Frame(shell.Window, shell.Frame, float64) {}
func (r *recorder) Event(_ shell.Window, e shell.Event)    { r.events = append(r.events, e) }

// shell.Window.Close promises the handler a Closed event, and Ebiten ends a
// game by having Update return Termination. Nothing in this example reaches
// Close at runtime — the host implements no WindowControl — so this is the
// only thing that exercises it. The closing branch returns before the input
// pump, so it runs with no Ebiten loop behind it.
func TestCloseEndsTheGameAfterClosed(t *testing.T) {
	rec := &recorder{}
	g := newGame()
	g.h = rec

	g.Close()
	err := g.Update()
	if !errors.Is(err, ebiten.Termination) {
		t.Fatalf("Update after Close returned %v, want ebiten.Termination", err)
	}

	var closed int
	for _, e := range rec.events {
		if _, ok := e.(shell.Closed); ok {
			closed++
		}
	}
	if closed != 1 {
		t.Fatalf("handler saw %d Closed events, want 1 (all events: %v)", closed, rec.events)
	}
}
