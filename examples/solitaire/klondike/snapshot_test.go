package klondike

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSnapshotRoundTrip(t *testing.T) {
	g := New(5, 3)
	// Play a bit so the snapshot has non-trivial waste/foundation/history state.
	for i := 0; i < 20; i++ {
		acts := g.LegalActions()
		if len(acts) > 0 {
			a := acts[0]
			g.Move(a.From, a.FromIdx, a.To)
		} else {
			g.Draw()
		}
	}

	// Through JSON and back must be identical.
	data, err := json.Marshal(g.Save())
	if err != nil {
		t.Fatal(err)
	}
	var snap Snapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		t.Fatal(err)
	}
	g2 := Restore(snap)

	if !reflect.DeepEqual(g.Save(), g2.Save()) {
		t.Fatal("restored game differs from the original")
	}
	if g2.CardTotal() != 52 {
		t.Fatalf("restored game has %d cards, want 52", g2.CardTotal())
	}
	if g2.DrawCount() != 3 {
		t.Fatalf("restored draw count = %d, want 3", g2.DrawCount())
	}

	// The restored game keeps playing correctly (undo still reverses).
	before := g2.MoveCount()
	if g2.Draw() && !g2.Undo() {
		t.Fatal("restored game does not undo")
	}
	if g2.MoveCount() != before {
		t.Fatalf("move count drifted after draw+undo: %d != %d", g2.MoveCount(), before)
	}
}
