package scene_test

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// The two colours every blend assertion below is built from. Neither channel
// pair is equal and none is 0 or 1, so a mode that silently composites
// normally shows up as a wrong number rather than a coincidence — the same
// pair paint/blend_test.go pins its mode table to.
const (
	dstR, dstG, dstB = 204, 153, 51 // RGB(0.8, 0.6, 0.2)
	srcR, srcG, srcB = 51, 102, 229 // RGB(0.2, 0.4, 0.9)
)

func dstColor() paint.Color { return paint.RGB(0.8, 0.6, 0.2) }
func srcColor() paint.Color { return paint.RGB(0.2, 0.4, 0.9) }

// blendedFrame paints an opaque destination and then an overlapping source
// under mode, through whatever Canvas it is given. It is the scene every test
// here records, replays or renders directly.
func blendedFrame(t *testing.T, c paint.Canvas, mode paint.BlendMode) {
	t.Helper()
	c.FillRect(geom.RectXYWH(0, 0, 200, 160), dstColor())
	b := paint.Blends(c)
	if b == nil {
		t.Fatalf("%T does not implement paint.Blender", c)
	}
	b.PushBlend(mode)
	c.FillRect(geom.RectXYWH(40, 40, 80, 60), srcColor())
	b.PopBlend()
}

// allBlendModes is every mode paint exposes, discovered rather than listed:
// String names the known ones and reports the rest as unknown, so a mode added
// to paint joins every table below without anyone remembering to add it.
func allBlendModes(t *testing.T) []paint.BlendMode {
	t.Helper()
	var out []paint.BlendMode
	for m := paint.BlendNormal; m.String() != "BlendMode(?)"; m++ {
		out = append(out, m)
	}
	// Belt and braces: a String that stopped naming modes would quietly turn
	// every loop below into zero iterations, which is the failure mode AGENTS
	// warns about — a test that passes by not running.
	if len(out) < 29 {
		t.Fatalf("found %d blend modes, want the W3C set of at least 29", len(out))
	}
	return out
}

// renderAt and renderWithAt are render/renderWith at a chosen device scale.
// The scale is the point: logical coordinates above, physical below, and a
// blend or a gradient that is right at 1x can be wrong at 2x.
func renderAt(t *testing.T, scale float32, draw func(paint.Canvas)) []byte {
	t.Helper()
	return renderWithAt(t, scale, func(c paint.Canvas, _ scene.Measurer) { draw(c) })
}

