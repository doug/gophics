package app

import (
	"image"
	"image/color"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/testfont"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/text"
	"github.com/doug/gophics/widget"
)

// symbolFont is a face covering only ✓ and ⋮ — runes Go Regular lacks — so
// the tests can tell a fallback glyph from the primary's .notdef box.
func symbolFont(t *testing.T) []byte {
	t.Helper()
	data, err := testfont.Remap(goregular.TTF, map[rune]rune{'✓': 'V', '⋮': ':'})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// fontOf returns which parsed face every glyph of s came from in family, and
// fails if the string shaped through more than one face.
func fontOf(t *testing.T, p *paint.Painter, family, s string) (*text.Font, uint32) {
	t.Helper()
	l := p.ShapeIn(family, s, 16)
	if len(l.Glyphs) == 0 {
		t.Fatalf("%q in %q shaped to no glyphs", s, family)
	}
	for _, g := range l.Glyphs[1:] {
		if g.Font != l.Glyphs[0].Font {
			t.Fatalf("%q in %q spans two faces", s, family)
		}
	}
	return l.Glyphs[0].Font, l.Glyphs[0].GID
}

func countInk(img image.Image) int {
	b := img.Bounds()
	n := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if c := color.RGBAModel.Convert(img.At(x, y)).(color.RGBA); c.R < 128 {
				n++
			}
		}
	}
	return n
}

// TestFallbacksResolveRunesThePrimaryLacks: a rune missing from Config.Font
// shapes through Config.Fallbacks rather than as .notdef, and the fallback
// glyph actually reaches the pixels.
func TestFallbacksResolveRunesThePrimaryLacks(t *testing.T) {
	cfg := Config{
		Size:      geom.Size{W: 120, H: 60},
		Font:      goregular.TTF,
		Fallbacks: [][]byte{symbolFont(t)},
	}
	h, err := NewHeadless(widget.Text{Value: "✓", Size: 32, Color: paint.RGB(0, 0, 0)}, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	p := h.Owner().Painter
	primary := p.ShapeIn("", "a", 16).Glyphs[0].Font

	f, gid := fontOf(t, p, "", "✓")
	if f == primary {
		t.Fatal("✓ shaped with the primary font, which has no glyph for it")
	}
	if gid == 0 {
		t.Fatal("✓ shaped to .notdef in the fallback")
	}
	if ink := countInk(h.Render()); ink == 0 {
		t.Fatal("fallback glyph left no ink")
	}

	// Without the fallback the same rune is the primary's .notdef: the
	// control that says the assertion above is measuring the chain.
	cfg.Fallbacks = nil
	h2, err := NewHeadless(widget.Text{Value: "✓"}, cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, gid := fontOf(t, h2.Owner().Painter, "", "✓"); gid != 0 {
		t.Fatalf("without a fallback ✓ shaped to gid %d, want .notdef", gid)
	}
}

// TestNamedFamilyFallsThroughTheChain: a named family lacking a rune takes
// it from the fallbacks, and a family lacking Latin takes that from the
// default font — family, fallbacks, then primary.
func TestNamedFamilyFallsThroughTheChain(t *testing.T) {
	symbols := symbolFont(t)
	h, err := NewHeadless(widget.Text{Value: "✓", Font: "display"}, Config{
		Size: geom.Size{W: 120, H: 60},
		Font: goregular.TTF,
		FontFamilies: map[string][]byte{
			"display": gobold.TTF, // Latin only, like a display serif
			"symbols": symbols,    // no Latin at all
		},
		Fallbacks: [][]byte{symbols},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	p := h.Owner().Painter
	primary, _ := fontOf(t, p, "", "a")
	display, _ := fontOf(t, p, "display", "a")
	fallback, _ := fontOf(t, p, "", "✓")
	if display == primary || fallback == primary || fallback == display {
		t.Fatal("test faces are not distinct")
	}

	if f, gid := fontOf(t, p, "display", "✓"); f != fallback || gid == 0 {
		t.Fatalf("display ✓ came from %p gid %d, want the fallback %p", f, gid, fallback)
	}
	if f, gid := fontOf(t, p, "symbols", "abc"); f != primary || gid == 0 {
		t.Fatalf("symbols abc came from %p gid %d, want the primary %p", f, gid, primary)
	}
	// The symbols family is a second parse of the fallback's bytes, so its
	// own *text.Font is a fourth face — and it, not the fallback, wins for
	// the runes it has.
	if f, gid := fontOf(t, p, "symbols", "✓"); f == fallback || f == primary || f == display || gid == 0 {
		t.Fatal("a family's own face should win for runes it has")
	}
	if ink := countInk(h.Render()); ink == 0 {
		t.Fatal("display-family fallback glyph left no ink")
	}
}

// TestMixedFaceMeasurementIsStable: a string that shapes across two faces
// measures the same every time, the same through the paragraph path, and
// wider than the same string without the fallback-only rune — the widths
// layout keys its caches on cannot wobble with the face a glyph came from.
func TestMixedFaceMeasurementIsStable(t *testing.T) {
	h, err := NewHeadless(widget.Text{Value: "a ✓ b"}, Config{
		Size:      geom.Size{W: 120, H: 60},
		Font:      goregular.TTF,
		Fallbacks: [][]byte{symbolFont(t)},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	p := h.Owner().Painter
	const mixed = "a ✓ b ⋮"
	w := p.MeasureWidthIn("", mixed, 16)
	if w <= 0 {
		t.Fatalf("width %v", w)
	}
	for i := 0; i < 3; i++ {
		h.Render()
		if again := p.MeasureWidthIn("", mixed, 16); again != w {
			t.Fatalf("width drifted %v -> %v after frame %d", w, again, i)
		}
	}
	if plain := p.MeasureWidthIn("", "a  b ", 16); w <= plain {
		t.Fatalf("mixed %v not wider than plain %v: fallback glyphs advanced nothing", w, plain)
	}
	lines := p.ParagraphIn("", mixed, 16, 1e9)
	if len(lines) != 1 || lines[0].Width != w {
		t.Fatalf("paragraph width %+v, line width %v", lines, w)
	}
}
