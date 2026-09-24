package widget

import "testing"

// A Modal registers with its owner for as long as it is mounted, and the
// innermost (most recently mounted) one is the one Escape reaches.
func TestModalInnermostTakesEscape(t *testing.T) {
	o := newOwner()
	var got []string
	inner := func() { got = append(got, "inner") }
	outer := func() { got = append(got, "outer") }

	o.SetRoot(Modal{OnEscape: outer, Child: Modal{OnEscape: inner, Child: Sized{W: 1, H: 1}}})
	o.FlushBuilds()
	f := o.TopModal()
	if f == nil {
		t.Fatal("no modal registered")
	}
	f()
	if len(got) != 1 || got[0] != "inner" {
		t.Fatalf("Escape reached %v, want the inner modal", got)
	}

	// Unmounting the inner layer hands Escape back to the outer one.
	o.SetRoot(Modal{OnEscape: outer, Child: Sized{W: 1, H: 1}})
	o.FlushBuilds()
	got = nil
	o.TopModal()()
	if len(got) != 1 || got[0] != "outer" {
		t.Fatalf("Escape reached %v after the inner modal unmounted, want outer", got)
	}

	o.SetRoot(Sized{W: 1, H: 1})
	o.FlushBuilds()
	if o.TopModal() != nil {
		t.Fatal("a modal stayed registered after unmount")
	}
}

// The handler is read at dispatch time, not at mount: a rebuild that changes
// OnEscape is honoured without remounting the layer.
func TestModalUsesCurrentHandler(t *testing.T) {
	o := newOwner()
	n := 0
	o.SetRoot(Modal{OnEscape: func() { n = 1 }, Child: Sized{W: 1, H: 1}})
	o.FlushBuilds()
	o.SetRoot(Modal{OnEscape: func() { n = 2 }, Child: Sized{W: 1, H: 1}})
	o.FlushBuilds()
	o.TopModal()()
	if n != 2 {
		t.Fatalf("Escape ran handler %d, want the rebuilt one (2)", n)
	}
	if len(o.modals) != 1 {
		t.Fatalf("%d modals registered after a rebuild, want 1", len(o.modals))
	}
}
