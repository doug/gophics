package mobile

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// The mobile platform-view bridge, driven by a recording host. There is no
// device here, so what is checked is the part that has gone wrong in this file
// family before: the capability is nil until a host registers (the invariant),
// the kinds the host named are the kinds that open, and bounds cross in
// physical pixels rather than logical ones — which is invisible at 1x and
// three times wrong on a real phone.

type recordingPVHost struct {
	created   []string // "id kind params"
	bounds    []string // "id x y w h"
	sent      []string
	destroyed []int
}

func (h *recordingPVHost) CreateView(id int, kind, params string) {
	h.created = append(h.created, sprint(id, kind, params))
}

func (h *recordingPVHost) SetViewBounds(id, x, y, w, hh int) {
	h.bounds = append(h.bounds, sprintInts(id, x, y, w, hh))
}

func (h *recordingPVHost) SendToView(id int, msg string) {
	h.sent = append(h.sent, sprint(id, msg, ""))
}

func (h *recordingPVHost) DestroyView(id int) { h.destroyed = append(h.destroyed, id) }

// A host that has not registered means nil, not an inert capability. This is
// the normal case — most apps embed nothing — and it is what lets an app hide
// the affordance instead of offering a view that never appears.
func TestPlatformViewsNilUntilAHostRegisters(t *testing.T) {
	b := &Bridge{}
	if got := b.PlatformViews(); got != nil {
		t.Errorf("PlatformViews() = %v before a host registered, want nil", got)
	}
	if got := b.WebView(); got != nil {
		t.Errorf("WebView() = %v before a host registered, want nil", got)
	}

	b.SetPlatformViewHost(&recordingPVHost{}, "webview, map")
	pv := b.PlatformViews()
	if pv == nil {
		t.Fatal("PlatformViews() is still nil after a host registered")
	}
	if got := pv.Kinds(); len(got) != 2 || got[0] != "webview" || got[1] != "map" {
		t.Errorf("Kinds() = %v, want [webview map] — the comma-separated list is "+
			"parsed here because gomobile cannot bind a []string parameter", got)
	}
	if b.WebView() == nil {
		t.Error("WebView() is nil though the host offers the webview kind")
	}
}

// A host that registers and names no kinds is a real, distinct answer: the
// capability is published (it can host views) and Kinds is empty (it builds
// none). An app that reads Kinds can tell that from a nil capability and
// degrade differently.
func TestPlatformViewsWithNoKindsIsNotNil(t *testing.T) {
	b := &Bridge{}
	b.SetPlatformViewHost(&recordingPVHost{}, "")
	pv := b.PlatformViews()
	if pv == nil {
		t.Fatal("a registered host with no kinds published nil")
	}
	if got := pv.Kinds(); len(got) != 0 {
		t.Errorf("Kinds() = %v, want empty", got)
	}
	if h := pv.Open("webview", "https://example.com", geom.Rect{}, shell.PlatformViewHandlers{}); h != nil {
		t.Error("Open succeeded for a kind the host did not register")
	}
	if b.WebView() != nil {
		t.Error("WebView() is non-nil though the host offers no webview kind")
	}
}

// Bounds cross in physical pixels, the same units as SetSurface and SetInsets.
// Android view geometry is already in pixels and an iOS host divides by the
// screen scale; a Bridge that sent logical pixels would be a third of the size
// on a 3x phone and nobody would see it on a simulator.
func TestPlatformViewBoundsCrossInPhysicalPixels(t *testing.T) {
	host := &recordingPVHost{}
	b := &Bridge{scale: 3}
	b.SetPlatformViewHost(host, "webview")
	pv := b.PlatformViews()

	h := pv.Open("webview", "https://example.com", geom.RectXYWH(10, 20, 30, 40), shell.PlatformViewHandlers{})
	if h == nil {
		t.Fatal("Open returned no handle for a registered kind")
	}
	if len(host.bounds) != 1 || host.bounds[0] != "1 30 60 90 120" {
		t.Fatalf("bounds = %v, want [1 30 60 90 120] (logical x3)", host.bounds)
	}

	// An empty rect is one canonical "hidden" rather than whichever degenerate
	// rect the layout produced, so a host can test w == 0 and be done.
	h.SetBounds(geom.RectXYWH(5, 5, 0, 12))
	if got := host.bounds[len(host.bounds)-1]; got != "1 0 0 0 0" {
		t.Errorf("an empty rect crossed as %q, want \"1 0 0 0 0\"", got)
	}
}

// Ready, failure and messages all correlate by the id the Bridge handed the
// host, and a closed view hears nothing — the host may still call back for an
// id it has already been asked to destroy.
func TestPlatformViewCallbacksCorrelateById(t *testing.T) {
	host := &recordingPVHost{}
	b := &Bridge{scale: 1}
	b.SetPlatformViewHost(host, "webview")
	pv := b.PlatformViews()

	var ready, fails, msgs int
	h := pv.Open("webview", "https://a.example", geom.RectXYWH(0, 0, 10, 10), shell.PlatformViewHandlers{
		OnReady:   func() { ready++ },
		OnError:   func(error) { fails++ },
		OnMessage: func(string) { msgs++ },
	})
	id := 1 // the first request id this Bridge issues
	if len(host.created) != 1 || host.created[0] != "1 webview https://a.example" {
		t.Fatalf("created = %v", host.created)
	}

	b.DeliverViewReady(id)
	b.DeliverViewMessage(id, "loaded")
	b.FailView(id+1, "some other view") // an id that is not ours
	if ready != 1 || msgs != 1 || fails != 0 {
		t.Errorf("ready=%d msgs=%d fails=%d, want 1/1/0", ready, msgs, fails)
	}

	h.Send("https://b.example")
	if len(host.sent) != 1 {
		t.Errorf("sent = %v", host.sent)
	}

	h.Close()
	if len(host.destroyed) != 1 || host.destroyed[0] != id {
		t.Errorf("destroyed = %v, want [%d]", host.destroyed, id)
	}
	// Idempotent, and deaf afterwards: the widget closes on unmount without
	// knowing whether the view already failed.
	h.Close()
	b.DeliverViewReady(id)
	b.DeliverViewMessage(id, "late")
	if len(host.destroyed) != 1 {
		t.Errorf("Close is not idempotent: destroyed = %v", host.destroyed)
	}
	if ready != 1 || msgs != 1 {
		t.Errorf("a closed view still heard callbacks: ready=%d msgs=%d", ready, msgs)
	}
}

// sprint/sprintInts keep the recording host's expectations readable without
// pulling fmt into a file that is otherwise about integers.
func sprint(id int, a, b string) string {
	s := itoa(id) + " " + a
	if b != "" {
		s += " " + b
	}
	return s
}

func sprintInts(vals ...int) string {
	s := ""
	for i, v := range vals {
		if i > 0 {
			s += " "
		}
		s += itoa(v)
	}
	return s
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	if neg {
		v = -v
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
