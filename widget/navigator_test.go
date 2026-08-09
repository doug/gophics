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
	for i := 0; i < 240; i++ {
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
	for i := 0; i < n; i++ {
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
