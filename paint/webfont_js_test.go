//go:build js && wasm

// These run under node via go_js_wasm_exec, like text's own web-font tests:
// UseWebFonts exists only on js, so this is the only configuration that can
// compile it. Nothing here needs the browser — the source is a stub, not a
// fetch — because what is under test is the Painter's half: that the method
// reaches the primary shaper, propagates to the others, and drops the caches
// that would otherwise keep serving the tofu measured before the font landed.
package paint

import (
	"bytes"
	"context"
	"log"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/testfont"
	"github.com/doug/gophics/text"
)

// '✓' is U+2713; Go Regular has no glyph for it, which is what makes it the
// rune to fall back on. text's own tests use the same one for the same reason.
const missingRune = '✓'

// coveringFont is a face whose only glyph is missingRune.
func coveringFont(t *testing.T) []byte {
	t.Helper()
	data, err := testfont.Remap(goregular.TTF, map[rune]rune{missingRune: 'V'})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type stubSource struct {
	data  []byte
	calls int
}

func (s *stubSource) FontForScript(context.Context, text.Script) ([]byte, error) {
	s.calls++
	return s.data, nil
}

// quiet swallows text's "no web font for script" log line so a real failure
// is readable.
func quiet(t *testing.T) {
	t.Helper()
	old := log.Writer()
	log.SetOutput(&bytes.Buffer{})
	t.Cleanup(func() { log.SetOutput(old) })
}

func painterWithFont(t *testing.T) *Painter {
	t.Helper()
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	if f := p.shaperFor("").Primary(); f != nil && f.HasGlyph(missingRune) {
		t.Fatalf("Go Regular grew a glyph for U+%04X; pick another rune", missingRune)
	}
	return p
}

// TestPainterUseWebFontsReachesTheShaper is the plumbing: the method has to
// install the source on the primary shaper, or nothing downstream of it can
// ever fetch anything.
func TestPainterUseWebFontsReachesTheShaper(t *testing.T) {
	quiet(t)
	p := painterWithFont(t)
	src := &stubSource{data: coveringFont(t)}
	landed := make(chan struct{}, 4)
	if err := p.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}

	// The first shaping of an uncovered rune is tofu and must be: shaping is
	// synchronous and the bytes are not there yet. It is also what asks.
	first := p.ShapeIn("", string(missingRune), 16)
	if len(first.Glyphs) != 1 {
		t.Fatalf("shaped %d glyphs, want 1", len(first.Glyphs))
	}
	if first.Glyphs[0].Font.HasGlyph(missingRune) {
		t.Fatal("the first pass already had a covering face")
	}
	<-landed
	if src.calls != 1 {
		t.Fatalf("the source was asked %d times, want 1", src.calls)
	}
}

// TestPainterUseWebFontsInvalidatesTheShapeCache is the failure this method
// exists to prevent. Without it the Painter answers from the line it shaped
// before the font arrived — for ever, because the cache key has no generation
// in it — and the feature looks broken after having actually worked.
func TestPainterUseWebFontsInvalidatesTheShapeCache(t *testing.T) {
	quiet(t)
	p := painterWithFont(t)

	// Measure first, so there is a memoized tofu answer to go stale. This is
	// also why UseWebFonts clears at registration: a cache hit never reaches
	// the shaper, so without that clear the fetch below would never start.
	p.MeasureWidthIn("", string(missingRune), 16)
	if len(p.shapes) == 0 {
		t.Fatal("the shape cache did not memoize the first measurement")
	}

	src := &stubSource{data: coveringFont(t)}
	landed := make(chan struct{}, 4)
	gen := p.ShapeGen()
	if err := p.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if len(p.shapes) != 0 {
		t.Fatal("registering a web font source left the stale measurement cached")
	}
	if p.ShapeGen() == gen {
		t.Fatal("ShapeGen did not advance, so caches outside the Painter keep theirs")
	}

	p.ShapeIn("", string(missingRune), 16) // tofu again, and starts the fetch
	// The raw field, not ShapeGen(): ShapeGen is itself one of the calls that
	// applies a pending landing, so reading it here would consume the news
	// this half of the test is waiting to observe.
	gen = p.shapeGen
	<-landed

	line := p.ShapeIn("", string(missingRune), 16)
	if len(line.Glyphs) != 1 {
		t.Fatalf("shaped %d glyphs, want 1", len(line.Glyphs))
	}
	if !line.Glyphs[0].Font.HasGlyph(missingRune) {
		t.Fatal("the Painter served the line it shaped before the font landed")
	}
	if p.shapeGen == gen {
		t.Fatal("a landed font did not advance ShapeGen")
	}
}