func renderWithAt(t *testing.T, scale float32, draw func(paint.Canvas, scene.Measurer)) []byte {
	t.Helper()
	p := paint.NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	c := p.BeginOffscreen(geom.Size{W: 200, H: 160}, scale)
	draw(c, p)
	var buf bytes.Buffer
	if err := png.Encode(&buf, p.Image()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func pixel(t *testing.T, img image.Image, x, y int) (r, g, b, a uint8) {
	t.Helper()
	cr, cg, cb, ca := img.At(x, y).RGBA()
	return uint8(cr >> 8), uint8(cg >> 8), uint8(cb >> 8), uint8(ca >> 8)
}

// TestWidgetDrawBlendsDuringARealFrame is the test this whole op pair exists
// for, and the one a check of the offscreen painter alone would pass while the
// feature was unreachable.
//
// The canvas a widget's Draw receives in a real frame is not the painter
// canvas — app records the frame into a scene.List first and replays it
// afterwards — so paint.Blends(c) answering nil for the recorder meant blend
// modes worked in paint's own tests and in no application anywhere. This drives
// the whole pipeline: widget tree, record, diff, damaged replay, pixels.
func TestWidgetDrawBlendsDuringARealFrame(t *testing.T) {
	render := func(mode paint.BlendMode) image.Image {
		t.Helper()
		var reached bool
		root := widget.Canvas{Draw: func(c paint.Canvas, size geom.Size) {
			reached = true
			if paint.Blends(c) == nil {
				t.Errorf("paint.Blends returned nil for %T, the canvas a "+
					"widget's Draw is handed during a frame", c)
				return
			}
			blendedFrame(t, c, mode)
		}}
		h, err := app.NewHeadless(root, app.Config{
			Size:       geom.Size{W: 200, H: 160},
			Background: paint.RGB(0, 0, 0),
			EdgeToEdge: true,
		}, 1)
		if err != nil {
			t.Fatal(err)
		}
		img := h.Render()
		if !reached {
			t.Fatal("the widget's Draw never ran")
		}
		return img
	}

	// Inside the source rect, Normal leaves the source and Multiply leaves
	// d*s/255 per channel. Asserting the numbers rather than "the two differ"
	// is what proves the recorded mode reached the rasterizer instead of being
	// dropped on the way through the display list.
	r, g, b, _ := pixel(t, render(paint.BlendNormal), 80, 70)
	if r != srcR || g != srcG || b != srcB {
		t.Errorf("BlendNormal painted (%d,%d,%d), want the source (%d,%d,%d)",
			r, g, b, srcR, srcG, srcB)
	}
	r, g, b, _ = pixel(t, render(paint.BlendMultiply), 80, 70)
	if want := [3]uint8{41, 61, 46}; r != want[0] || g != want[1] || b != want[2] {
		t.Errorf("BlendMultiply painted (%d,%d,%d), want d*s/255 = %v; a mode "+
			"set in a widget's Draw did not change the pixels", r, g, b, want)
	}
	// Outside the source rect only the destination was painted, so the mode
	// must not have leaked past its PopBlend.
	r, g, b, _ = pixel(t, render(paint.BlendMultiply), 160, 140)
	if r != dstR || g != dstG || b != dstB {
		t.Errorf("outside the blended shape = (%d,%d,%d), want the untouched "+
			"destination (%d,%d,%d)", r, g, b, dstR, dstG, dstB)
	}
}

// TestRecorderBlendsRoundTripThroughTheList pins the display list itself: a
// recorded mode must replay to the same pixels a direct paint produces, for
// every mode, at 1x and 2x. Anything the recorder dropped or reordered — the
// mode, its scope, the balance of the stack — comes out as a pixel difference
// here.
func TestRecorderBlendsRoundTripThroughTheList(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		for _, mode := range allBlendModes(t) {
			direct := renderAt(t, scale, func(c paint.Canvas) { blendedFrame(t, c, mode) })

			var list scene.List
			blendedFrame(t, list.Recorder(nil), mode)
			if list.HasLayers() {
				t.Fatalf("%v: a blend group must not force a full repaint", mode)
			}
			replayed := renderAt(t, scale, func(c paint.Canvas) { list.Replay(c) })

			if !bytes.Equal(direct, replayed) {
				t.Errorf("%v at %gx: replay through the display list diverged "+
					"from a direct paint", mode, scale)
			}
		}
	}
}

// TestDamagedBlendReplayMatchesFullRepaint is the damage half of the same
// claim, and the one that could be subtly wrong: a blend mode reads the
// destination, so a replay restricted to a damage rect has to reconstruct the
// same destination a full repaint would have built.
//
// It does, for every mode, because no mode samples anything but the pixel it
// is writing — see the note on ReplayDamage. The oracle is the one
// TestDamageReplayOpacityIncrementalMatchesFull uses: paint frame A in full,
// then partially replay frame B onto it, and demand the exact pixels of a
// fresh full render of B.
func TestDamagedBlendReplayMatchesFullRepaint(t *testing.T) {
	// Frame B moves the destination rect under the blended shape, so the
	// damage lands squarely across it and the blend has to be recomputed
	// against freshly repainted pixels rather than retained ones.
	frame := func(c paint.Canvas, mode paint.BlendMode, shift float32) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.RGB(0.1, 0.1, 0.12))
		c.FillRect(geom.RectXYWH(20, 30+shift, 120, 50), dstColor())
		b := paint.Blends(c)
		if b == nil {
			t.Fatalf("%T does not implement paint.Blender", c)
		}
		b.PushBlend(mode)
		c.FillRect(geom.RectXYWH(40, 40, 80, 60), srcColor())
		b.PopBlend()
	}

	for _, scale := range []float32{1, 2} {
		for _, mode := range allBlendModes(t) {
			var a, b scene.List
			frame(a.Recorder(nil), mode, 0)
			frame(b.Recorder(nil), mode, 6)

			direct := renderAt(t, scale, func(c paint.Canvas) { frame(c, mode, 6) })
			incremental := renderWithAt(t, scale, func(c paint.Canvas, m scene.Measurer) {
				a.Replay(c) // the retained surface
				damage, changed := b.Diff(&a, m)
				if !changed {
					t.Fatalf("%v: a moved shape must report change", mode)
				}
				c.PushClip(damage)
				b.ReplayDamage(c, damage, m)
				c.PopClip()
			})
			if !bytes.Equal(direct, incremental) {
				t.Errorf("%v at %gx: damaged replay of a blended scene diverged "+
					"from a full repaint", mode, scale)
			}
		}
	}
}

