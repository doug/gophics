package widget

import (
	"testing"
)

// navFixture mounts a Navigator with a probe home page and returns the owner
// and its navState.
func navFixture(t *testing.T) (*Owner, *navState) {
	t.Helper()
	o := newOwner()
	o.SetRoot(Navigator{Home: probe{ID: "home"}})
	o.FlushBuilds()
	st := digState[Navigator](o.root)
	if st == nil {
		t.Fatal("navigator state not found")
	}
	return o, st.(*navState)
}

// pumpNav runs frames until the navigator's transition settles.
func pumpNav(t *testing.T, o *Owner, s *navState) {
	t.Helper()
	for range 240 {
		o.TickAll(0.016)
		o.FlushBuilds()
		if s.trans == nil && !s.slide.Running() {
			return
		}
	}
	t.Fatal("navigator transition did not settle within 240 frames")
}

// tickNav advances n frames without requiring the transition to finish.
func tickNav(o *Owner, n int) {
	for range n {
		o.TickAll(0.016)
		o.FlushBuilds()
	}
}

func TestNavigatorPushPopSequence(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}
	if nav.Depth() != 1 {
		t.Fatalf("initial depth = %d, want 1", nav.Depth())
	}

	homeState := stateOf(o.root, "home")
	if homeState == nil {
		t.Fatal("home page not mounted")
	}

	s.push(probe{ID: "detail"})
	if nav.Depth() != 2 {
		t.Fatalf("depth after push = %d, want 2", nav.Depth())
	}
	pumpNav(t, o, s)
	if nav.Depth() != 2 || s.trans != nil {
		t.Fatalf("after push settles: depth=%d trans=%v", nav.Depth(), s.trans)
	}
	if stateOf(o.root, "detail") == nil {
		t.Fatal("pushed page not mounted")
	}
	// Home stays mounted (offstage) under the pushed page: state survives.
	if got := stateOf(o.root, "home"); got != homeState {
		t.Fatal("home page state lost under a pushed page")
	}

	s.pop()
	pumpNav(t, o, s)
	if nav.Depth() != 1 {
		t.Fatalf("depth after pop = %d, want 1", nav.Depth())
	}
	if stateOf(o.root, "detail") != nil {
		t.Fatal("popped page still mounted")
	}
	if got := stateOf(o.root, "home"); got != homeState {
		t.Fatal("home page state lost across push+pop")
	}
}

// Pop on the home page (empty stack, no transition) is a no-op.
func TestNavigatorPopOnRootIsNoop(t *testing.T) {
	o, s := navFixture(t)
	s.pop()
	o.FlushBuilds()
	if s.trans != nil || len(s.stack) != 0 {
		t.Fatalf("pop on root started a transition: trans=%v stack=%d", s.trans, len(s.stack))
	}
	if d := (Nav{s: s}).Depth(); d != 1 {
		t.Fatalf("depth = %d, want 1", d)
	}
}

// M2: a push landing during a pop transition must first settle the pop —
// applying its stack truncation — then push. Before the fix the pop's
// completion never ran (its handler saw the push transition) and the popped
// page was silently retained.
func TestNavigatorPushDuringPop(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}

	pageA, pageB := probe{ID: "A"}, probe{ID: "B"}
	s.push(pageA)
	pumpNav(t, o, s)
	if nav.Depth() != 2 {
		t.Fatalf("setup: depth = %d, want 2", nav.Depth())
	}

	s.pop()
	tickNav(o, 3) // mid-flight
	if s.trans == nil || !s.trans.popping {
		t.Fatal("setup: pop transition not in flight")
	}

	s.push(pageB)
	// The pop settled synchronously: A is gone, B is the sole stack entry.
	if len(s.stack) != 1 {
		t.Fatalf("stack len after push-during-pop = %d, want 1 (pop truncation lost?)", len(s.stack))
	}
	if s.stack[0] != Widget(pageB) {
		t.Fatalf("stack top = %#v, want pageB", s.stack[0])
	}
	if s.trans == nil || s.trans.popping {
		t.Fatal("push transition not active after settling the pop")
	}

	pumpNav(t, o, s)
	if nav.Depth() != 2 {
		t.Fatalf("final depth = %d, want 2", nav.Depth())
	}
	if s.top() != Widget(pageB) {
		t.Fatalf("top = %#v, want pageB", s.top())
	}
	if stateOf(o.root, "A") != nil {
		t.Fatal("popped page A still mounted after push-during-pop")
	}
	if stateOf(o.root, "B") == nil {
		t.Fatal("pushed page B not mounted")
	}
}

