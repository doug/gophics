//go:build linux

package platform

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// listenScript registers for the events a screen reader cares about and prints
// each one until it is stopped. e.type carries the full name, so a checkbox
// being ticked arrives as "object:state-changed:checked".
const listenScript = `
import sys, threading, pyatspi
def cb(e):
    src = ""
    try:
        src = e.source.name or ""
    except Exception:
        pass
    data = e.any_data if isinstance(e.any_data, str) else ""
    print("EVENT|%s|%s|%s|%s" % (e.type, e.detail1, src, data))
    sys.stdout.flush()
pyatspi.Registry.registerEventListener(cb,
    "object:state-changed", "object:property-change",
    "object:children-changed", "object:announcement")
threading.Timer(6.0, pyatspi.Registry.stop).start()
pyatspi.Registry.start()
`

// TestATSPIEmitsChangeEvents checks the half that serving a tree on demand
// cannot cover.
//
// A screen reader reads a node once and then waits to be told when it changes.
// Without events it will keep announcing a checkbox as unchecked long after the
// user ticked it, and will never notice a node appear — the tree would be
// correct on every query and wrong in practice. So this drives real changes and
// asserts they arrive at a real client.
func TestATSPIEmitsChangeEvents(t *testing.T) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		t.Skip("no session bus")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 unavailable")
	}
	if addr, _ := a11yBusAddress(); addr == "" {
		t.Skip("no accessibility bus")
	}

	b := newATSPIBridge("gophics-atspi-events")
	if b == nil {
		t.Skip("could not reach the accessibility bus")
	}
	defer b.Close()

	base := []A11yNode{
		{ID: 1, ParentID: -1, Role: "group", Label: "Root", W: 400, H: 300},
		{ID: 2, ParentID: 1, Role: "button", Label: "Send", W: 80, H: 30, Tappable: true},
		{ID: 3, ParentID: 1, Role: "checkbox", Label: "Agree", W: 120, H: 24, Checkable: true},
	}
	b.SetTree(base, func(int) {})
	time.Sleep(500 * time.Millisecond)

	listener := exec.Command("python3", "-c", listenScript)
	out, err := listener.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := listener.Start(); err != nil {
		t.Fatal(err)
	}
	// Give the listener time to register its match rules; events emitted
	// before that are genuinely lost, which is the client's problem to solve
	// and not something to paper over here.
	time.Sleep(1500 * time.Millisecond)

	// Tick the checkbox, rename the button, and add a node.
	changed := []A11yNode{
		{ID: 1, ParentID: -1, Role: "group", Label: "Root", W: 400, H: 300},
		{ID: 2, ParentID: 1, Role: "button", Label: "Sending…", W: 80, H: 30, Tappable: true},
		{ID: 3, ParentID: 1, Role: "checkbox", Label: "Agree", W: 120, H: 24,
			Checkable: true, Checked: true},
		{ID: 4, ParentID: 1, Role: "text", Label: "Sent", W: 100, H: 20},
	}
	b.SetTree(changed, func(int) {})
	b.Announce("5 results", true)

	// Drop a node, to exercise the removal path.
	b.SetTree(changed[:3], func(int) {})

	buf := make([]byte, 64*1024)
	n, _ := out.Read(buf)
	text := string(buf[:n])
	// Keep reading until the listener stops on its own timer.
	for {
		m, err := out.Read(buf)
		if m > 0 {
			text += string(buf[:m])
		}
		if err != nil {
			break
		}
	}
	listener.Wait()
	t.Logf("events seen by pyatspi:\n%s", text)

	for _, want := range []struct{ desc, sub string }{
		{"checkbox ticked", "object:state-changed:checked|1|"},
		{"button renamed", "object:property-change:accessible-name"},
		{"node added", "object:children-changed:add"},
		{"node removed", "object:children-changed:remove"},
		{"announcement", "object:announcement"},
	} {
		if !strings.Contains(text, want.sub) {
			t.Errorf("no event for %s (looking for %q) in:\n%s", want.desc, want.sub, text)
		}
	}
	// The message has to survive, not just the event.
	if !strings.Contains(text, "5 results") {
		t.Errorf("announcement arrived without its text:\n%s", text)
	}
}

// An unchanged republish must stay silent. gophics rebuilds and republishes
// whenever the widget tree changes, which for an animating UI is every frame;
// broadcasting an identical tree at 60Hz would drown the bus and make a screen
// reader unusable, which is the opposite of the point.
//
// The diff is the thing under test, so this drives it directly rather than
// counting bus traffic — no accessibility bus required.
func TestATSPINoEventsWhenNothingChanged(t *testing.T) {
	nodes := []A11yNode{
		{ID: 1, ParentID: -1, Role: "group", Label: "Root", W: 100, H: 100},
		{ID: 2, ParentID: 1, Role: "button", Label: "Go", W: 20, H: 10, Tappable: true},
	}
	prev := buildTree(nodes)
	next := buildTree(append([]A11yNode(nil), nodes...))
	if got := diffCount(prev, next); got != 0 {
		t.Errorf("an identical republish produced %d events, want 0", got)
	}
}

// Bounds moving is not an event. A node that only slides — a scrolling list,
// an animation — must not emit, or a screen reader is told something changed
// every frame while nothing it reads has.
func TestATSPIMovementIsNotAnEvent(t *testing.T) {
	before := buildTree([]A11yNode{{ID: 1, ParentID: -1, Role: "button", Label: "Go", X: 0, Y: 0, W: 20, H: 10}})
	after := buildTree([]A11yNode{{ID: 1, ParentID: -1, Role: "button", Label: "Go", X: 0, Y: 40, W: 20, H: 10}})
	if got := diffCount(before, after); got != 0 {
		t.Errorf("a node moving produced %d events, want 0", got)
	}
}

// Each tracked state change is one event, and the ones that move together
// (disabled clears both ENABLED and SENSITIVE) are reported as both, because a
// reader may be tracking either.
func TestATSPIStateChangesCounted(t *testing.T) {
	on := buildTree([]A11yNode{{ID: 1, ParentID: -1, Role: "checkbox", Checkable: true}})
	ticked := buildTree([]A11yNode{{ID: 1, ParentID: -1, Role: "checkbox", Checkable: true, Checked: true}})
	if got := diffCount(on, ticked); got != 1 {
		t.Errorf("ticking a checkbox produced %d events, want 1", got)
	}
	disabled := buildTree([]A11yNode{{ID: 1, ParentID: -1, Role: "checkbox", Checkable: true, Disabled: true}})
	if got := diffCount(on, disabled); got != 2 {
		t.Errorf("disabling produced %d events, want 2 (enabled and sensitive)", got)
	}
}

// diffCount reports how many events emitDiff would send, without sending them.
func diffCount(old, cur *a11yTree) int {
	n := 0
	for _, node := range cur.nodes {
		prev, existed := old.byID[node.ID]
		if !existed {
			n++
			continue
		}
		if prev.Label != node.Label || prev.Value != node.Value {
			n++
		}
		for _, st := range trackedStates {
			if st.of(prev) != st.of(node) {
				n++
			}
		}
	}
	for _, node := range old.nodes {
		if _, still := cur.byID[node.ID]; !still {
			n++
		}
	}
	return n
}