// TestBlendModeChangeDamagesTheGroupContent is the differ trap in isolation.
//
// Two frames that differ only in their blend mode hold byte-identical draw
// ops, so the positional diff matches every one of them in its common suffix
// and the sole op in the changed window is the push itself — which paints
// nothing and whose own bounds are empty. Taking those bounds literally gives
// empty damage, and the frame keeps the shapes it painted under the old mode.
func TestBlendModeChangeDamagesTheGroupContent(t *testing.T) {
	m := measurer(t)
	var a, b scene.List
	blendedFrame(t, a.Recorder(nil), paint.BlendMultiply)
	blendedFrame(t, b.Recorder(nil), paint.BlendScreen)

	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("a changed blend mode must report change")
	}
	// The group's content is the one blended fill; the destination fill before
	// the push is unchanged and outside the group.
	want := geom.RectXYWH(40, 40, 80, 60)
	if damage != want {
		t.Fatalf("damage = %v, want the blend group's content %v", damage, want)
	}
}

// TestBlendModeChangeRepaintsThePixels is the same thing one level down: that
// the damage above is not merely non-empty but sufficient. A mode swap with
// nothing else moving is exactly the animation an app would write.
func TestBlendModeChangeRepaintsThePixels(t *testing.T) {
	var a, b scene.List
	blendedFrame(t, a.Recorder(nil), paint.BlendMultiply)
	blendedFrame(t, b.Recorder(nil), paint.BlendScreen)

	direct := render(t, func(c paint.Canvas) { blendedFrame(t, c, paint.BlendScreen) })
	incremental := renderWith(t, func(c paint.Canvas, m scene.Measurer) {
		a.Replay(c)
		damage, _ := b.Diff(&a, m)
		c.PushClip(damage)
		b.ReplayDamage(c, damage, m)
		c.PopClip()
	})
	if !bytes.Equal(direct, incremental) {
		t.Fatal("a frame that changed only its blend mode kept the pixels it " +
			"painted under the old one")
	}
}

// TestBlendGroupRemovalDamagesWhatItPainted covers the other three edits to a
// group boundary — adding one, removing one, and the ops shifting position
// around it — through the case that erases rather than tints. BlendDstOut
// knocks a hole in the destination, so a removal that was not repainted leaves
// a visible hole in a frame that does not ask for one.
func TestBlendGroupRemovalDamagesWhatItPainted(t *testing.T) {
	frame := func(c paint.Canvas, blended bool) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), dstColor())
		if blended {
			b := paint.Blends(c)
			b.PushBlend(paint.BlendDstOut)
			c.FillRect(geom.RectXYWH(40, 40, 80, 60), srcColor())
			b.PopBlend()
			return
		}
		c.FillRect(geom.RectXYWH(40, 40, 80, 60), srcColor())
	}

	var a, b scene.List
	frame(a.Recorder(nil), true)
	frame(b.Recorder(nil), false)

	direct := render(t, func(c paint.Canvas) { frame(c, false) })
	incremental := renderWith(t, func(c paint.Canvas, m scene.Measurer) {
		a.Replay(c)
		damage, changed := b.Diff(&a, m)
		if !changed {
			t.Fatal("removing a blend group must report change")
		}
		c.PushClip(damage)
		b.ReplayDamage(c, damage, m)
		c.PopClip()
	})
	if !bytes.Equal(direct, incremental) {
		t.Fatal("removing a BlendDstOut group left the hole it had punched")
	}
}

