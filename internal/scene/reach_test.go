package scene_test

import (
	"bytes"
	"image"
	"math"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/paint"
)

// incrementalMatchesFull replays a fully, then b clipped to b.Diff(a), and
// reports whether the result is pixel-identical to a full render of b.
func incrementalMatchesFull(t *testing.T, a, b *scene.List, drawB func(paint.Canvas)) (damage geom.Rect, same bool) {
	t.Helper()
	damage, changed := b.Diff(a, measurer(t))
	if !changed {
		t.Fatal("changed scene must report change")
	}
	direct := render(t, drawB)
	incremental := renderWith(t, func(c paint.Canvas, m scene.Measurer) {
		a.Replay(c)
		c.PushClip(damage)
		b.ReplayDamage(c, damage, m)
		c.PopClip()
	})
	return damage, bytes.Equal(direct, incremental)
}

// The CPU backdrop blur is three box passes of the radius, so a blurred pixel
// depends on backdrop up to three radii away — not one. A change between one
// and three radii from a panel used to repaint alone, and the panel kept a
// blur of the old colour.
func TestDiffBlurReachMatchesTheKernel(t *testing.T) {
	const r = 12
	panel := geom.RectXYWH(60, 40, 60, 60)
	for _, tc := range []struct {
		name string
		gap  float32
		want bool // the panel repaints
	}{
		{"one radius out", r + 2, true},
		{"two radii out", 2 * r, true},
		{"just inside the reach", 3*r - 1, true},
		{"beyond the reach", 3*r + 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			strip := geom.RectXYWH(panel.Max.X+tc.gap, 40, 20, 60)
			frame := func(c paint.Canvas, col paint.Color) {
				c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.RGB(1, 1, 1))
				c.FillRect(strip, col)
				c.BackdropBlur(panel, r)
			}
			var a, b scene.List
			frame(a.Recorder(), paint.RGB(0, 0, 0))
			frame(b.Recorder(), paint.RGB(1, 1, 1))
			damage, same := incrementalMatchesFull(t, &a, &b, func(c paint.Canvas) { frame(c, paint.RGB(1, 1, 1)) })
			if got := damage.Union(panel) == damage; got != tc.want {
				t.Errorf("gap %v: damage %v includes the panel = %v, want %v", tc.gap, damage, got, tc.want)
			}
			if !same {
				t.Errorf("gap %v: incremental replay left stale blur in the panel", tc.gap)
			}
		})
	}
}

// DrawSprite rotates about Dst's centre, so a rotated sprite's corners lie
// outside Dst. Damage taken from Dst alone left those corners stale.
func TestDiffRotatedSpriteDamageCoversItsCorners(t *testing.T) {
	atlas := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for i := range atlas.Pix {
		atlas.Pix[i] = 255
	}
	dst := geom.RectXYWH(80, 60, 40, 40)
	frame := func(c paint.Canvas, rot float32) {
		c.FillRect(geom.RectXYWH(0, 0, 200, 160), paint.RGB(0, 0, 0))
		c.DrawSprite(atlas, paint.Sprite{Src: atlas.Bounds(), Dst: dst, Rotation: rot})
	}
	var a, b scene.List
	frame(a.Recorder(), 0)
	frame(b.Recorder(), math.Pi/4)
	damage, same := incrementalMatchesFull(t, &a, &b, func(c paint.Canvas) { frame(c, math.Pi/4) })
	reach := float32(20*math.Sqrt2) - 20 // how far a 45° corner sticks out of Dst
	if damage.Min.X > dst.Min.X-reach || damage.Max.X < dst.Max.X+reach ||
		damage.Min.Y > dst.Min.Y-reach || damage.Max.Y < dst.Max.Y+reach {
		t.Errorf("damage %v does not cover the corners of %v rotated 45° (they reach %.1f beyond)", damage, dst, reach)
	}
	if !same {
		t.Error("incremental replay of a rotated sprite diverged from a full repaint")
	}
}
