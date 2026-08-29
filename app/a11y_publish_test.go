package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
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
		Gestures: widget.Gestures{OnTap: func() { *tapped++ }},
		Child:    widget.Text{S: *a.label},
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

// atWindow is a Window whose only capability is accessibility, used to install
// the fake through the real wiring.
//
// The tests used to write core.Owner.Accessibility directly. That field is
// unexported now, and the rewrite is an improvement rather than a workaround:
// going through WireCapabilities exercises the type-assert and the posted
// wrapper too, so what is under test is the path the runtime actually takes.
type atWindow struct {
	shell.Window
	at shell.Accessibility
}

func (w atWindow) Accessibility() shell.Accessibility { return w.at }

// installAT wires at into h through the same path a real shell uses.
func installAT(h *Headless, at shell.Accessibility) {
	h.core.Owner.WireCapabilities(atWindow{at: at})
}

func TestPublishA11yOnlyOnChange(t *testing.T) {
	h, _, _ := newLabelApp(t)

	at := &fakeAT{}
	installAT(h, at)
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
	installAT(h, at)
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
	installAT(h, at)
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
	// Posted, not inline. The activate callback arrives from whatever thread the
	// platform's assistive technology runs on, so the wrapper routes it through
	// Owner.Post and it lands on the next drain — which is the whole point of
	// the wrapper, and something the old test could not see because it wrote the
	// capability field directly and bypassed it.
	if *tapped != 0 {
		t.Errorf("activate ran inline (tapped = %d); it must be posted to the UI goroutine", *tapped)
	}
	h.core.drainPosted()
	if *tapped != 1 {
		t.Errorf("tapped = %d after draining, want 1", *tapped)
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

// bgApp paints nothing, so whatever the runner clears with is what shows.
type bgApp struct{}

func (bgApp) Build(widget.Ctx) widget.Widget { return widget.Sized{} }

// The background has to follow the platform's colour scheme. It used to be a
// single colour resolved once at startup while the theme a widget reads is
// chosen per frame from the same signal — so a light background stayed put
// under dark-theme text, and nothing errored.
func TestBackgroundFollowsColorScheme(t *testing.T) {
	light := paint.RGB(1, 1, 1)
	dark := paint.RGB(0, 0, 0)
	h, err := NewHeadless(bgApp{}, Config{
		Size: geom.Size{W: 40, H: 40}, Font: goregular.TTF,
		Background: light, BackgroundDark: dark,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}

	at := func() (r, g, b uint32) {
		img := h.Render()
		c := img.At(20, 20)
		r, g, b, _ = c.RGBA()
		return
	}
	if r, _, _ := at(); r < 0xf000 {
		t.Errorf("light scheme: background red = %#x, want near white", r)
	}
	h.SetDarkMode(true)
	if r, _, _ := at(); r > 0x1000 {
		t.Errorf("dark scheme: background red = %#x, want near black", r)
	}
	h.SetDarkMode(false)
	if r, _, _ := at(); r < 0xf000 {
		t.Errorf("back to light: background red = %#x, want near white", r)
	}
}

// With no dark variant given, one colour is used in both — the old behaviour,
// so this is not a breaking change for apps that paint their own background.
func TestBackgroundWithoutDarkVariant(t *testing.T) {
	h, err := NewHeadless(bgApp{}, Config{
		Size: geom.Size{W: 40, H: 40}, Font: goregular.TTF,
		Background: paint.RGB(1, 1, 1),
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.SetDarkMode(true)
	r, _, _, _ := h.Render().At(20, 20).RGBA()
	if r < 0xf000 {
		t.Errorf("background red = %#x, want the single colour unchanged", r)
	}
}