// The symmetric guard: a pop landing during a push transition settles the
// push (its page stays on the stack) and then pops it cleanly.
func TestNavigatorPopDuringPush(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}

	s.push(probe{ID: "A"})
	tickNav(o, 3) // mid-flight
	if s.trans == nil || s.trans.popping {
		t.Fatal("setup: push transition not in flight")
	}

	s.pop()
	if s.trans == nil || !s.trans.popping {
		t.Fatal("pop transition not active after settling the push")
	}
	pumpNav(t, o, s)
	if nav.Depth() != 1 {
		t.Fatalf("final depth = %d, want 1", nav.Depth())
	}
	if stateOf(o.root, "A") != nil {
		t.Fatal("page A still mounted after pop-during-push")
	}
	if stateOf(o.root, "home") == nil {
		t.Fatal("home page missing")
	}
}

// A second pop during a pop settles the first (one page off) and pops the
// next: two Pop calls remove two pages.
func TestNavigatorPopDuringPop(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}

	s.push(probe{ID: "A"})
	pumpNav(t, o, s)
	s.push(probe{ID: "B"})
	pumpNav(t, o, s)
	if nav.Depth() != 3 {
		t.Fatalf("setup: depth = %d, want 3", nav.Depth())
	}

	s.pop()
	tickNav(o, 3)
	s.pop()
	pumpNav(t, o, s)
	if nav.Depth() != 1 {
		t.Fatalf("final depth = %d, want 1 (two pops must remove two pages)", nav.Depth())
	}
	if stateOf(o.root, "A") != nil || stateOf(o.root, "B") != nil {
		t.Fatal("popped pages still mounted")
	}
}

// A pop during a pop with only one page settles the first pop (emptying the
// stack) and then has nothing to pop — home stays.
func TestNavigatorPopDuringLastPop(t *testing.T) {
	o, s := navFixture(t)
	s.push(probe{ID: "A"})
	pumpNav(t, o, s)

	s.pop()
	tickNav(o, 2)
	s.pop() // settles: stack empties; nothing left to pop
	if len(s.stack) != 0 || s.trans != nil {
		t.Fatalf("stack=%d trans=%v, want empty and no transition", len(s.stack), s.trans)
	}
	pumpNav(t, o, s)
	if d := (Nav{s: s}).Depth(); d != 1 {
		t.Fatalf("depth = %d, want 1", d)
	}
	if stateOf(o.root, "home") == nil {
		t.Fatal("home page missing")
	}
}

// TestNavigatorPushDuringPushIsIgnored covers the double-tap: two taps on a list
// row inside the push transition must open one route, not two. Every other
// interleaving settles and proceeds (see the tests above); this one is the
// accidental repeat of a single action, and dropping it is what the platforms do.
func TestNavigatorPushDuringPushIsIgnored(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}

	page := probe{ID: "A"}
	s.push(page)
	tickNav(o, 3) // mid-flight
	if s.trans == nil || s.trans.popping {
		t.Fatal("setup: push transition not in flight")
	}

	s.push(page) // the second tap
	pumpNav(t, o, s)

	if got := nav.Depth(); got != 2 {
		t.Errorf("depth after a double-tap push = %d, want 2 (home + one route)", got)
	}
}

// TestNavigatorPushAfterSettlingStillWorks: the guard must only cover the
// in-flight window, or navigation would stop working after the first push.
func TestNavigatorPushAfterSettlingStillWorks(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}

	s.push(probe{ID: "A"})
	pumpNav(t, o, s)
	s.push(probe{ID: "B"})
	pumpNav(t, o, s)

	if got := nav.Depth(); got != 3 {
		t.Errorf("depth after two settled pushes = %d, want 3", got)
	}
}

