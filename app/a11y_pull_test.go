package app

import (
	"image"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// toggleApp is a button that, once tapped, adds a labeled node under itself.
type toggleApp struct{ on *bool }

func (a toggleApp) Build(widget.Ctx) widget.Widget {
	kids := []widget.Widget{
		widget.Interactive{
			Gestures: widget.Gestures{OnTap: func() { *a.on = !*a.on }},
			Child:    widget.Sized{W: 200, H: 50},
		},
	}
	if *a.on {
		kids = append(kids, widget.Semantics{Label: "new", Child: widget.Sized{W: 200, H: 50}})
	}
	return widget.Column(kids...)
}

// The semantics tree read between an event and its frame is built AND laid
// out. It used to be flushed through RootBox alone, which consumed the dirty
// set without placing the new children: the node added by the tap was missing
// (a Flex visits only the children it has laid out), and the frame that
// followed did not know there had been anything to build.
func TestSemanticsBetweenEventAndFrameIsLaidOut(t *testing.T) {
	on := false
	h, err := NewHeadless(toggleApp{&on}, Config{Size: geom.Size{W: 200, H: 200}}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	h.Tap(geom.Pt{X: 10, Y: 10})
	if !on {
		t.Fatal("tap did not toggle")
	}
	h.Owner().RebuildAll() // what a SetState in the tap handler does
	var got *layout.SemNode
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Label == "new" {
			got = &n
		}
	}
	if got == nil {
		t.Fatal("the node added by the tap is not in the tree read before the frame")
	}
	if want := geom.RectXYWH(0, 50, 200, 50); got.Rect != want {
		t.Errorf("node added by the tap sits at %v before the frame, want %v: read without a layout", got.Rect, want)
	}
	if got.Offscreen || got.Visible != got.Rect {
		t.Errorf("node added by the tap is offscreen=%v visible=%v before the frame", got.Offscreen, got.Visible)
	}
}

// A build flushed by an out-of-frame pull of the tree — the mobile bridge's
// A11yRefresh runs on the platform's schedule, not the frame's — still
// reaches the bridge on the next frame. The pull used to flush the build
// without recording it, so a semantic-only change was published only if the
// pull itself happened to be the reader; a push-side bridge kept the old tree.
func TestOutOfFrameA11yPullStillRepublishes(t *testing.T) {
	label := "one"
	h, err := NewHandler(semLabelApp{&label}, Config{Size: geom.Size{W: 100, H: 100}})
	if err != nil {
		t.Fatal(err)
	}
	sh := h.(*shellHandler)
	at := &fakeAT{}
	w := fakeA11yWindow{at: at}
	f := &fakeFrame{size: geom.Size{W: 100, H: 100}, scale: 1,
		tgt: shell.PixelTarget{Put: func(*image.RGBA, geom.Rect) {}}}
	sh.Frame(w, f, 0)
	sh.Frame(w, f, 1.0/60)
	if len(at.trees) != 1 || groupLabel(at.trees[0]) != "one" {
		t.Fatalf("after two frames: %d trees, label %q", len(at.trees), groupLabel(at.trees[len(at.trees)-1]))
	}

	label = "two"
	sh.Event(w, shell.Insets{}) // RebuildAll, as any state change would
	if got := groupLabel(sh.A11yTree(1)); got != "two" {
		t.Fatalf("tree pulled between the event and its frame has label %q", got)
	}
	sh.Frame(w, f, 1.0/60)
	if got := groupLabel(at.trees[len(at.trees)-1]); got != "two" {
		t.Errorf("label changed to %q and was pulled before the frame; the bridge still has %q", label, got)
	}
	if len(at.trees) != 2 {
		t.Errorf("published %d trees, want 2", len(at.trees))
	}
}
