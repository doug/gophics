package widget

import (
	"context"
	"sync"
	"testing"
	"time"
)

// postQueue stands in for the app runner's posted-work channel: Post is called
// from background goroutines, drain runs the work on the test's goroutine.
type postQueue struct {
	mu  sync.Mutex
	fns []func()
}

func (q *postQueue) post(fn func()) {
	q.mu.Lock()
	q.fns = append(q.fns, fn)
	q.mu.Unlock()
}

func (q *postQueue) drain() int {
	q.mu.Lock()
	fns := q.fns
	q.fns = nil
	q.mu.Unlock()
	for _, fn := range fns {
		fn()
	}
	return len(fns)
}

// A widget's context is live while it is in the tree and cancelled once it
// leaves, which is the whole point: work started for a widget stops when the
// widget does.
func TestContextCancelledOnUnmount(t *testing.T) {
	o := newOwner()
	o.SetRoot(probe{ID: "a"})
	s := stateOf(o.root, "a")
	if s == nil {
		t.Fatal("probe did not mount")
	}

	ctx := s.el.ctx().Lifetime()
	if err := ctx.Err(); err != nil {
		t.Fatalf("a mounted widget's context is already cancelled: %v", err)
	}

	o.SetRoot(Sized{W: 1, H: 1}) // replaces the probe, unmounting it
	if s.disposes != 1 {
		t.Fatalf("probe was not unmounted (disposes=%d); the rest of this test "+
			"would pass without checking anything", s.disposes)
	}
	if err := ctx.Err(); err == nil {
		t.Error("the widget left the tree and its context is still live — work " +
			"started on its behalf would run on against a tree it cannot touch")
	} else if err != context.Canceled {
		t.Errorf("ctx.Err() = %v, want context.Canceled", err)
	}
}

// Removing an ancestor must stop work started anywhere below it, so that a
// fetch begun deep inside a dismissed page does not outlive the page.
//
// Note this passes on the recursive unmount walk alone — it does not exercise
// parent derivation, which TestChildContextDerivesFromParent covers.
func TestContextCancelledWhenAncestorRemoved(t *testing.T) {
	o := newOwner()
	o.SetRoot(Column(probe{ID: "child"}))
	s := stateOf(o.root, "child")
	if s == nil {
		t.Fatal("child probe did not mount")
	}
	ctx := s.el.ctx().Lifetime()
	if err := ctx.Err(); err != nil {
		t.Fatalf("child context already cancelled while mounted: %v", err)
	}

	// The child is never unmounted by name; its ancestor is replaced.
	o.SetRoot(Sized{W: 1, H: 1})
	if err := ctx.Err(); err == nil {
		t.Error("removing an ancestor left a descendant's context live — a fetch " +
			"started deep in a removed page would keep running")
	}
}

// A caller that asks for a context after the widget is gone must not be handed
// a live one: it would start work with nothing left to cancel it.
func TestContextAfterUnmountIsAlreadyCancelled(t *testing.T) {
	o := newOwner()
	o.SetRoot(probe{ID: "a"})
	s := stateOf(o.root, "a")
	el := s.el
	o.SetRoot(Sized{W: 1, H: 1})

	// First call on this element happens only now, after it left the tree.
	if err := el.ctx().Lifetime().Err(); err == nil {
		t.Error("Context() on an unmounted element returned a live context")
	}
}

// PostState is the one safe way for a background goroutine to reach widget
// state: it must defer to the UI goroutine rather than mutating in place.
func TestPostStateDefersToTheRunner(t *testing.T) {
	o := newOwner()
	q := &postQueue{}
	o.Post = q.post
	o.SetRoot(probe{ID: "a"})
	s := stateOf(o.root, "a")

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.PostState(func() { s.local = 42 })
	}()
	<-done

	if s.local == 42 {
		t.Fatal("PostState ran fn on the calling goroutine — that is the data " +
			"race the whole method exists to prevent")
	}
	if n := q.drain(); n != 1 {
		t.Fatalf("drained %d posted funcs, want 1", n)
	}
	if s.local != 42 {
		t.Errorf("after draining, local = %d, want 42 — the work never arrived", s.local)
	}
	if !s.el.dirty {
		t.Error("PostState delivered the value but did not mark the subtree for " +
			"rebuild, so the screen would keep showing the old one")
	}
}

// Work in flight when the widget goes away must be dropped, not applied to a
// state whose element is no longer in the tree.
func TestPostStateDroppedAfterUnmount(t *testing.T) {
	o := newOwner()
	q := &postQueue{}
	o.Post = q.post
	o.SetRoot(probe{ID: "a"})
	s := stateOf(o.root, "a")

	s.PostState(func() { s.local = 99 }) // in flight…
	o.SetRoot(Sized{W: 1, H: 1})         // …widget leaves the tree…
	q.drain()                            // …and only now does the work land

	if s.local == 99 {
		t.Error("a result arrived after the widget was gone and was applied anyway")
	}
}

// Headless and test embeddings run without a runner. The documented fallback is
// to apply inline rather than silently drop the update.
func TestPostStateInlineWithoutRunner(t *testing.T) {
	o := newOwner() // Post is nil
	o.SetRoot(probe{ID: "a"})
	s := stateOf(o.root, "a")

	s.PostState(func() { s.local = 7 })
	if s.local != 7 {
		t.Errorf("with no Owner.Post the update was dropped (local = %d, want 7)", s.local)
	}
}

// A state that never mounted has no tree to notify; PostState must be a no-op
// rather than a nil dereference.
func TestPostStateOnUnmountedStateIsSafe(t *testing.T) {
	s := &probeState{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.PostState(func() { s.local = 1 })
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("PostState blocked on a state that was never mounted")
	}
}

// A child's context must derive from its parent's rather than from Background:
// a child's lifetime is genuinely bounded by its parent's, and deriving makes
// that true independently of the unmount walk reaching every descendant.
//
// Cancelling the ancestor directly is what separates this from the test above,
// which the recursive walk satisfies on its own.
func TestChildContextDerivesFromParent(t *testing.T) {
	o := newOwner()
	o.SetRoot(Column(probe{ID: "child"}))
	s := stateOf(o.root, "child")
	if s == nil {
		t.Fatal("child probe did not mount")
	}
	child := s.el.ctx().Lifetime()
	if child.Err() != nil {
		t.Fatal("child context cancelled while mounted")
	}
	if o.root == s.el {
		t.Fatal("the probe is the root, so there is no ancestor to cancel and " +
			"this test would prove nothing")
	}

	// Cancel the ancestor alone. Nothing is unmounted, so only derivation can
	// carry the cancellation down.
	o.root.lifetime()
	o.root.lifeCancel()

	if child.Err() == nil {
		t.Error("cancelling an ancestor left the child's context live — the " +
			"child derives from Background instead of its parent")
	}
}
