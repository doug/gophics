package app

import (
	"image"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// The damage rect has to reach the host, or computing it was pointless.
//
// present() has always called ReplayDamaged with a damage rect and redrawn only
// the dirty region — then handed Put the whole surface, so every embedder
// re-uploaded every pixel of a frame in which one small thing had changed. The
// rect was computed and dropped at the last step.
//
// Three cases pin the contract: the whole surface when there is no previous
// frame for the host to keep, an empty rect when nothing changed at all, and a
// rect smaller than the surface when something small did.
func TestPixelTargetReceivesDamage(t *testing.T) {
	col := paint.RGB(0.2, 0.4, 0.9)
	root := widget.Center(widget.Sized{W: 20, H: 20, Child: widget.Decorated{Color: col}})

	var got []geom.Rect
	f := &fakeFrame{
		size: geom.Size{W: 200, H: 150}, scale: 1,
		tgt: shell.PixelTarget{Put: func(_ *image.RGBA, d geom.Rect) { got = append(got, d) }},
	}
	sh, w := newPresentHarness(t, root)
	surface := geom.Rect{Max: geom.Pt{X: 200, Y: 150}}

	sh.Frame(w, f, 1.0/60)
	if len(got) != 1 {
		t.Fatalf("first frame: %d puts, want 1", len(got))
	}
	if got[0] != surface {
		t.Errorf("first frame damage = %v, want the whole surface %v — there is no "+
			"previous frame for the host to keep", got[0], surface)
	}

	// An unchanged frame damages nothing, so a host may skip the upload.
	got = nil
	sh.Frame(w, f, 1.0/60)
	if len(got) != 1 {
		t.Fatalf("unchanged frame: %d puts, want 1", len(got))
	}
	if !got[0].IsEmpty() {
		t.Errorf("unchanged frame damage = %v, want empty", got[0])
	}

	// Recolour the square: the damage must be the square, not the screen. This
	// is the case the whole change exists for.
	got = nil
	sh.core.Owner.SetRoot(widget.Center(widget.Sized{
		W: 20, H: 20, Child: widget.Decorated{Color: paint.RGB(0.9, 0.3, 0.2)},
	}))
	sh.Frame(w, f, 1.0/60)
	if len(got) != 1 {
		t.Fatalf("changed frame: %d puts, want 1", len(got))
	}
	d := got[0]
	if d.IsEmpty() {
		t.Fatal("a recoloured square damaged nothing")
	}
	if area, all := d.Dx()*d.Dy(), surface.Dx()*surface.Dy(); area >= all/2 {
		t.Errorf("damage %v covers %.0f%% of the surface; a 20x20 change should "+
			"cost far less than half the screen", d, 100*float64(area)/float64(all))
	}
}

// A transparent app must never replay partially.
//
// Its background is blended rather than opaque, so redrawing only the damaged
// region composites this frame's background over the pixels the last frame left
// there — the previous content shows through its own replacement. Skipping an
// unchanged frame is still fine (nothing is redrawn, and the retained surface
// is already correct); it is the *partial* replay that cannot be allowed.
func TestTransparentForcesFullReplay(t *testing.T) {
	small := func(c paint.Color) widget.Widget {
		return widget.Center(widget.Sized{W: 20, H: 20, Child: widget.Decorated{Color: c}})
	}
	h, err := NewHandler(small(paint.RGB(0.2, 0.4, 0.9)), Config{
		Size: geom.Size{W: 200, H: 150}, Transparent: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	sh := h.(*shellHandler)

	var got []geom.Rect
	f := &fakeFrame{
		size: geom.Size{W: 200, H: 150}, scale: 1,
		tgt: shell.PixelTarget{Put: func(_ *image.RGBA, d geom.Rect) { got = append(got, d) }},
	}
	w := &fakeWindow{}
	sh.Frame(w, f, 1.0/60)

	got = nil
	sh.core.Owner.SetRoot(small(paint.RGB(0.9, 0.3, 0.2)))
	sh.Frame(w, f, 1.0/60)

	if len(got) != 1 {
		t.Fatalf("%d puts, want 1", len(got))
	}
	surface := geom.Rect{Max: geom.Pt{X: 200, Y: 150}}
	if got[0] != surface {
		t.Errorf("damage = %v, want the whole surface %v — a translucent "+
			"background cannot be replayed over retained pixels", got[0], surface)
	}
}
