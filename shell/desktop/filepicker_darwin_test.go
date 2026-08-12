//go:build darwin && !ios && !js

package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doug/gophics/shell"
)

// TestExtensions covers the Accept→AppKit filter mapping: bare extensions are
// kept without their dot, MIME types are dropped (the extension filter can't use
// them), and blanks are ignored.
func TestExtensions(t *testing.T) {
	got := extensions([]string{".beancount", "bean", "text/plain", "", "  .csv "})
	want := []string{"beancount", "bean", "csv"}
	if len(got) != len(want) {
		t.Fatalf("extensions = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("extensions = %v, want %v", got, want)
		}
	}
	if got := extensions(nil); len(got) != 0 {
		t.Errorf("extensions(nil) = %v, want empty", got)
	}
	if got := extensions([]string{"image/*"}); len(got) != 0 {
		t.Errorf("MIME-only Accept should yield no extension filter, got %v", got)
	}
}

// TestOpenOffMainThreadDefersInsteadOfCrashing is the safety property that
// matters. An NSOpenPanel is an NSWindow, and AppKit aborts the whole process
// ("NSWindow should only be instantiated on the main thread!") if one is built off
// the main thread — an uncatchable Objective-C exception, not a Go panic. A Go test
// function never runs on the main thread, so this test *is* the off-main case: if
// the capability regresses to touching AppKit before dispatching, this test crashes
// the test binary rather than failing politely.
//
// With no frame loop running, nothing drains the queue, so the correct observable
// behaviour is "queued, callback not yet fired" — and no crash.
func TestOpenOffMainThreadDefersInsteadOfCrashing(t *testing.T) {
	w := &window{}
	p := macPicker{w: w}

	fired := false
	p.Open(shell.OpenOptions{Accept: []string{".beancount"}},
		func([]shell.PickedFile, error) { fired = true })
	if fired {
		t.Fatal("callback fired off the main thread; panel work must be deferred")
	}

	saveFired := false
	p.Save(shell.SaveOptions{Name: "x.beancount"}, []byte("data"), func(error) { saveFired = true })
	if saveFired {
		t.Fatal("Save callback fired off the main thread")
	}

	// Both operations should be queued for the main thread.
	w.mainMu.Lock()
	queued := len(w.mainQ)
	w.mainMu.Unlock()
	if queued != 2 {
		t.Errorf("queued %d main-thread tasks, want 2", queued)
	}

	// A nil callback must not panic or queue.
	p.Open(shell.OpenOptions{}, nil)
}

// TestQueueMainDrainOrder covers the dispatch queue itself: tasks run in FIFO
// order, draining is safe when empty, and a task that queues more work does not
// deadlock (drainMain swaps the queue out before running anything).
func TestQueueMainDrainOrder(t *testing.T) {
	w := &window{}
	w.drainMain() // empty drain must be a no-op

	var order []int
	w.queueMain(func() { order = append(order, 1) })
	w.queueMain(func() {
		order = append(order, 2)
		w.queueMain(func() { order = append(order, 3) }) // re-entrant queue
	})
	w.queueMain(nil) // ignored

	w.drainMain()
	if len(order) != 2 || order[0] != 1 || order[1] != 2 {
		t.Fatalf("first drain ran %v, want [1 2]", order)
	}
	w.drainMain() // the re-entrantly queued task runs on the next drain
	if len(order) != 3 || order[2] != 3 {
		t.Fatalf("second drain ran %v, want [1 2 3]", order)
	}
}

// TestSaveWritesFile covers the non-interactive half of Save: the bytes land on
// disk at the path the panel would have returned.
func TestSaveWritesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.beancount")
	want := []byte("2026-01-01 open Assets:Cash USD\n")
	if err := os.WriteFile(path, want, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("round trip = %q, want %q", got, want)
	}
}
