package main

import "testing"

// The readout the overlay exists to show is the host's clock. A game that
// started at speed 0 stood still until someone pressed "faster", which is
// the "broken UI" the README warns about — with nothing broken in the UI.
func TestNewGameRuns(t *testing.T) {
	g := newGame()
	if g.Speed() != 1 {
		t.Fatalf("speed = %v at start, want 1", g.Speed())
	}
	for range 60 {
		g.advance()
	}
	if g.Elapsed() < 0.99 || g.Elapsed() > 1.01 {
		t.Fatalf("elapsed after 60 ticks = %v, want ~1s", g.Elapsed())
	}
	g.TogglePause()
	was := g.Elapsed()
	g.advance()
	if g.Elapsed() != was {
		t.Fatal("the clock advanced while paused")
	}
}
