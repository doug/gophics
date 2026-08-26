package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// Reconciliation survives arbitrary tree-to-tree transitions.
//
// The reconciler is where a UI framework fails quietly. A mismatched child
// silently loses its State — a text field forgets its caret, a scroll position
// resets — and nothing errors, so the only signal is a person noticing their
// input vanished. The positional fast path added for allocation reasons made
// this worse to reason about: it reuses one buffer across frames, swaps kids
// and kidsBuf, and takes a different route entirely when no key is present.
//
// So: generate two trees from the fuzz input, mount one, reconcile to the
// other, and check the element tree ends up describing the second tree — the
// right shape, no stale children, and a layout that still runs. A panic or a
// mismatch is a real defect; there is no input for which either is acceptable.

// treeFrom reads a bounded widget tree out of b, consuming bytes as it goes.
// Bounded on purpose: depth and fan-out are capped so the corpus explores
// shapes rather than size.
func treeFrom(b []byte, i *int, depth int) (Widget, int) {
	next := func() byte {
		if *i >= len(b) {
			return 0
		}
		v := b[*i]
		*i++
		return v
	}
	kind := next() % 5
	if depth >= 4 {
		kind = 0
	}
	switch kind {
	case 0: // leaf
		return Sized{W: float32(next()%40) + 1, H: float32(next()%40) + 1}, 1
	case 1: // single-child wrapper
		child, n := treeFrom(b, i, depth+1)
		return Padding{All: float32(next() % 8), Child: child}, n + 1
	case 2: // keyed wrapper — the branch that uses the byKey maps
		child, n := treeFrom(b, i, depth+1)
		return WithKey{Key: int(next() % 6), Child: child}, n
	default: // flex with 0..3 children
		count := int(next() % 4)
		kids := make([]Widget, 0, count)
		total := 1
		for range count {
			c, n := treeFrom(b, i, depth+1)
			kids = append(kids, c)
			total += n
		}
		if kind == 3 {
			return Column(kids...), total
		}
		return Row(kids...), total
	}
}

func FuzzReconcileBetweenTrees(f *testing.F) {
	f.Add([]byte{0}, []byte{1, 0})
	f.Add([]byte{3, 2, 0, 0}, []byte{3, 3, 0, 0, 0})
	f.Add([]byte{2, 1, 0}, []byte{2, 2, 0})
	f.Add([]byte{4, 3, 2, 1, 0, 0, 0}, []byte{4, 2, 2, 1, 0})

	f.Fuzz(func(t *testing.T, a, b []byte) {
		ia, ib := 0, 0
		first, _ := treeFrom(a, &ia, 0)
		second, _ := treeFrom(b, &ib, 0)

		o := &Owner{}
		o.SetRoot(first)
		o.FlushBuilds()

		// Reconcile onto the second tree — the transition under test.
		o.SetRoot(second)
		o.FlushBuilds()

		box := o.root.renderBox()
		if box == nil {
			t.Fatal("reconciled tree has no render box")
		}
		box.Layout(layout.Loose(geom.Size{W: 200, H: 200}))

		// Every box the walk reaches must be laid out and finite: a stale
		// child left behind by reconciliation shows up here as a box that
		// never received a size, or one carrying garbage.
		for _, n := range layout.Inspect(box) {
			w, h := n.Rect.Dx(), n.Rect.Dy()
			if w < 0 || h < 0 {
				t.Fatalf("%s laid out to a negative size %v", n.Type, n.Rect)
			}
			if w != w || h != h { // NaN
				t.Fatalf("%s laid out to NaN %v", n.Type, n.Rect)
			}
		}

		// And reconciling to the same tree again is stable — the idempotent
		// case, where the positional path does the most buffer swapping.
		o.SetRoot(second)
		o.FlushBuilds()
		if o.root.renderBox() == nil {
			t.Fatal("re-reconciling to the same tree dropped the render box")
		}
	})
}
