//go:build gophics_gpu

package paint

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// TestGPUEmojiDraws is the GPU half of the mandatory case: an emoji-only label
// must put colour on the surface through the real direct-surface path, not just
// through the CPU rasterizer.
func TestGPUEmojiDraws(t *testing.T) {
	p := emojiPainter(t)
	const w, h = 80, 80
	p.BeginOffscreen(geom.Size{W: w, H: h}, 1)
	img := renderGPU(t, w, h, func(cc *gg.Context) {
		c := p.GPUCanvas(cc)
		c.Clear(RGB(0.5, 0.5, 0.5))
		c.TextIn("", emoji, geom.Pt{X: 8, Y: 60}, 40, RGB(0, 0, 0))
	})
	writePNG(t, "gpu_emoji.png", img)
	if n := countChromatic(img, img.Bounds()); n < 50 {
		t.Errorf("GPU emoji drew %d coloured pixels, want many", n)
	}
}

// TestGPUEmojiMixedRunKeepsBothHalves checks the decomposition on the GPU path:
// the Latin spans must still reach the glyph-mask tier and the emoji must still
// blit, in one frame.
func TestGPUEmojiMixedRunKeepsBothHalves(t *testing.T) {
	p := emojiPainter(t)
	const w, h = 260, 70
	p.BeginOffscreen(geom.Size{W: w, H: h}, 1)
	img := renderGPU(t, w, h, func(cc *gg.Context) {
		c := p.GPUCanvas(cc)
		c.Clear(RGB(1, 1, 1))
		c.TextIn("", "hi "+emoji+" there", geom.Pt{X: 4, Y: 48}, 26, RGB(0, 0, 0))
	})
	writePNG(t, "gpu_emoji_mixed.png", img)
	chroma := countChromatic(img, img.Bounds())
	dark := countDarkRect(img, img.Bounds())
	t.Logf("chromatic=%d dark=%d", chroma, dark)
	if chroma < 30 {
		t.Errorf("GPU mixed run: %d coloured pixels (emoji missing)", chroma)
	}
	if dark < 30 {
		t.Errorf("GPU mixed run: %d dark pixels (Latin missing)", dark)
	}
}

// TestGPUEmojiOrderAgainstText pins the invariant the whole route rests on:
// image draws queue into the same ordered draw list as shapes and text, so a
// glyph blitted before an opaque rect is covered by it, and one blitted after
// covers it.
//
// This is the measurement that retired the old "a mid-frame image draw drops
// the queued shapes" warning in TextIn. If it ever fails, that warning was
// right after all and this route has to be rethought.
func TestGPUEmojiOrderAgainstText(t *testing.T) {
	p := emojiPainter(t)
	const w, h = 80, 80
	p.BeginOffscreen(geom.Size{W: w, H: h}, 1)
	box := geom.Rect{Min: geom.Pt{X: 0, Y: 0}, Max: geom.Pt{X: w, Y: h}}

	under := renderGPU(t, w, h, func(cc *gg.Context) {
		c := p.GPUCanvas(cc)
		c.Clear(RGB(1, 1, 1))
		c.TextIn("", emoji, geom.Pt{X: 8, Y: 60}, 40, RGB(0, 0, 0))
		c.FillRect(box, RGB(0, 0, 1)) // opaque, drawn after: must hide the emoji
	})
	// countChromatic is no use here — a solid blue fill is itself chromatic —
	// so assert the stronger thing: every pixel is the blue, i.e. nothing of
	// the emoji shows through.
	for y := range h {
		for x := range w {
			o := y*under.Stride + x*4
			r, g, b := under.Pix[o], under.Pix[o+1], under.Pix[o+2]
			if r > 24 || g > 24 || b < 200 {
				t.Fatalf("emoji drawn before an opaque fill shows through at (%d,%d): %d,%d,%d", x, y, r, g, b)
			}
		}
	}

	over := renderGPU(t, w, h, func(cc *gg.Context) {
		c := p.GPUCanvas(cc)
		c.Clear(RGB(1, 1, 1))
		c.FillRect(box, RGB(0, 0, 1))
		c.TextIn("", emoji, geom.Pt{X: 8, Y: 60}, 40, RGB(0, 0, 0))
	})
	writePNG(t, "gpu_emoji_order.png", over)
	nonBlue := 0
	for y := range h {
		for x := range w {
			o := y*over.Stride + x*4
			if over.Pix[o] > 24 || over.Pix[o+1] > 24 {
				nonBlue++
			}
		}
	}
	if nonBlue < 50 {
		t.Errorf("emoji drawn after an opaque fill is not visible: %d non-blue pixels", nonBlue)
	}
}

