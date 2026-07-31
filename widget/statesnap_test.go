package widget

import (
	"encoding/json"
	"testing"
)

// --- demo widgets exercising both opt-in paths -----------------------------

// pager selects one of N pages; its index is the "where am I" location.
// Exported field → serialized by plain json.Marshal, zero boilerplate.
type pager struct{ Pages []Widget }

func (pager) CreateState() State { return &pagerState{} }

type pagerState struct {
	StateBase[pager]
	Index int // exported: captured for free
}

func (s *pagerState) Build(Ctx) Widget {
	pages := s.W().Pages
	if s.Index < 0 || s.Index >= len(pages) {
		return Sized{W: 1, H: 1}
	}
	return pages[s.Index]
}

// counter holds a count; exported field, serialized directly.
type counter struct{}

func (counter) CreateState() State { return &counterState{} }

type counterState struct {
	StateBase[counter]
	N int
}

func (s *counterState) Build(Ctx) Widget { return Sized{W: float32(s.N), H: 10} }

// scroller keeps an unexported offset but opts in via Snapshottable with a
// small DTO — the encapsulated path a framework widget would use.
type scroller struct{}

func (scroller) CreateState() State { return &scrollerState{} }

type scrollerState struct {
	StateBase[scroller]
	offset float32 // unexported: NOT captured by plain json
}

func (s *scrollerState) Build(Ctx) Widget { return Sized{W: 5, H: s.offset} }

type scrollDTO struct {
	Offset float32 `json:"offset"`
}

func (s *scrollerState) SaveState() any { return scrollDTO{Offset: s.offset} }
func (s *scrollerState) LoadState(d json.RawMessage) {
	var dto scrollDTO
	if json.Unmarshal(d, &dto) == nil {
		s.offset = dto.Offset
	}
}

// transient has neither exported fields nor Snapshottable → must be skipped.
type transient struct{}

func (transient) CreateState() State { return &transientState{} }

type transientState struct {
	StateBase[transient]
	secret int // unexported, no opt-in
}

func (s *transientState) Build(Ctx) Widget { return Sized{W: 1, H: 1} }

// app is the root: a pager over two pages, each carrying a counter + scroller,
// plus a transient widget that should never appear in the snapshot.
type app struct{}

func (app) CreateState() State { return &appState{} }

type appState struct{ StateBase[app] }

func (s *appState) Build(Ctx) Widget {
	page := func() Widget {
		return Column(counter{}, scroller{}, transient{})
	}
	return pager{Pages: []Widget{page(), page()}}
}

// digState finds the first element whose widget is of the given concrete type.
func digState[W any](el *element) State {
	if _, ok := el.widget.(W); ok {
		return el.state
	}
	for _, c := range el.childElements() {
		if s := digState[W](c); s != nil {
			return s
		}
	}
	return nil
}

func TestStateSnapshotRestore(t *testing.T) {
	// 1. Build the tree and drive it to a specific place: page 2, count 7,
	//    scrolled to 120.
	o1 := &Owner{}
	o1.SetRoot(app{})
	o1.FlushBuilds()

	pg := digState[pager](o1.root).(*pagerState)
	pg.Index = 1
	o1.RebuildAll()
	o1.FlushBuilds() // now showing page 2
	digState[counter](o1.root).(*counterState).N = 7
	digState[scroller](o1.root).(*scrollerState).offset = 120

	// 2. Snapshot, then round-trip through JSON bytes — exactly what a
	//    hot-restart does (old process writes a file, new process reads it).
	snap := o1.SnapshotState()
	blob, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	t.Logf("snapshot (%d entries):\n%s", len(snap), blob)

	var restored StateSnapshot
	if err := json.Unmarshal(blob, &restored); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}

	// The transient widget must not be in the snapshot.
	for path := range restored {
		if containsType(path, "transient") {
			t.Fatalf("transient state leaked into snapshot at %s", path)
		}
	}

	// 3. Fresh tree (simulating the rebuilt/relaunched app), then restore.
	o2 := &Owner{}
	o2.SetRoot(app{})
	o2.FlushBuilds()

	// Sanity: the fresh tree starts at defaults (page 1, count 0, offset 0).
	if pg2 := digState[pager](o2.root).(*pagerState); pg2.Index != 0 {
		t.Fatalf("fresh pager index = %d, want 0", pg2.Index)
	}

	o2.RestoreState(restored)

	// 4. We must be exactly where we were.
	if got := digState[pager](o2.root).(*pagerState).Index; got != 1 {
		t.Errorf("restored pager index = %d, want 1", got)
	}
	if got := digState[counter](o2.root).(*counterState).N; got != 7 {
		t.Errorf("restored counter N = %d, want 7", got)
	}
	if got := digState[scroller](o2.root).(*scrollerState).offset; got != 120 {
		t.Errorf("restored scroller offset = %v, want 120", got)
	}
}

func containsType(path, name string) bool {
	for i := 0; i+len(name) <= len(path); i++ {
		if path[i:i+len(name)] == name {
			return true
		}
	}
	return false
}
