package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// fakeAT stands in for a platform screen-reader bridge.
type fakeAT struct {
	trees     [][]shell.A11yNode
	activate  func(id int)
	announced []string
}

func (f *fakeAT) Announce(msg string, assertive bool) { f.announced = append(f.announced, msg) }

func (f *fakeAT) SetTree(nodes []shell.A11yNode, activate func(id int)) {
	f.trees = append(f.trees, nodes)
	f.activate = activate
}

// labelApp renders one tappable node. The label is read through a pointer so
// a test can change it and rebuild.
type labelApp struct {
	label  *string
	tapped *int
}

func (a labelApp) Build(widget.Ctx) widget.Widget {
	tapped := a.tapped
	return widget.Padding{All: 8, Child: widget.Interactive{
		Handler: widget.Handler{OnTap: func() { *tapped++ }},
		Child:   widget.Text{S: *a.label},
	}}
}

// newLabelApp wires a headless app around a mutable label.
func newLabelApp(t *testing.T) (*Headless, *string, *int) {
	t.Helper()
	label, tapped := "Send", 0
	h, err := NewHeadless(labelApp{label: &label, tapped: &tapped}, Config{
		Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, &label, &tapped
}

func TestPublishA11yOnlyOnChange(t *testing.T) {
	h, _, _ := newLabelApp(t)

	at := &fakeAT{}
	h.core.Owner.Accessibility = at
	sh := &shellHandler{core: h.core}

	sh.publishA11y()
	if len(at.trees) != 1 {
		t.Fatalf("first publish: %d trees, want 1", len(at.trees))
	}
	if len(at.trees[0]) < 2 {
		t.Fatalf("published %d nodes, want the root plus content", len(at.trees[0]))
	}

	// The same frame republished must not reach the bridge again: a screen
	// reader rebuilding its node tree per frame is the bug this guards.
	sh.publishA11y()
	sh.publishA11y()
	if len(at.trees) != 1 {
		t.Fatalf("unchanged tree published %d times, want 1", len(at.trees))
	}
}

func TestPublishA11yRepublishesWhenLabelChanges(t *testing.T) {
	h, label, _ := newLabelApp(t)

	at := &fakeAT{}
	h.core.Owner.Accessibility = at
	sh := &shellHandler{core: h.core}
	sh.publishA11y()

	*label = "Sending…"
	h.Owner().RebuildAll()
	h.Render()
	sh.publishA11y()

	if len(at.trees) != 2 {
		t.Fatalf("changed tree published %d times, want 2", len(at.trees))
	}
	if !hasLabel(at.trees[1], "Sending…") {
		t.Errorf("republished tree has no updated label: %v", labels(at.trees[1]))
	}
}

// The bridge is handed an activation callback, and it must reach the widget.
func TestPublishA11yActivateRoutesToWidget(t *testing.T) {
	h, _, tapped := newLabelApp(t)

	at := &fakeAT{}
	h.core.Owner.Accessibility = at
	sh := &shellHandler{core: h.core}
	sh.publishA11y()

	if at.activate == nil {
		t.Fatal("bridge got no activate callback")
	}
	var btn = -1
	for _, n := range at.trees[0] {
		if n.Tappable {
			btn = n.ID
		}
	}
	if btn < 0 {
		t.Fatal("no tappable node published")
	}
	at.activate(btn)
	if *tapped != 1 {
		t.Errorf("tapped = %d, want 1", *tapped)
	}
}

// A window with no accessibility bridge must not panic or do work.
func TestPublishA11yWithoutBridge(t *testing.T) {
	h, _, _ := newLabelApp(t)
	sh := &shellHandler{core: h.core}
	sh.publishA11y()
	if sh.lastA11y != nil {
		t.Error("tree cached with no bridge attached")
	}
}

func hasLabel(nodes []shell.A11yNode, want string) bool {
	for _, n := range nodes {
		if n.Label == want || n.Value == want {
			return true
		}
	}
	return false
}

func labels(nodes []shell.A11yNode) []string {
	var out []string
	for _, n := range nodes {
		if n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}
