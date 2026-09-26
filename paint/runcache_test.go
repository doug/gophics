package paint

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
)

// fillOutlines draws s by filling its glyph outlines straight onto the
// surface — the resolution-independent ground truth the run cache is an
// optimization of.
func fillOutlines(p *Painter, c Canvas, font, s string, pos geom.Pt, size float32) {
	cc := c.(*ggCanvas)
	line := p.ShapeIn(font, s, size)
	cc.dc.SetColor(color.NRGBA{A: 255})
	cc.dc.ClearPath()
	for _, g := range line.Glyphs {
		g.Font.AppendGlyphPath(ggSink{cc.dc}, g.GID, size, pos.X+g.X, pos.Y+g.Y)
	}
	cc.dc.Fill()
}

// inkOf sums the darkness of a white surface: how much text is on it.
func inkOf(img *image.RGBA) int {
	n := 0
	for i := 0; i < len(img.Pix); i += 4 {
		n += 255 - int(img.Pix[i])
	}
	return n
}

// renderText draws s at pos through the run cache (cached) or as a direct
// outline fill (direct), on a white surface.
func renderText(t *testing.T, fonts [][]byte, font, s string, pos geom.Pt, size, scale float32, cached bool) *image.RGBA {
	t.Helper()
	p := NewPainter()
	for i, data := range fonts {
		name := ""
		if i > 0 {
			name = font
		}
		if err := p.LoadFontFamily(name, data); err != nil {
			t.Fatal(err)
		}
	}
	c := p.BeginOffscreen(geom.Size{W: 220, H: 100}, scale)
	c.Clear(RGB(1, 1, 1))
	if cached {
		c.TextIn(font, s, pos, size, RGB(0, 0, 0))
	} else {
		fillOutlines(p, c, font, s, pos, size)
	}
	return p.SurfaceRGBA()
}

// The run cache used to rasterize each run into an image sized from the
// primary font's ascent and descent by the advance width, so anything outside
// that box — an accent above the ascender, an italic overhang past the last
// advance — was clipped off. The image is sized from the run's ink now, and
// this checks that no ink goes missing against a direct outline fill.
func TestRunCacheKeepsInkOutsideTheMetricsBox(t *testing.T) {
	for _, tc := range []struct {
		name  string
		fonts [][]byte
		font  string
		s     string
	}{
		{"accents above the ascender", [][]byte{goregular.TTF}, "", "ÅÊÎÕ Ǻ"},
		{"italic overhang", [][]byte{goregular.TTF, goitalic.TTF}, "italic", "ffff"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pos := geom.Pt{X: 20, Y: 60}
			direct := inkOf(renderText(t, tc.fonts, tc.font, tc.s, pos, 40, 1, false))
			cached := inkOf(renderText(t, tc.fonts, tc.font, tc.s, pos, 40, 1, true))
			if direct == 0 {
				t.Fatal("the direct fill drew nothing; the test measures nothing")
			}
			if lost := direct - cached; lost > direct/100 {
				t.Errorf("run cache lost %d of %d ink units (%.1f%%) against a direct fill", lost, direct, 100*float64(lost)/float64(direct))
			}
		})
	}
}

// A run whose position is on the device grid must blit at an integer device
// offset: the cached image's baseline sits on a whole pixel row, so the blit
// copies pixels instead of resampling them at a fractional phase. Resampled
// text is visibly softer than the direct fill it is supposed to equal.
func TestRunCacheBlitsOnTheDeviceGridUnresampled(t *testing.T) {
	for _, tc := range []struct{ size, scale float32 }{{14, 1}, {20, 1}, {40, 2}} {
		pos := geom.Pt{X: 20, Y: 50}
		direct := renderText(t, [][]byte{goregular.TTF}, "", "Hello xyz", pos, tc.size, tc.scale, false)
		cached := renderText(t, [][]byte{goregular.TTF}, "", "Hello xyz", pos, tc.size, tc.scale, true)
		differ, inked, maxd := 0, 0, 0
		for i := 0; i < len(direct.Pix); i += 4 {
			if direct.Pix[i] < 250 {
				inked++
			}
			d := int(direct.Pix[i]) - int(cached.Pix[i])
			if d < 0 {
				d = -d
			}
			if d > 8 {
				differ++
			}
			maxd = max(maxd, d)
		}
		// At scale 1 the two are the same pixels. At scale 2 the outlines are
		// pre-scaled for the cache and CTM-scaled for the direct fill, which
		// flattens the curves differently: a few edge pixels shift by a
		// level or two. Resampling at a fractional phase touched every edge
		// pixel of every glyph (hundreds at this size) by up to half the range.
		if differ > inked/50 || maxd > 40 {
			t.Errorf("size %v scale %v: %d of %d inked pixels differ by more than 8 (max %d) between the cached run and a direct fill", tc.size, tc.scale, differ, inked, maxd)
		}
	}
}

// Whitespace has no ink, so there is no image to cache or blit.
func TestRunCacheSkipsRunsWithoutInk(t *testing.T) {
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	p.BeginOffscreen(geom.Size{W: 10, H: 10}, 1)
	if r := p.runFor("", "   ", 14, RGB(0, 0, 0)); r != nil {
		t.Errorf("a run of spaces was rasterized into a %vx%v image", r.dstW, r.dstH)
	}
	if _, ok := p.InkBoundsIn("", "   ", 14); ok {
		t.Error("spaces reported ink bounds")
	}
	if r, ok := p.InkBoundsIn("", "Ǻ", 40); !ok || r.Min.Y >= -p.MetricsIn("", 40).Ascent {
		t.Errorf("Ǻ at 40: ink %v (ok=%v) should rise above the ascender %v", r, ok, p.MetricsIn("", 40).Ascent)
	}
}