// TestGPUManyEmojiStayInOnePass is the regression test for the reason the atlas
// exists. The substrate's image texture cache holds 64 entries; with one image
// per glyph, 65 distinct glyphs on screen push the frame from one render pass
// to two and more than double frame time (measured 1.56 ms to 3.24 ms, and
// 8.5 ms at 256 glyphs). Packed into one sheet it stays at one pass at any
// count.
//
// It fails the moment someone switches the atlas back to one image per glyph,
// which is the single change most likely to be made by accident.
func TestGPUManyEmojiStayInOnePass(t *testing.T) {
	p := emojiPainter(t)
	runes := colorEmojiRunes(p, 80, 20)
	if len(runes) < 40 {
		t.Skipf("capability skip: only %d distinct colour emoji glyphs available, want 40", len(runes))
	}
	const w, h = 600, 400
	p.BeginOffscreen(geom.Size{W: w, H: h}, 1)
	// Warm the atlas: the first frame a glyph appears legitimately re-uploads
	// the sheet, and this measures the steady state.
	draw := func(cc *gg.Context) {
		c := p.GPUCanvas(cc)
		c.Clear(RGB(1, 1, 1))
		x, y := float32(10), float32(30)
		for _, r := range runes {
			c.TextIn("", string(r), geom.Pt{X: x, Y: y}, 20, RGB(0, 0, 0))
			x += 28
			if x > w-30 {
				x, y = 10, y+28
			}
		}
	}
	renderGPU(t, w, h, draw)
	wgpu.ResetEncoderStats()
	img := renderGPU(t, w, h, draw)
	stats := wgpu.EncoderStats()
	writePNG(t, "gpu_emoji_many.png", img)
	t.Logf("%d distinct emoji: render passes=%d draws=%d switches=%d",
		len(runes), stats.RenderPasses, stats.DrawCalls, stats.PipelineSwitches)
	if n := countChromatic(img, img.Bounds()); n < 200 {
		t.Fatalf("the grid drew only %d coloured pixels; the test is vacuous", n)
	}
	if stats.RenderPasses > 1 {
		t.Errorf("%d distinct emoji took %d render passes, want 1: the glyphs are no longer "+
			"coming out of one atlas and the image texture cache is thrashing",
			len(runes), stats.RenderPasses)
	}
	// One sheet, whatever the glyph count.
	if sheet := p.emojiSheet(); sheet == nil {
		t.Error("no atlas sheet after drawing an emoji grid")
	} else {
		t.Logf("one sheet %v holds %d cells for %d glyphs", sheet.Bounds(), len(p.emoji.cells), len(runes))
		if got, want := len(p.emoji.cells), len(runes); got < want {
			t.Errorf("atlas holds %d cells for %d glyphs", got, want)
		}
	}
}

// colorEmojiRunes collects up to n distinct runes that shape to distinct
// colour-bitmap glyphs at the given size.
func colorEmojiRunes(p *Painter, n int, size float32) []rune {
	var out []rune
	seen := map[uint32]bool{}
	for r := rune(0x1F600); r <= 0x1F9FF && len(out) < n; r++ {
		gs := p.ShapeIn("", string(r), size).Glyphs
		if len(gs) != 1 {
			continue
		}
		g := gs[0]
		if g.GID == 0 || seen[g.GID] {
			continue
		}
		if _, ok := p.colorBox(g.Font, g.GID, size); !ok {
			continue
		}
		seen[g.GID] = true
		out = append(out, r)
	}
	return out
}