// TestBlendSurvivesACulledNeighbour pins the balance of the push/pop pair
// across a damage replay that skips ops. ReplayDamage walks the whole list and
// culls by bounds, so a blend boundary that could be culled would leave the
// mode set for the rest of the frame (a dropped pop) or unset for the group (a
// dropped push) — and the ops either side of this group are well outside the
// damage rect precisely so that they are culled.
func TestBlendSurvivesACulledNeighbour(t *testing.T) {
	frame := func(c paint.Canvas, tint paint.Color) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.RGB(0.1, 0.1, 0.12))
		c.FillRect(geom.RectXYWH(0, 0, 20, 20), paint.RGB(0, 0.6, 0)) // culled
		b := paint.Blends(c)
		b.PushBlend(paint.BlendMultiply)
		c.FillRect(geom.RectXYWH(60, 60, 80, 60), tint)
		b.PopBlend()
		c.FillRect(geom.RectXYWH(180, 140, 20, 20), paint.RGB(0.6, 0, 0)) // culled
	}

	var a, b scene.List
	frame(a.Recorder(nil), paint.RGB(0.2, 0.4, 0.9))
	frame(b.Recorder(nil), paint.RGB(0.3, 0.5, 0.8))

	direct := render(t, func(c paint.Canvas) { frame(c, paint.RGB(0.3, 0.5, 0.8)) })
	incremental := renderWith(t, func(c paint.Canvas, m scene.Measurer) {
		a.Replay(c)
		damage, _ := b.Diff(&a, m)
		if damage.Dx() >= 200 {
			t.Fatalf("damage %v should be the tinted rect, not the surface", damage)
		}
		c.PushClip(damage)
		b.ReplayDamage(c, damage, m)
		c.PopClip()
	})
	if !bytes.Equal(direct, incremental) {
		t.Fatal("an unbalanced blend stack survived a damaged replay")
	}
}

// TestNestedBlendAndOpacityGroupsDiffSeparately guards the one place the two
// group kinds could be confused: groupBounds counts nesting depth in the kind
// it was asked about, so an opacity group holding a blend group (and the
// reverse) must each still find their own partner rather than stopping at the
// other's.
func TestNestedBlendAndOpacityGroupsDiffSeparately(t *testing.T) {
	frame := func(c paint.Canvas, alpha float32, mode paint.BlendMode) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), dstColor())
		b := paint.Blends(c)
		c.PushOpacity(alpha)
		b.PushBlend(mode)
		c.FillRect(geom.RectXYWH(40, 40, 60, 40), srcColor())
		b.PopBlend()
		c.FillRect(geom.RectXYWH(50, 90, 60, 40), srcColor())
		c.PopOpacity()
	}

	m := measurer(t)
	var a, b scene.List
	frame(a.Recorder(nil), 0.4, paint.BlendMultiply)
	frame(b.Recorder(nil), 0.9, paint.BlendMultiply)

	damage, changed := b.Diff(&a, m)
	if !changed {
		t.Fatal("a changed alpha must report change")
	}
	// The opacity group's content is both fills; the blend boundary inside it
	// contributes no bounds of its own and must not have truncated the scan.
	want := geom.RectXYWH(40, 40, 70, 90)
	if damage != want {
		t.Fatalf("damage = %v, want the opacity group's whole content %v",
			damage, want)
	}

	// And the other way round: changing the mode inside damages only the
	// blend group, not the opacity group around it.
	var cc scene.List
	frame(cc.Recorder(nil), 0.4, paint.BlendScreen)
	damage, changed = cc.Diff(&a, m)
	if !changed {
		t.Fatal("a changed blend mode must report change")
	}
	if want := geom.RectXYWH(40, 40, 60, 40); damage != want {
		t.Fatalf("damage = %v, want just the blend group's content %v", damage, want)
	}
}

// TestRecorderReplayedOntoARecorderKeepsTheMode: a list replayed into another
// list must carry the blend ops over, because that is how a cached subtree is
// re-recorded into a parent frame.
func TestRecorderReplayedOntoARecorderKeepsTheMode(t *testing.T) {
	var a scene.List
	blendedFrame(t, a.Recorder(nil), paint.BlendOverlay)

	var b scene.List
	a.Replay(b.Recorder(nil))
	if b.Len() != a.Len() {
		t.Fatalf("re-recorded %d ops, want the original %d", b.Len(), a.Len())
	}
	if _, changed := b.Diff(&a, measurer(t)); changed {
		t.Fatal("a list re-recorded through a recorder differs from the original")
	}
}
