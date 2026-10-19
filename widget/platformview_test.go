package widget_test

import (
	"errors"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// widget.PlatformView against a fake shell, which is the only way to test it:
// the real implementations hand a rect to an OS compositor, and the thing worth
// checking is not that a WKWebView appears but that the *bookkeeping* is right —
// one view per element, bounds that follow the layout, hidden rather than closed
// when scrolled away, and closed exactly once on unmount.
//
// That bookkeeping is the whole reason the widget exists. The capability alone
// leaves the handle unowned (shell/platformview.go), and a handle that outlives
// its element leaves a native view on screen over whatever replaced it, with
// nobody holding a reference to close it.

// --- a fake platform-view shell ---------------------------------------------

type fakePV struct {
	kinds []string
	open  []*fakePVHandle
}

type fakePVHandle struct {
	kind, params string
	bounds       geom.Rect
	sent         []string
	closes       int
	h            shell.PlatformViewHandlers
}

func (f *fakePV) Kinds() []string { return f.kinds }

func (f *fakePV) Open(kind, params string, b geom.Rect, h shell.PlatformViewHandlers) shell.PlatformViewHandle {
	for _, k := range f.kinds {
		if k == kind {
			hd := &fakePVHandle{kind: kind, params: params, bounds: b, h: h}
			f.open = append(f.open, hd)
			return hd
		}
	}
	return nil
}

// live returns the handles that have not been closed — what is still on screen.
func (f *fakePV) live() []*fakePVHandle {
	var out []*fakePVHandle
	for _, h := range f.open {
		if h.closes == 0 {
			out = append(out, h)
		}
	}
	return out
}

func (h *fakePVHandle) SetBounds(b geom.Rect) { h.bounds = b }
func (h *fakePVHandle) Send(msg string)       { h.sent = append(h.sent, msg) }
func (h *fakePVHandle) Close()                { h.closes++ }

// pvWindow publishes only this capability, so the fake is installed through the
// real wiring — WireCapabilities exercises the type-assert and the generated
// posted wrapper too, which writing the Owner field would skip.
type pvWindow struct {
	shell.Window
	pv shell.PlatformViews
}

func (w pvWindow) PlatformViews() shell.PlatformViews { return w.pv }

// box is a solid rectangle, used as a placeholder that a golden image would
// show through the hole.
func pvBoxWidget(c paint.Color) widget.Widget {
	return widget.Fill{Color: c}
}

// --- tests -------------------------------------------------------------------

// The capability must actually reach the widget tree as non-nil and produce a
// view at the rect the layout gave it. A capability that is nil everywhere is
// not an implementation, so this is the test that says it is one.
func TestPlatformViewOpensAtItsLayoutRect(t *testing.T) {
	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	var ready int
	root := widget.Padding{All: 20, Child: widget.Column(
		widget.PlatformView{
			Kind:   shell.PlatformViewWebView,
			Params: "https://example.com",
			W:      60, H: 40,
			OnReady: func() { ready++ },
		},
		widget.Spacer(),
	)}
	h := headless(t, root, 200, 120)
	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	h.Render()

	if len(pv.open) != 1 {
		t.Fatalf("opened %d views, want 1 — the capability did not reach the widget", len(pv.open))
	}
	got := pv.open[0]
	if got.kind != shell.PlatformViewWebView || got.params != "https://example.com" {
		t.Errorf("opened kind=%q params=%q", got.kind, got.params)
	}
	// 60x40 as asked, centred on the column's cross axis inside the padding.
	if want := geom.RectXYWH(70, 20, 60, 40); got.bounds != want {
		t.Errorf("bounds = %v, want %v", got.bounds, want)
	}

	// OnReady is the shell's to deliver, and arrives after Open returns. The
	// generated posted wrapper marshals it through Owner.Post, so it lands on
	// the next frame rather than inside this call — which is the contract, and
	// the reason a test has to render again to see it.
	got.h.OnReady()
	h.Render()
	if ready != 1 {
		t.Errorf("OnReady fired %d times, want 1", ready)
	}
}

// Nothing in the widget may require a capability. On most builds — every
// desktop one, every mobile host that registers nothing — PlatformViews() is
// nil, and the widget has to lay out and paint its placeholder as if it were
// an ordinary box.
func TestPlatformViewWithoutTheCapabilityIsJustItsPlaceholder(t *testing.T) {
	root := widget.PlatformView{
		Kind:        shell.PlatformViewWebView,
		Params:      "https://example.com",
		Placeholder: pvBoxWidget(paint.RGB(1, 0, 0)),
	}
	h := headless(t, root, 100, 60)
	h.Render() // no WireCapabilities at all: ctx.PlatformViews() is nil
	h.Render()
	// Reaching here without a panic is most of the point; the rest is that it
	// filled the space it was given, like Canvas.
	if img := h.Render(); img == nil {
		t.Fatal("no frame")
	}
}

// A kind the host does not offer must fail once and then stay quiet. Reporting
// it every frame would make an app's error handler a log spammer, and retrying
// forever would hide a permanent answer behind a transient-looking one.
func TestPlatformViewReportsAnUnsupportedKindExactlyOnce(t *testing.T) {
	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	var errs []error
	root := widget.PlatformView{
		Kind:    shell.PlatformViewMap,
		OnError: func(err error) { errs = append(errs, err) },
	}
	h := headless(t, root, 100, 60)
	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	for range 5 {
		h.Render()
	}
	if len(errs) != 1 {
		t.Fatalf("OnError fired %d times, want 1: %v", len(errs), errs)
	}
	if !errors.Is(errs[0], widget.ErrPlatformViewKind) {
		t.Errorf("OnError got %v, want ErrPlatformViewKind", errs[0])
	}
	if len(pv.open) != 0 {
		t.Errorf("opened %d views for a kind the host does not offer", len(pv.open))
	}
}

// The "report it once" latch must not outlive the kind it was about. An app
// that falls back from "map" to "webview" has to get the web view — a latch
// keyed on nothing would swallow the second request forever, and the symptom
// would be a placeholder that never resolves with no error to explain it.
func TestPlatformViewRetriesAfterTheKindChanges(t *testing.T) {
	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	kind := shell.PlatformViewMap
	var errs int
	root, model := newMutable(func(widget.Ctx) widget.Widget {
		return widget.PlatformView{
			Kind: kind, W: 50, H: 50,
			OnError: func(error) { errs++ },
		}
	})
	h := headless(t, root, 100, 100)
	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	h.Render()
	h.Render() // drain the posted error
	if errs != 1 || len(pv.open) != 0 {
		t.Fatalf("errs=%d opened=%d, want 1/0", errs, len(pv.open))
	}

	kind = shell.PlatformViewWebView
	model.Rebuild()
	h.Render()

	if len(pv.open) != 1 {
		t.Errorf("opened %d views after switching to a supported kind, want 1", len(pv.open))
	}
}

// Scrolled out of the viewport must hide the view, not close it. The whole
// reason is state: a half-filled form or a playing video has to survive being
// scrolled past, and a close-and-reopen loses it (and, on a list of maps,
// reloads every one on every pass).
//
// This is also the test that would catch viewport culling silently stranding a
// view at its last position: culling skips Paint, and Paint is the only thing
// that moves or hides it.
func TestPlatformViewScrolledOutOfViewIsHiddenNotClosed(t *testing.T) {
	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	root := widget.Scroll{
		Child: widget.Column(
			widget.PlatformView{Kind: shell.PlatformViewWebView, H: 50},
			widget.Sized{H: 2000},
		),
	}
	h := headless(t, root, 100, 100)
	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	h.Render()

	if len(pv.open) != 1 {
		t.Fatalf("opened %d views, want 1", len(pv.open))
	}
	view := pv.open[0]
	if view.bounds.IsEmpty() {
		t.Fatalf("view opened with an empty rect: %v", view.bounds)
	}

	// At the pointer, far enough to carry the view off the top. A negative
	// delta scrolls the content up, which is "scroll down" to a reader.
	h.ScrollAt(geom.Pt{X: 50, Y: 25}, geom.Pt{Y: -400})
	h.Render()

	if !view.bounds.IsEmpty() {
		t.Errorf("scrolled far past the view and its bounds are still %v — a "+
			"native view left at its last position composites over whatever is "+
			"there now", view.bounds)
	}
	if view.closes != 0 {
		t.Errorf("closed the view on scroll (%d times); scrolling away must keep "+
			"its state for when it scrolls back", view.closes)
	}
	if len(pv.open) != 1 {
		t.Errorf("opened %d views in total; scrolling must not reopen", len(pv.open))
	}

	// Scrolling back reveals the same view, which is what "keeps its state"
	// looks like from the outside.
	h.ScrollAt(geom.Pt{X: 50, Y: 25}, geom.Pt{Y: 400})
	h.Render()
	if view.bounds.IsEmpty() {
		t.Errorf("scrolled back and the view is still hidden")
	}
	if len(pv.open) != 1 || pv.open[0] != view {
		t.Errorf("scrolling back built a new view instead of revealing the old one")
	}
}

// Clipping is a rect intersection and nothing more, which is all an
// OS-composited view can be given. A view half inside its viewport reports the
// visible half.
func TestPlatformViewIsClippedToItsAncestor(t *testing.T) {
	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	root := widget.Scroll{
		Child: widget.Column(
			widget.Sized{H: 60},
			widget.PlatformView{Kind: shell.PlatformViewWebView, H: 80},
			widget.Sized{H: 2000},
		),
	}
	h := headless(t, root, 100, 100)
	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	h.Render()

	if len(pv.open) != 1 {
		t.Fatalf("opened %d views, want 1", len(pv.open))
	}
	// The view starts at y=60 and is 80 tall, in a 100-tall viewport: the
	// bottom 40 are clipped away.
	if want := geom.RectXYWH(0, 60, 100, 40); pv.open[0].bounds != want {
		t.Errorf("bounds = %v, want %v (clipped to the viewport)", pv.open[0].bounds, want)
	}
}

// Unmounting closes the view, exactly once. This is the leak the capability
// cannot prevent on its own.
func TestPlatformViewClosesOnUnmount(t *testing.T) {
	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	show := true
	root, model := newMutable(func(widget.Ctx) widget.Widget {
		if !show {
			return widget.Sized{W: 10, H: 10}
		}
		return widget.PlatformView{Kind: shell.PlatformViewWebView, W: 50, H: 50}
	})
	h := headless(t, root, 100, 100)
	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	h.Render()
	if len(pv.live()) != 1 {
		t.Fatalf("%d live views after mount, want 1", len(pv.live()))
	}

	show = false
	model.Rebuild()
	h.Render()

	if n := len(pv.live()); n != 0 {
		t.Errorf("%d views still live after unmount — a handle that outlives its "+
			"element leaves a native view over whatever replaced it", n)
	}
	if got := pv.open[0].closes; got != 1 {
		t.Errorf("Close called %d times, want exactly 1", got)
	}
}

// Changing Kind or Params is a different view, so the old one closes and a new
// one opens. The alternative — mutating in place — would be wrong for a map
// (different coordinates is a different camera, not a different view) and is
// not expressible for every kind.
func TestPlatformViewReopensWhenParamsChange(t *testing.T) {
	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	url := "https://a.example"
	root, model := newMutable(func(widget.Ctx) widget.Widget {
		return widget.PlatformView{Kind: shell.PlatformViewWebView, Params: url, W: 50, H: 50}
	})
	h := headless(t, root, 100, 100)
	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	h.Render()

	url = "https://b.example"
	model.Rebuild()
	h.Render()

	if len(pv.open) != 2 {
		t.Fatalf("opened %d views, want 2 (the old one replaced)", len(pv.open))
	}
	if pv.open[0].closes != 1 {
		t.Errorf("the first view was not closed (%d)", pv.open[0].closes)
	}
	if pv.open[1].params != "https://b.example" {
		t.Errorf("second view params = %q", pv.open[1].params)
	}
}

// A capability that arrives after the tree has mounted and built must still be
// picked up. This is the trap app/present.go documents: capabilities are nil
// during Init and the first Build, so a widget that decided once would decide
// "no platform views" forever.
func TestPlatformViewPicksUpALateCapability(t *testing.T) {
	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	root := widget.PlatformView{Kind: shell.PlatformViewWebView, W: 50, H: 50}
	h := headless(t, root, 100, 100)
	h.Render() // mounted, built and painted with no capability at all
	if len(pv.open) != 0 {
		t.Fatal("opened a view before the capability existed")
	}

	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	h.Render()

	if len(pv.open) != 1 {
		t.Errorf("opened %d views after the capability arrived, want 1 — a widget "+
			"that reads the capability once reads it while it is still nil", len(pv.open))
	}
}

// The controller is the app's handle on a live view, and has to be inert
// whenever there is not one — which is most builds. Nothing here may require a
// nil check at the call site.
func TestPlatformViewControllerIsInertWithoutAView(t *testing.T) {
	var ctl widget.PlatformViewController
	if ctl.Ready() {
		t.Error("Ready() before mount")
	}
	ctl.Send("pause") // must not panic

	pv := &fakePV{kinds: []string{shell.PlatformViewWebView}}
	root := widget.PlatformView{
		Kind: shell.PlatformViewWebView, W: 50, H: 50, Controller: &ctl,
	}
	h := headless(t, root, 100, 100)
	h.Owner().WireCapabilities(pvWindow{pv: pv})
	h.Owner().RebuildAll()
	h.Render()

	if len(pv.open) != 1 {
		t.Fatalf("opened %d views, want 1", len(pv.open))
	}
	if ctl.Ready() {
		t.Error("Ready() before the shell reported the view ready")
	}
	pv.open[0].h.OnReady()
	h.Render() // the posted wrapper delivers on the next frame
	if !ctl.Ready() {
		t.Error("Ready() is false after the shell reported ready")
	}
	ctl.Send("pause")
	if got := pv.open[0].sent; len(got) != 1 || got[0] != "pause" {
		t.Errorf("sent %v, want [pause]", got)
	}
}
