package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

type navHome struct{ log *[]string }

func (h navHome) CreateState() widget.State { return &navHomeState{log: h.log} }

type navHomeState struct {
	widget.StateBase[navHome]
	log     *[]string
	counter int // must survive push/pop underneath the stack
}

func (s *navHomeState) Build(ctx widget.Ctx) widget.Widget {
	nav := ctx.MustOf[widget.Nav]()
	s.counter++
	*s.log = append(*s.log, "home-build")
	return widget.Interactive{
		Gestures: widget.Gestures{OnTap: func() { nav.Push(detail{log: s.log}) }},
		Child:    widget.Sized{W: 200, H: 200},
	}
}

type detail struct{ log *[]string }

func (d detail) Build(ctx widget.Ctx) widget.Widget {
	nav := ctx.MustOf[widget.Nav]()
	*d.log = append(*d.log, "detail-build")
	return widget.Interactive{
		Gestures: widget.Gestures{OnTap: func() { nav.Pop() }},
		Child:    widget.Decorated{Color: pRGB(0.9, 0.2, 0.2), Child: widget.Sized{W: 200, H: 200}},
	}
}

func pRGB(r, g, b float32) (c struct{ R, G, B, A float32 }) {
	return struct{ R, G, B, A float32 }{r, g, b, 1}
}

func settle(h *Headless) {
	for h.Step(0.016) {
		h.Render()
	}
	h.Render()
}

func TestNavigatorPushPop(t *testing.T) {
	var log []string
	h, err := NewHeadless(widget.Navigator{Home: navHome{log: &log}},
		Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// Tap home: pushes detail; the red page covers after settling.
	h.Tap(geom.Pt{X: 100, Y: 100})
	settle(h)
	img := h.Render()
	if r, _, _, _ := img.At(100, 100).RGBA(); r < 0xB000 {
		t.Fatalf("detail page should cover after push, r=%x", r)
	}

	// Tap detail: pops back home.
	h.Tap(geom.Pt{X: 100, Y: 100})
	settle(h)
	img = h.Render()
	if r, _, _, _ := img.At(100, 100).RGBA(); r > 0x8000 {
		t.Fatalf("home should be visible after pop, r=%x", r)
	}
}

func TestNavigatorEdgeSwipeBack(t *testing.T) {
	var log []string
	h, err := NewHeadless(widget.Navigator{Home: navHome{log: &log}},
		Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Tap(geom.Pt{X: 100, Y: 100}) // push detail
	settle(h)
	if r, _, _, _ := h.Render().At(100, 100).RGBA(); r < 0xB000 {
		t.Fatalf("precondition: detail should cover, r=%x", r)
	}

	// Swipe in from the left edge, past the threshold → pop back home.
	h.DragTo(geom.Pt{X: 4, Y: 100}, geom.Pt{X: 120, Y: 100})
	h.Release(geom.Pt{X: 120, Y: 100})
	settle(h)
	if r, _, _, _ := h.Render().At(100, 100).RGBA(); r > 0x8000 {
		t.Fatalf("edge-swipe should pop back to home, r=%x", r)
	}
}

func TestNavigatorShortEdgeSwipeDoesNotPop(t *testing.T) {
	var log []string
	h, err := NewHeadless(widget.Navigator{Home: navHome{log: &log}},
		Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Tap(geom.Pt{X: 100, Y: 100})
	settle(h)
	// A short edge drag (under the threshold) must not pop.
	h.DragTo(geom.Pt{X: 4, Y: 100}, geom.Pt{X: 40, Y: 100})
	h.Release(geom.Pt{X: 40, Y: 100})
	settle(h)
	if r, _, _, _ := h.Render().At(100, 100).RGBA(); r < 0xB000 {
		t.Fatalf("short edge swipe should not pop (detail still up), r=%x", r)
	}
}

func TestNavigatorSlideAnimates(t *testing.T) {
	var log []string
	h, err := NewHeadless(widget.Navigator{Home: navHome{log: &log}},
		Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Tap(geom.Pt{X: 100, Y: 100})
	// Mid-transition: the incoming page is partway across, so the left
	// edge shows home (unpainted background) while the right shows red.
	h.Step(0.05)
	img := h.Render()
	rl, _, _, _ := img.At(4, 100).RGBA()
	rr, _, _, _ := img.At(196, 100).RGBA()
	if rl > 0x4000 || rr < 0xB000 {
		t.Fatalf("mid-slide expected split frame: left=%x right=%x", rl, rr)
	}
	settle(h)
	img = h.Render()
	if r, _, _, _ := img.At(4, 100).RGBA(); r < 0xB000 {
		t.Fatalf("after settle detail covers fully, left=%x", r)
	}
}