// Replace slides C in like a push and, once settled, drops B — the page it
// covered — while A below keeps its state and C keeps the element it was
// built into mid-flight. The last point is why pages carry stable keys: with
// index keys, removing B shifts C's index and remounts it at the finish.
func TestNavigatorReplaceMidStack(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}

	s.push(probe{ID: "A"})
	pumpNav(t, o, s)
	s.push(probe{ID: "B"})
	pumpNav(t, o, s)
	aState := stateOf(o.root, "A")
	if aState == nil || stateOf(o.root, "B") == nil {
		t.Fatal("setup: A and B not mounted")
	}

	nav.Replace(probe{ID: "C"})
	if s.trans == nil || s.trans.popping || !s.trans.replace {
		t.Fatalf("replace transition = %+v, want a replacing push", s.trans)
	}
	tickNav(o, 3) // mid-flight: both B and C are mounted and visible
	cState := stateOf(o.root, "C")
	if cState == nil || stateOf(o.root, "B") == nil {
		t.Fatal("mid-flight: B (leaving) and C (arriving) must both be mounted")
	}

	pumpNav(t, o, s)
	if nav.Depth() != 3 {
		t.Fatalf("depth after replace = %d, want 3 (home, A, C)", nav.Depth())
	}
	if s.stack[0] != Widget(probe{ID: "A"}) || s.stack[1] != Widget(probe{ID: "C"}) {
		t.Fatalf("stack = %#v, want [A C]", s.stack)
	}
	if stateOf(o.root, "B") != nil {
		t.Fatal("replaced page B still mounted after the transition")
	}
	if got := stateOf(o.root, "A"); got != aState {
		t.Fatal("page A below the replacement lost its state")
	}
	if got := stateOf(o.root, "C"); got != cState {
		t.Fatal("replacement page C was remounted when B was dropped (key shifted)")
	}
}

// Replace on the home page has nothing to drop: it is a push.
func TestNavigatorReplaceOnHomePushes(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}
	homeState := stateOf(o.root, "home")

	nav.Replace(probe{ID: "A"})
	if s.trans == nil || s.trans.replace {
		t.Fatalf("replace on home = %+v, want a plain push transition", s.trans)
	}
	pumpNav(t, o, s)
	if nav.Depth() != 2 {
		t.Fatalf("depth = %d, want 2", nav.Depth())
	}
	if stateOf(o.root, "home") != homeState {
		t.Fatal("home lost its state under a replace-as-push")
	}
}

// A Replace landing during a pop settles the pop first (its page is gone,
// synchronously), then replaces the page the pop exposed.
func TestNavigatorReplaceDuringPopSettlesFirst(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}

	s.push(probe{ID: "A"})
	pumpNav(t, o, s)
	s.push(probe{ID: "B"})
	pumpNav(t, o, s)
	s.pop()
	tickNav(o, 3)
	if s.trans == nil || !s.trans.popping {
		t.Fatal("setup: pop not in flight")
	}

	nav.Replace(probe{ID: "C"})
	if len(s.stack) != 2 || s.stack[0] != Widget(probe{ID: "A"}) || s.stack[1] != Widget(probe{ID: "C"}) {
		t.Fatalf("stack right after replace-during-pop = %#v, want [A C] (pop settled, C pushed)", s.stack)
	}
	if s.trans == nil || !s.trans.replace {
		t.Fatalf("transition = %+v, want a replace", s.trans)
	}
	pumpNav(t, o, s)
	if len(s.stack) != 1 || s.stack[0] != Widget(probe{ID: "C"}) {
		t.Fatalf("stack after settle = %#v, want [C]", s.stack)
	}
	if stateOf(o.root, "A") != nil || stateOf(o.root, "B") != nil {
		t.Fatal("A or B still mounted after replace-during-pop")
	}
	if stateOf(o.root, "C") == nil {
		t.Fatal("C not mounted")
	}
}

