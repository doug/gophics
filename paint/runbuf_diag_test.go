//go:build gossamer_gpu

package paint

// Regression guard for the "text renders as a solid block" bug. It reproduces
// the trigger without needing a GPU device: importing gg's GPU accelerator (via
// the gossamer_gpu tag) registers a global tile-based CoverageFiller, and gg's
// RasterizerAuto routes complex multi-contour paths — like a glyph run — to it.
// That filler mishandles multi-contour winding, filling the gaps between glyphs
// solid. runFor pins the glyph scratch context to RasterizerAnalytic to avoid
// it; this test fails if that pin is ever removed.

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

func TestGlyphRunNotBlock(t *testing.T) {
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatalf("font: %v", err)
	}
	// A dense run of narrow strokes with wide transparent gaps between them —
	// the worst case for the tile filler's winding bug.
	run := p.runFor("", "IIIIIIIIII", 48, RGB(0, 0, 0))
	if run == nil {
		t.Fatal("nil run")
	}
	src := run.buf.ToStdImage()
	b := src.Bounds()

	var opaque int
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if _, _, _, a := src.At(x, y).RGBA(); a>>8 >= 250 {
				opaque++
			}
		}
	}
	frac := float64(opaque) / float64(b.Dx()*b.Dy())
	t.Logf("glyph run %dx%d: %.1f%% opaque", b.Dx(), b.Dy(), frac*100)
	// Ten thin vertical strokes cover only a small fraction of their bounding
	// box; a block would be ~50%+. Anything above 30% means the gaps filled in.
	if frac > 0.30 {
		t.Errorf("glyph run rasterized as a near-solid block: %.1f%% opaque", frac*100)
	}
}
