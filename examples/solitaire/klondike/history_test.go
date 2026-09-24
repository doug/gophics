package klondike

import "testing"

// A save's history is data from a shared, editable store, and Undo used to
// trust it: a Draw against an empty waste indexed waste[-1], a Count larger
// than the destination pile sliced negative, and an unknown pile kind was a
// nil dereference. Each of these passed the 52-card check the loader applies.
func TestRestoreDropsHistoryThatDoesNotFitThePiles(t *testing.T) {
	cases := map[string][]Move{
		"draw from an empty waste": {{Draw: 1}},
		"count past the pile":      {{From: Pile{Tableau, 0}, To: Pile{Tableau, 1}, Count: 40}},
		"unknown pile kind":        {{From: Pile{Kind: 9}, To: Pile{Tableau, 0}, Count: 1}},
		"index off the end":        {{From: Pile{Foundation, 7}, To: Pile{Tableau, 0}, Count: 1}},
		"zero count":               {{From: Pile{Tableau, 0}, To: Pile{Tableau, 1}}},
		"valid then invalid":       {{Draw: 1}, {From: Pile{Tableau, 0}, To: Pile{Tableau, 1}, Count: 1}},
	}
	for name, history := range cases {
		snap := New(1, 1).Save()
		snap.History = history
		g := Restore(snap)
		if g.CardTotal() != 52 {
			t.Fatalf("%s: the deck check would already reject this save (%d cards)", name, g.CardTotal())
		}
		if g.MoveCount() != 0 {
			t.Errorf("%s: Restore kept %d moves of a history that cannot be undone", name, g.MoveCount())
		}
		if g.Undo() {
			t.Errorf("%s: Undo reported success with nothing valid to undo", name)
		}
		if g.CardTotal() != 52 {
			t.Errorf("%s: %d cards after Undo", name, g.CardTotal())
		}
	}
}

// A genuine history survives Restore's check — the trial undo runs on a copy.
func TestRestoreKeepsAValidHistory(t *testing.T) {
	g := New(3, 3)
	for range 15 {
		if acts := g.LegalActions(); len(acts) > 0 {
			g.Move(acts[0].From, acts[0].FromIdx, acts[0].To)
		} else {
			g.Draw()
		}
	}
	want := g.MoveCount()
	if want == 0 {
		t.Fatal("no moves were made")
	}
	r := Restore(g.Save())
	if r.MoveCount() != want {
		t.Fatalf("Restore kept %d of %d valid moves", r.MoveCount(), want)
	}
	for r.MoveCount() > 0 {
		if !r.Undo() {
			t.Fatal("a valid history failed to undo")
		}
	}
	if r.CardTotal() != 52 {
		t.Fatalf("%d cards after undoing everything", r.CardTotal())
	}
}

// Undo also guards itself, for a history corrupted after Restore ran.
func TestUndoRefusesAMoveThatDoesNotFit(t *testing.T) {
	g := New(1, 1)
	g.history = []Move{{Draw: 3}}
	if g.Undo() {
		t.Error("Undo succeeded against an empty waste")
	}
	if g.MoveCount() != 0 {
		t.Errorf("the bad history was kept: %d moves", g.MoveCount())
	}
	if g.CardTotal() != 52 {
		t.Errorf("%d cards", g.CardTotal())
	}
}
