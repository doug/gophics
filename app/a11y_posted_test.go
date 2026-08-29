package app

import (
	"testing"

	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// fakeA11yWindow is a shell.Window that also offers an accessibility bridge,
// which is what makes wireCapabilities pick the bridge up.
type fakeA11yWindow struct{ at shell.Accessibility }

func (fakeA11yWindow) Invalidate()                          {}
func (fakeA11yWindow) SetTitle(string)                      {}
func (fakeA11yWindow) Close()                               {}
func (fakeA11yWindow) ClipboardRead() (string, error)       { return "", nil }
func (fakeA11yWindow) ClipboardWrite(string) error          { return nil }
func (fakeA11yWindow) OpenURL(string) error                 { return nil }
func (fakeA11yWindow) DarkMode() bool                       { return false }
func (w fakeA11yWindow) Accessibility() shell.Accessibility { return w.at }

// TestA11yActivateIsPosted pins the contract that an assistive technology's
// activation reaches widget state on the UI goroutine, not on whichever thread
// the platform bridge happened to call from.
//
// This is worth a test of its own because the wrapper is invisible at the call
// site: it is applied in generated code, and the older a11y tests assign
// Owner.Accessibility directly, so they never see it. Before Accessibility
// gained SetTree it had no callback parameter and so got no wrapper at all —
// meaning AppKit's accessibilityPerformPress and Android's performAction
// reached widget state straight from their own threads.
func TestA11yActivateIsPosted(t *testing.T) {
	at := &fakeAT{}
	o := &widget.Owner{}

	var queue []func()
	o.Post = func(fn func()) { queue = append(queue, fn) }

	o.WireCapabilities(fakeA11yWindow{at: at})
	if o.Accessibility() == nil {
		t.Fatal("wireCapabilities did not pick up the bridge")
	}

	activated := 0
	o.Accessibility().SetTree(nil, func(id int) { activated = id })
	if at.activate == nil {
		t.Fatal("bridge was handed no activate callback")
	}

	// The platform calls activate. Nothing may happen yet: this stands in for
	// the call arriving on the AppKit or Android accessibility thread.
	at.activate(7)
	if activated != 0 {
		t.Fatal("activate ran inline — it must be deferred to the UI goroutine")
	}
	if len(queue) != 1 {
		t.Fatalf("posted %d funcs, want 1", len(queue))
	}

	queue[0]()
	if activated != 7 {
		t.Errorf("after draining the post queue, activated = %d, want 7", activated)
	}
}

// TestA11yPostedNilSafe covers the degenerate wirings: a window whose bridge is
// nil must not become a non-nil interface holding nil, and an Owner with no
// Post must still get a working bridge rather than a panic on first callback.
func TestA11yPostedNilSafe(t *testing.T) {
	o := &widget.Owner{}
	o.Post = func(fn func()) { fn() }
	o.WireCapabilities(fakeA11yWindow{at: nil})
	if o.Accessibility() != nil {
		t.Error("a nil bridge produced a non-nil Accessibility")
	}

	// No Post: callbacks fire inline, which is the documented fallback.
	at := &fakeAT{}
	o2 := &widget.Owner{}
	o2.WireCapabilities(fakeA11yWindow{at: at})
	if o2.Accessibility() == nil {
		t.Fatal("bridge dropped when Post was nil")
	}
	got := 0
	o2.Accessibility().SetTree(nil, func(id int) { got = id })
	at.activate(3)
	if got != 3 {
		t.Errorf("with no Post, activate should run inline; got = %d, want 3", got)
	}
}
