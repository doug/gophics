package app

import (
	"testing"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// field is a focusable strip recording focus and text events.
type field struct {
	Name string
	Log  *[]string
}

func (f field) Build(widget.Ctx) widget.Widget {
	name := f.Name
	log := f.Log
	return widget.Interactive{
		Handler: widget.Handler{
			OnText:  func(s string) { *log = append(*log, name+":text:"+s) },
			OnFocus: func(v bool) { *log = append(*log, name+":focus:"+b2s(v)) },
		},
		Child: widget.Sized{W: 100, H: 20},
	}
}

func b2s(v bool) string {
	if v {
		return "t"
	}
	return "f"
}

func headless(t *testing.T, root widget.Widget) *Headless {
	t.Helper()
	h, err := NewHeadless(root, Config{Size: geom.Size{W: 200, H: 200}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

func TestTapToFocus(t *testing.T) {
	var log []string
	col := widget.Column(
		field{Name: "a", Log: &log},
		field{Name: "b", Log: &log},
	)
	h := headless(t, col)

	// Autofocus: first mounted focusable took focus.
	if len(log) == 0 || log[0] != "a:focus:t" {
		t.Fatalf("autofocus log = %v", log)
	}
	h.Type("x")
	if log[len(log)-1] != "a:text:x" {
		t.Fatalf("text should go to a: %v", log)
	}

	// Tap the second field: focus moves with callbacks.
	h.Tap(geom.Pt{X: 100, Y: 30})
	h.Type("y")
	n := len(log)
	if log[n-3] != "a:focus:f" || log[n-2] != "b:focus:t" || log[n-1] != "b:text:y" {
		t.Fatalf("focus transition log = %v", log)
	}
}

func TestDragCancelsTapAndDelivers(t *testing.T) {
	taps, drag := 0, geom.Pt{}
	root := widget.Center(widget.Interactive{
		Handler: widget.Handler{
			OnTap:  func() { taps++ },
			OnDrag: func(d geom.Pt) { drag = drag.Add(d) },
		},
		Child: widget.Sized{W: 100, H: 100},
	})
	h := headless(t, root)

	h.Tap(geom.Pt{X: 100, Y: 100})
	if taps != 1 {
		t.Fatalf("plain tap: taps = %d", taps)
	}
	h.Drag(geom.Pt{X: 100, Y: 100}, geom.Pt{X: 100, Y: 130})
	if taps != 1 {
		t.Fatal("drag beyond slop must cancel the tap")
	}
	if drag.Y != 30 {
		t.Fatalf("drag delta = %v, want Y=30", drag)
	}
}

func TestScrollWidget(t *testing.T) {
	// 10 rows of 30px in a 100px-tall window: max offset 200.
	rows := make([]widget.Widget, 10)
	for i := range rows {
		rows[i] = widget.Sized{W: 100, H: 30}
	}
	h, err := NewHeadless(
		widget.Scroll{Child: widget.Column(rows...)},
		Config{Size: geom.Size{W: 100, H: 100}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	vp := findViewport(h.Core.Owner.RootBox())
	if vp == nil {
		t.Fatal("no viewport in tree")
	}
	if vp.MaxOffset() != 200 {
		t.Fatalf("MaxOffset = %v, want 200", vp.MaxOffset())
	}

	h.Move(geom.Pt{X: 50, Y: 50})
	h.Scroll(geom.Pt{Y: -40}) // scroll down
	h.Render()
	if vp.Offset != 40 {
		t.Fatalf("offset after scroll = %v, want 40", vp.Offset)
	}

	h.Scroll(geom.Pt{Y: -1000}) // overscroll clamps
	h.Render()
	if vp.Offset != 200 {
		t.Fatalf("clamped offset = %v, want 200", vp.Offset)
	}

	// Drag-to-scroll works too.
	h.Drag(geom.Pt{X: 50, Y: 80}, geom.Pt{X: 50, Y: 30})
	h.Render()
	if vp.Offset != 200 {
		t.Fatalf("drag up at bottom stays clamped, got %v", vp.Offset)
	}
	h.Drag(geom.Pt{X: 50, Y: 30}, geom.Pt{X: 50, Y: 80})
	h.Render()
	if vp.Offset != 150 {
		t.Fatalf("drag down offset = %v, want 150", vp.Offset)
	}
}

func findViewport(b layout.Box) *layout.Viewport {
	for _, hit := range layout.HitTest(b, geom.Pt{X: 1, Y: 1}) {
		if vp, ok := hit.Box.(*layout.Viewport); ok {
			return vp
		}
	}
	return nil
}

func TestViewportClips(t *testing.T) {
	// A red 30px row scrolled fully out of a 20px viewport must not paint.
	red := paint.RGB(1, 0, 0)
	rows := widget.Column(
		widget.Decorated{Color: red, Child: widget.Sized{W: 50, H: 30}},
		widget.Sized{W: 50, H: 30},
	)
	h, err := NewHeadless(widget.Scroll{Child: rows},
		Config{Size: geom.Size{W: 50, H: 20}, Background: paint.RGB(0, 0, 0)}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Move(geom.Pt{X: 25, Y: 10})
	h.Scroll(geom.Pt{Y: -30}) // scroll red fully out
	img := h.Render()

	r, _, _, _ := img.At(25, 10).RGBA()
	if r > 0x1000 {
		t.Fatalf("red leaked through viewport clip: r=%x", r)
	}
}