// Replace shares Push's double-tap guard: one landing during a push (or
// another replace) is dropped.
func TestNavigatorReplaceDuringPushIsIgnored(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}

	s.push(probe{ID: "A"})
	tickNav(o, 3)
	nav.Replace(probe{ID: "C"})
	pumpNav(t, o, s)
	if nav.Depth() != 2 || s.top() != Widget(probe{ID: "A"}) {
		t.Fatalf("after replace-during-push: depth=%d top=%#v, want 2 and A", nav.Depth(), s.top())
	}

	nav.Replace(probe{ID: "D"})
	tickNav(o, 3)
	nav.Replace(probe{ID: "E"})
	pumpNav(t, o, s)
	if nav.Depth() != 2 || s.top() != Widget(probe{ID: "D"}) {
		t.Fatalf("after replace-during-replace: depth=%d top=%#v, want 2 and D", nav.Depth(), s.top())
	}
}

// PopToRoot drops the offstage middle pages at once and pops the top page
// over Home with a single transition; Home keeps its state throughout.
func TestNavigatorPopToRoot(t *testing.T) {
	o, s := navFixture(t)
	nav := Nav{s: s}
	homeState := stateOf(o.root, "home")

	for _, id := range []string{"A", "B", "C"} {
		s.push(probe{ID: id})
		pumpNav(t, o, s)
	}
	if nav.Depth() != 4 {
		t.Fatalf("setup: depth = %d, want 4", nav.Depth())
	}

	nav.PopToRoot()
	if s.trans == nil || !s.trans.popping || s.trans.under != Widget(probe{ID: "home"}) {
		t.Fatalf("transition = %+v, want a pop onto home", s.trans)
	}
	if nav.Depth() != 2 || s.top() != Widget(probe{ID: "C"}) {
		t.Fatalf("mid-flight depth=%d top=%#v, want 2 and C", nav.Depth(), s.top())
	}
	tickNav(o, 3)
	if stateOf(o.root, "A") != nil || stateOf(o.root, "B") != nil {
		t.Fatal("middle pages still mounted during PopToRoot")
	}
	if stateOf(o.root, "C") == nil {
		t.Fatal("top page unmounted before its pop finished")
	}

	pumpNav(t, o, s)
	if nav.Depth() != 1 {
		t.Fatalf("depth after PopToRoot = %d, want 1", nav.Depth())
	}
	if stateOf(o.root, "C") != nil {
		t.Fatal("top page still mounted after PopToRoot")
	}
	if stateOf(o.root, "home") != homeState {
		t.Fatal("home lost its state across PopToRoot")
	}

	nav.PopToRoot() // on home: nothing to do
	o.FlushBuilds()
	if s.trans != nil {
		t.Fatal("PopToRoot on home started a transition")
	}
}

// The hot-restart snapshot still round-trips through a stack shaped by
// Replace, and the restored pages get keys like any others.
func TestNavigatorReplaceSnapshotRoundTrips(t *testing.T) {
	RegisterSnapshotType[storyPage]()
	o := newOwner()
	o.SetRoot(Navigator{Home: Sized{W: 1, H: 1}})
	o.FlushBuilds()
	s := digState[Navigator](o.root).(*navState)
	nav := Nav{s: s}

	nav.Push(storyPage{ID: 1})
	pumpNav(t, o, s)
	nav.Push(storyPage{ID: 2})
	pumpNav(t, o, s)
	nav.Replace(storyPage{ID: 3})
	pumpNav(t, o, s)

	snap := o.SnapshotState()
	o2 := newOwner()
	o2.SetRoot(Navigator{Home: Sized{W: 1, H: 1}})
	o2.FlushBuilds()
	o2.RestoreState(snap)
	s2 := digState[Navigator](o2.root).(*navState)
	if len(s2.stack) != 2 {
		t.Fatalf("restored stack len = %d, want 2", len(s2.stack))
	}
	if s2.stack[0] != Widget(storyPage{ID: 1}) || s2.stack[1] != Widget(storyPage{ID: 3}) {
		t.Fatalf("restored stack = %#v, want [1 3]", s2.stack)
	}
	if len(s2.keys) != 2 || s2.keys[0] == s2.keys[1] {
		t.Fatalf("restored keys = %v, want two distinct keys", s2.keys)
	}
}
