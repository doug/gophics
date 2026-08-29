package widget

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/layout"
)

// captureLog redirects the standard logger into a buffer for the test's
// duration, returning the buffer.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// Two siblings sharing a reconciliation key is an app data bug (M3). The
// reconciler must not insert the same *element twice (double paint, corrupted
// unmount): the duplicate gets a fresh element, and a warning naming the key
// is logged once.
func TestDuplicateKeyMountsFreshAndWarns(t *testing.T) {
	buf := captureLog(t)
	const key = "dup-key-m3-test"

	o := newOwner()
	row := func() Widget {
		return Row(
			WithKey{Key: key, Child: probe{ID: "a"}},
			WithKey{Key: key, Child: probe{ID: "b"}},
		)
	}
	o.SetRoot(row())
	sa, sb := stateOf(o.root, "a"), stateOf(o.root, "b")
	if sa == nil || sb == nil {
		t.Fatal("initial mount incomplete")
	}

	// Reconcile: the duplicate key is now detected.
	o.SetRoot(row())

	kids := o.root.kids
	if len(kids) != 2 {
		t.Fatalf("got %d children, want 2", len(kids))
	}
	if kids[0] == kids[1] {
		t.Fatal("the same *element landed in the child list twice (M3 regression)")
	}
	for _, k := range kids {
		if !k.mounted {
			t.Fatal("child element not mounted after duplicate-key reconcile")
		}
	}

	// Exactly one of the two old elements can survive; the other unmounts.
	// Its probe state must be disposed exactly once.
	if got := sa.disposes + sb.disposes; got != 1 {
		t.Fatalf("disposes across old states = %d, want exactly 1", got)
	}

	// The diagnostic is unmissable and names the key.
	logged := buf.String()
	if !strings.Contains(logged, "duplicate reconciliation key") {
		t.Fatalf("no duplicate-key warning logged; log: %q", logged)
	}
	if !strings.Contains(logged, key) {
		t.Fatalf("warning does not name the key %q; log: %q", key, logged)
	}

	// Once per key: a further reconcile must not repeat the warning.
	o.SetRoot(row())
	if got := strings.Count(buf.String(), "duplicate reconciliation key"); got != 1 {
		t.Fatalf("warning logged %d times, want once per key", got)
	}

	// The tree stays usable: layout must not panic and both children must
	// occupy distinct slots.
	fx := o.RootBox().(*layoutbox.Flex)
	fx.Layout(layout.Unbounded())
	if len(fx.Children) != 2 {
		t.Fatalf("flex has %d children after duplicate-key reconcile, want 2", len(fx.Children))
	}
	if fx.Children[0].Box == fx.Children[1].Box {
		t.Fatal("flex attached the same box twice")
	}
}

// A duplicate among brand-new keyed children (no old element for the key yet)
// is detected on the same reconcile via the seen-key set.
func TestDuplicateKeyAmongNewChildrenWarns(t *testing.T) {
	buf := captureLog(t)
	const key = "dup-key-new-children-test"

	o := newOwner()
	o.SetRoot(Row(Sized{W: 1, H: 1}))
	// Reconcile from a keyless list straight into a duplicated pair: neither
	// child has an old element, but the duplicate must still be reported.
	o.SetRoot(Row(
		WithKey{Key: key, Child: probe{ID: "a"}},
		WithKey{Key: key, Child: probe{ID: "b"}},
	))
	if !strings.Contains(buf.String(), key) {
		t.Fatalf("duplicate among new children not reported; log: %q", buf.String())
	}
	if kids := o.root.kids; len(kids) != 2 || kids[0] == kids[1] {
		t.Fatalf("children corrupted: %d kids", len(kids))
	}
}

// Unkeyed children match positionally; a type change at a position must
// dispose the old state and mount fresh (through reconcileRenderChildren,
// complementing TestTypeChangeRemounts which exercises the root path).
func TestUnkeyedTypeChangeRemountsInList(t *testing.T) {
	o := newOwner()
	o.SetRoot(Column(probe{ID: "x"}))
	sx := stateOf(o.root, "x")
	if sx == nil || sx.inits != 1 {
		t.Fatalf("bad initial state: %+v", sx)
	}

	o.SetRoot(Column(Sized{W: 5, H: 5}))
	if sx.disposes != 1 {
		t.Fatalf("type change did not dispose old state (disposes=%d)", sx.disposes)
	}

	o.SetRoot(Column(probe{ID: "x"}))
	s2 := stateOf(o.root, "x")
	if s2 == nil || s2 == sx {
		t.Fatal("state must be fresh after remount")
	}
	if s2.inits != 1 {
		t.Fatalf("fresh state inits = %d, want 1", s2.inits)
	}
}

// Keyed and unkeyed children reconcile independently: swapping the keyed pair
// around a fixed unkeyed middle preserves all three states.
func TestMixedKeyedUnkeyedReorder(t *testing.T) {
	o := newOwner()
	o.SetRoot(Row(
		WithKey{Key: "k1", Child: probe{ID: "a"}},
		probe{ID: "u"},
		WithKey{Key: "k2", Child: probe{ID: "b"}},
	))
	sa, su, sb := stateOf(o.root, "a"), stateOf(o.root, "u"), stateOf(o.root, "b")
	sa.local, su.local, sb.local = 1, 2, 3

	o.SetRoot(Row(
		WithKey{Key: "k2", Child: probe{ID: "b"}},
		probe{ID: "u"},
		WithKey{Key: "k1", Child: probe{ID: "a"}},
	))
	if got := stateOf(o.root, "a"); got != sa {
		t.Fatal("keyed child a lost state across reorder")
	}
	if got := stateOf(o.root, "b"); got != sb {
		t.Fatal("keyed child b lost state across reorder")
	}
	if got := stateOf(o.root, "u"); got != su {
		t.Fatal("unkeyed positional child lost state")
	}
	if sa.disposes+su.disposes+sb.disposes != 0 {
		t.Fatal("reorder disposed a surviving child")
	}
}