// TestPainterUseWebFontsInvalidatesTheRunCache is the same claim one layer
// down, in pixels, and against the cache that hurts most: p.runs holds
// *rasterized* glyph images keyed by (font, string, size, colour, scale) with
// no generation in the key, so a stale entry is not a stale number but the
// tofu bitmap itself, blitted for as long as the entry lives.
func TestPainterUseWebFontsInvalidatesTheRunCache(t *testing.T) {
	quiet(t)
	p := painterWithFont(t)

	drawn := func() []byte {
		c := p.BeginOffscreen(geom.Size{W: 40, H: 40}, 1)
		c.FillRect(geom.RectXYWH(0, 0, 40, 40), RGB(1, 1, 1))
		c.TextIn("", string(missingRune), geom.Pt{X: 6, Y: 28}, 24, RGB(0, 0, 0))
		img := p.SurfaceRGBA()
		out := make([]byte, len(img.Pix))
		copy(out, img.Pix)
		return out
	}

	tofu := drawn()

	src := &stubSource{data: coveringFont(t)}
	landed := make(chan struct{}, 4)
	if err := p.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if again := drawn(); !bytes.Equal(tofu, again) {
		t.Fatal("the frame before the font landed should still be tofu")
	}
	<-landed

	if covered := drawn(); bytes.Equal(tofu, covered) {
		t.Fatal("the Painter blitted the tofu it rasterized before the font " +
			"landed; the run cache was not invalidated")
	}
}

// TestPainterUseWebFontsPropagatesToEveryFamily mirrors LoadSystemFonts: a
// named family must resolve through the same fetched faces, including a family
// registered after the call — LoadFontFamily hands a new shaper the default's
// map, and this checks that it is the web-font map by then.
func TestPainterUseWebFontsPropagatesToEveryFamily(t *testing.T) {
	quiet(t)
	p := painterWithFont(t)
	if err := p.LoadFontFamily("before", goregular.TTF); err != nil {
		t.Fatal(err)
	}

	src := &stubSource{data: coveringFont(t)}
	landed := make(chan struct{}, 8)
	if err := p.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	if err := p.LoadFontFamily("after", goregular.TTF); err != nil {
		t.Fatal(err)
	}

	p.ShapeIn("", string(missingRune), 16) // tofu; asks once for the script
	<-landed

	for _, family := range []string{"", "before", "after"} {
		line := p.ShapeIn(family, string(missingRune), 16)
		if len(line.Glyphs) != 1 {
			t.Fatalf("family %q shaped %d glyphs, want 1", family, len(line.Glyphs))
		}
		if !line.Glyphs[0].Font.HasGlyph(missingRune) {
			t.Errorf("family %q did not reach the fetched face", family)
		}
	}
	// One shared map means one fetch for the script, not one per family.
	if src.calls != 1 {
		t.Errorf("the source was asked %d times, want 1 for three families", src.calls)
	}
}

// TestPainterUseWebFontsRejectsANilSource: the error comes from text, and this
// checks the Painter passes it back rather than reporting success — the
// capability rule in AGENTS.md, applied to a configuration mistake.
func TestPainterUseWebFontsRejectsANilSource(t *testing.T) {
	if err := painterWithFont(t).UseWebFonts(context.Background(), nil, nil); err == nil {
		t.Fatal("a nil WebFontSource was accepted")
	}
}

// TestPainterUseWebFontsAcceptsANilNotify: a caller with nothing to invalidate
// outside the Painter still gets the Painter's own caches dropped, so the next
// frame that reshapes is correct even though nothing asked for one.
func TestPainterUseWebFontsAcceptsANilNotify(t *testing.T) {
	quiet(t)
	p := painterWithFont(t)
	src := &stubSource{data: coveringFont(t)}
	if err := p.UseWebFonts(context.Background(), src, nil); err != nil {
		t.Fatal(err)
	}
	p.ShapeIn("", string(missingRune), 16) // starts the fetch
	for range 500 {
		if line := p.ShapeIn("", string(missingRune), 16); len(line.Glyphs) == 1 &&
			line.Glyphs[0].Font.HasGlyph(missingRune) {
			return
		}
		// Yield to the fetch goroutine. There is no notify to wait on, which
		// is the configuration under test: the face still installs, and the
		// next reshape picks it up.
		time.Sleep(time.Millisecond)
	}
	t.Fatal("the font never reached a reshaped line with a nil notify")
}
