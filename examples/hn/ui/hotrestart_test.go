package ui

import (
	"encoding/json"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// waitLabel settles the app until a semantics label containing substr appears,
// or the deadline passes; reports whether it appeared.
func waitLabel(h *app.Headless, substr string) bool {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		settle(h)
		if hasLabel(h, substr) {
			return true
		}
		time.Sleep(time.Millisecond)
		h.Render()
	}
	return hasLabel(h, substr)
}

func mountHN(t *testing.T) *app.Headless {
	t.Helper()
	h, err := app.NewHeadless(HN{API: fakeAPI{stories: 500, commentsPer: 5}, PageSize: 500},
		app.Config{Size: geom.Size{W: 480, H: 720}, Background: colBg, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

// TestHotRestartRestoresThread is the end-to-end proof: navigate into a story's
// thread, snapshot the app through JSON (the exact hot-restart path), restore
// into a freshly booted app, and confirm it lands back on the thread — not the
// feed. Relies on the thread page's Back button, which the feed lacks.
func TestHotRestartRestoresThread(t *testing.T) {
	h, _ := harness(t) // feed loaded
	if hasLabel(h, "Back") {
		t.Fatal("feed unexpectedly shows a Back button")
	}

	h.Tap(geom.Pt{X: 240, Y: 80}) // open the first story's thread
	if !waitLabel(h, "Back") {
		t.Fatal("tapping a story did not open a thread page")
	}

	// Snapshot and round-trip through JSON — what a restart does.
	blob, err := json.Marshal(h.Core.Owner.SnapshotState())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("app snapshot: %s", blob)
	var snap widget.StateSnapshot
	if err := json.Unmarshal(blob, &snap); err != nil {
		t.Fatal(err)
	}

	// A fresh app starts on the feed; restore should put it back on the thread.
	h2 := mountHN(t)
	if hasLabel(h2, "Back") {
		t.Fatal("fresh app should start on the feed, not a thread")
	}
	h2.Core.Owner.RestoreState(snap)
	if !waitLabel(h2, "Back") {
		t.Error("restored app did not land back on the thread page")
	}
}
