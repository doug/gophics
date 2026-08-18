package mobile

import (
	"sync"
	"testing"

	"github.com/doug/gophics/shell"
)

func TestLifecycleStartsActive(t *testing.T) {
	b := NewBridge(nil)
	if got := b.Lifecycle().State(); got != shell.StateActive {
		t.Errorf("initial state = %v, want active", got)
	}
}

func TestLifecycleTransitions(t *testing.T) {
	b := NewBridge(nil)
	var seen []shell.AppState
	b.Lifecycle().OnChange(func(s shell.AppState) { seen = append(seen, s) })

	b.SetAppState(int(shell.StateInactive))
	b.SetAppState(int(shell.StateBackground))
	b.SetAppState(int(shell.StateActive))

	want := []shell.AppState{shell.StateInactive, shell.StateBackground, shell.StateActive}
	if len(seen) != len(want) {
		t.Fatalf("saw %v, want %v", seen, want)
	}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("saw %v, want %v", seen, want)
		}
	}
	if got := b.Lifecycle().State(); got != shell.StateActive {
		t.Errorf("final State() = %v, want active", got)
	}
}

// A repeated state must not fire again. Android sends onPause before onStop and
// again on the way back, so an app persisting state on every callback would
// write repeatedly for one visit to the background.
func TestLifecycleIgnoresRepeats(t *testing.T) {
	b := NewBridge(nil)
	n := 0
	b.Lifecycle().OnChange(func(shell.AppState) { n++ })

	b.SetAppState(int(shell.StateBackground))
	b.SetAppState(int(shell.StateBackground))
	b.SetAppState(int(shell.StateBackground))
	if n != 1 {
		t.Errorf("fired %d times for one transition, want 1", n)
	}
}

// A host passing something outside the ladder is a wiring bug. Ignoring it
// beats clamping to background, which would pause an app that is running fine.
func TestLifecycleIgnoresUnknownState(t *testing.T) {
	b := NewBridge(nil)
	n := 0
	b.Lifecycle().OnChange(func(shell.AppState) { n++ })
	b.SetAppState(99)
	b.SetAppState(-1)
	if n != 0 {
		t.Errorf("fired %d times for out-of-range states, want 0", n)
	}
	if got := b.Lifecycle().State(); got != shell.StateActive {
		t.Errorf("state changed to %v on a bad value", got)
	}
}

// SetAppState arrives on the host UI thread while the widget tree reads State()
// on its own goroutine, so the state must be race-free on its own.
func TestLifecycleConcurrentReadWrite(t *testing.T) {
	b := NewBridge(nil)
	lc := b.Lifecycle()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 500 {
			b.SetAppState(i % 3)
		}
	}()
	go func() {
		defer wg.Done()
		for range 500 {
			_ = lc.State()
		}
	}()
	wg.Wait()
}
