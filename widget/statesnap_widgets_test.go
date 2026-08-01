package widget

import (
	"encoding/json"
	"testing"
)

// storyPage is a registered page type — a plain value struct, like an app's
// route page.
type storyPage struct{ ID int }

func (p storyPage) Build(Ctx) Widget { return Sized{W: float32(p.ID), H: 10} }

func TestScrollSnapshotRestore(t *testing.T) {
	o := &Owner{}
	o.SetRoot(Scroll{Child: Sized{W: 10, H: 5000}})
	o.FlushBuilds()

	sc := digState[Scroll](o.root).(*scrollState)
	sc.offset = 850

	snap := o.SnapshotState()
	blob, _ := json.Marshal(snap)

	var restored StateSnapshot
	if err := json.Unmarshal(blob, &restored); err != nil {
		t.Fatal(err)
	}

	o2 := &Owner{}
	o2.SetRoot(Scroll{Child: Sized{W: 10, H: 5000}})
	o2.FlushBuilds()
	if got := digState[Scroll](o2.root).(*scrollState).offset; got != 0 {
		t.Fatalf("fresh scroll offset = %v, want 0", got)
	}
	o2.RestoreState(restored)
	if got := digState[Scroll](o2.root).(*scrollState).offset; got != 850 {
		t.Errorf("restored scroll offset = %v, want 850", got)
	}
}

func TestNavigatorSnapshotRestore(t *testing.T) {
	RegisterSnapshotType[storyPage]()

	o := &Owner{}
	o.SetRoot(Navigator{Home: Sized{W: 1, H: 1}})
	o.FlushBuilds()

	// Navigate two pages deep.
	nav := digState[Navigator](o.root).(*navState)
	nav.stack = []Widget{storyPage{ID: 7}, storyPage{ID: 42}}

	snap := o.SnapshotState()
	blob, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("nav snapshot:\n%s", blob)

	var restored StateSnapshot
	if err := json.Unmarshal(blob, &restored); err != nil {
		t.Fatal(err)
	}

	// Fresh navigator at home, then restore.
	o2 := &Owner{}
	o2.SetRoot(Navigator{Home: Sized{W: 1, H: 1}})
	o2.FlushBuilds()
	nav2 := digState[Navigator](o2.root).(*navState)
	if len(nav2.stack) != 0 {
		t.Fatalf("fresh nav depth = %d, want 0 pushed", len(nav2.stack))
	}

	o2.RestoreState(restored)

	nav2 = digState[Navigator](o2.root).(*navState)
	if len(nav2.stack) != 2 {
		t.Fatalf("restored nav depth = %d, want 2 pushed", len(nav2.stack))
	}
	if p, ok := nav2.stack[1].(storyPage); !ok || p.ID != 42 {
		t.Errorf("restored top page = %#v, want storyPage{ID:42}", nav2.stack[1])
	}
}
