package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// probe is a stateful widget whose state records its identity and lifecycle.
type probe struct {
	ID    string
	Value int
}

func (p probe) CreateState() State { return &probeState{} }

type probeState struct {
	StateBase[probe]
	inits, disposes int
	local           int // survives rebuilds, dies on remount
}

func (s *probeState) Init(Ctx) { s.inits++ }
func (s *probeState) Dispose() { s.disposes++ }
func (s *probeState) Build(ctx Ctx) Widget {
	return Sized{W: 10 + float32(s.local), H: 10}
}

// stateOf digs the probeState out of an element tree by widget ID.
func stateOf(el *element, id string) *probeState {
	if p, ok := el.widget.(probe); ok && p.ID == id {
		return el.state.(*probeState)
	}
	if el.child != nil {
		if s := stateOf(el.child, id); s != nil {
			return s
		}
	}
	for _, k := range el.kids {
		if s := stateOf(k, id); s != nil {
			return s
		}
	}
	return nil
}

func newOwner() *Owner { return &Owner{} }

func TestStatefulRebuildPreservesState(t *testing.T) {
	o := newOwner()
	o.SetRoot(probe{ID: "a"})
	s := stateOf(o.root, "a")
	if s == nil || s.inits != 1 {
		t.Fatalf("state not initialized: %+v", s)
	}

	s.local = 5
	s.SetState(nil)
	o.FlushBuilds()
	if got := o.RootBox().Layout(layout.Unbounded()); got.W != 15 {
		t.Fatalf("rebuild did not reflect state: W=%v, want 15", got.W)
	}

	// Reconcile to a new widget value of the same type: state must survive.
	o.SetRoot(probe{ID: "a", Value: 2})
	if s2 := stateOf(o.root, "a"); s2 != s {
		t.Fatal("state was recreated on same-type reconcile")
	}
	if s.W().Value != 2 {
		t.Fatalf("state widget not updated: %+v", s.W())
	}
}

func TestTypeChangeRemounts(t *testing.T) {
	o := newOwner()
	o.SetRoot(probe{ID: "a"})
	s := stateOf(o.root, "a")
	o.SetRoot(Sized{W: 1, H: 1})
	if s.disposes != 1 {
		t.Fatal("old state not disposed on type change")
	}
	o.SetRoot(probe{ID: "a"})
	if s2 := stateOf(o.root, "a"); s2 == s {
		t.Fatal("state must be fresh after remount")
	}
}

func TestKeyedReorderPreservesState(t *testing.T) {
	o := newOwner()
	row := func(ids ...string) Widget {
		var children []Widget
		for _, id := range ids {
			children = append(children, WithKey{Key: id, Child: probe{ID: id}})
		}
		return Row(children...)
	}
	o.SetRoot(row("a", "b", "c"))
	sa, sb := stateOf(o.root, "a"), stateOf(o.root, "b")
	sa.local = 7

	o.SetRoot(row("c", "a", "b"))
	if got := stateOf(o.root, "a"); got != sa {
		t.Fatal("keyed reorder lost state identity for a")
	}
	if got := stateOf(o.root, "b"); got != sb {
		t.Fatal("keyed reorder lost state identity for b")
	}
	if sa.disposes != 0 {
		t.Fatal("keyed child was disposed during reorder")
	}

	// Removal disposes.
	o.SetRoot(row("a"))
	if sb.disposes != 1 {
		t.Fatal("removed keyed child not disposed")
	}
}

func TestUnkeyedPositionalReuse(t *testing.T) {
	o := newOwner()
	o.SetRoot(Row(probe{ID: "x"}, probe{ID: "y"}))
	sx := stateOf(o.root, "x")
	// Same shape, new values: positional reuse keeps state.
	o.SetRoot(Row(probe{ID: "x", Value: 1}, probe{ID: "y", Value: 1}))
	if stateOf(o.root, "x") != sx {
		t.Fatal("positional reuse failed")
	}
}

func TestFlexAttachEnforcesFactors(t *testing.T) {
	o := newOwner()
	o.SetRoot(Row(
		Sized{W: 30, H: 10},
		Expand(Sized{H: 10}),
	))
	box := o.RootBox().(*layout.Flex)
	box.Layout(layout.Tight(geom.Size{W: 100, H: 10}))
	if got := box.Children[1].Box.Size().W; got != 70 {
		t.Fatalf("expanded child W = %v, want 70", got)
	}
}

func TestBuildDirtyPropagation(t *testing.T) {
	o := newOwner()
	frames := 0
	o.RequestFrame = func() { frames++ }
	o.SetRoot(Column(probe{ID: "a"}, probe{ID: "b"}))
	s := stateOf(o.root, "a")
	s.SetState(func() { s.local = 3 })
	if frames == 0 {
		t.Fatal("SetState must request a frame")
	}
	o.FlushBuilds()
	// Only "a" rebuilt; "b" untouched. a's box width reflects local=3.
	fx := o.RootBox().(*layout.Flex)
	fx.Layout(layout.Unbounded())
	if got := fx.Children[0].Box.Size().W; got != 13 {
		t.Fatalf("dirty rebuild W = %v, want 13", got)
	}
}
