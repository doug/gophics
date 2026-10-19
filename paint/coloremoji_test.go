package paint

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/text"
)

const emoji = "\U0001F600" // GRINNING FACE

// emojiPainter returns a Painter whose chain reaches a colour-bitmap emoji
// face, or capability-skips. The skip is a capability skip, not a pass: a CI
// container has no system fonts, and Windows' system emoji font is COLR, which
// this route deliberately does not render (see text/coloremoji.go).
func emojiPainter(t *testing.T) *Painter {
	t.Helper()
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	if err := p.LoadSystemFonts(); err != nil {
		t.Skipf("capability skip: no system fonts on this machine: %v", err)
	}
	if !hasColorEmoji(p) {
		t.Skip("capability skip: no colour-bitmap emoji font installed " +
			"(a COLR-only font such as Segoe UI Emoji is expected to land here)")
	}
	return p
}

func hasColorEmoji(p *Painter) bool {
	for _, g := range p.ShapeIn("", emoji, 32).Glyphs {
		if _, ok := p.colorBox(g.Font, g.GID, 32); ok {
			return true
		}
	}
	return false
}

// TestInkBoundsCoversEmoji is the quiet failure from the plan, and the one that
// made every other fix invisible on the CPU path.
//
// Two separate things break if ink bounds ignore a bitmap glyph. A mixed run
// under-reports its top, because an emoji's box is a full em against a Latin
// ink box of about 0.75 em, so a partial repaint leaves a stale band across the
// emoji. And a label whose text is *only* an emoji has no outlines at all, so
// InkBoundsIn reported ok=false, runFor returned nil, and the label drew
// nothing at all with no error anywhere.
func TestInkBoundsCoversEmoji(t *testing.T) {
	p := emojiPainter(t)

	plain, ok := p.InkBoundsIn("", "aa", 20)
	if !ok {
		t.Fatal(`InkBoundsIn("aa") reported no ink`)
	}
	mixed, ok := p.InkBoundsIn("", "a"+emoji+"a", 20)
	if !ok {
		t.Fatal("InkBoundsIn of a mixed run reported no ink")
	}
	t.Logf("plain=%+v mixed=%+v", plain, mixed)
	if mixed.Dy() <= plain.Dy() {
		t.Errorf("mixed run ink height %g not taller than plain %g: the emoji contributes no extent",
			mixed.Dy(), plain.Dy())
	}
	if mixed.Min.Y >= plain.Min.Y {
		t.Errorf("mixed run ink top %g is no higher than plain %g", mixed.Min.Y, plain.Min.Y)
	}

	// The emoji-only label: the silent one.
	only, ok := p.InkBoundsIn("", emoji, 20)
	if !ok {
		t.Fatal("InkBoundsIn of an emoji-only string reported no ink: " +
			"the run cache will return nil and the label will draw nothing")
	}
	if only.Dx() <= 0 || only.Dy() <= 0 {
		t.Errorf("emoji-only ink bounds have no extent: %+v", only)
	}
}

// renderCPU draws into an offscreen CPU surface and returns the pixels.
func renderCPU(t *testing.T, p *Painter, w, h int, scale float32, draw func(Canvas)) *image.RGBA {
	t.Helper()
	c := p.BeginOffscreen(geom.Size{W: float32(w), H: float32(h)}, scale)
	draw(c)
	img := p.SurfaceRGBA()
	if img == nil {
		t.Fatal("no surface")
	}
	return img
}

// countChromatic counts pixels whose channels are not all equal — pixels that
// no monochrome text fill could have produced over a grey background. This is
// the assertion that matters: "no error was returned" is what the bug looked
// like before, so the test has to find colour on screen.
func countChromatic(img *image.RGBA, r image.Rectangle) int {
	n := 0
	r = r.Intersect(img.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			o := y*img.Stride + x*4
			cr, cg, cb := int(img.Pix[o]), int(img.Pix[o+1]), int(img.Pix[o+2])
			if max(cr, max(cg, cb))-min(cr, min(cg, cb)) > 24 {
				n++
			}
		}
	}
	return n
}

// TestEmojiOnlyLabelDrawsPixels is the mandatory case: the one that used to
// fail silently. A label whose entire text is an emoji must put colour on the
// surface, at 1x and at 2x — the scale matters because the atlas cell is
// physical pixels and the destination is logical, and getting that backwards
// is invisible at 1x.
func TestEmojiOnlyLabelDrawsPixels(t *testing.T) {
	for _, scale := range []float32{1, 2} {
		p := emojiPainter(t)
		const w, h = 64, 64
		img := renderCPU(t, p, w, h, scale, func(c Canvas) {
			c.Clear(RGB(0.5, 0.5, 0.5))
			c.TextIn("", emoji, geom.Pt{X: 8, Y: 44}, 32, RGB(0, 0, 0))
		})
		n := countChromatic(img, img.Bounds())
		t.Logf("scale=%g: %d chromatic pixels of %d", scale, n, w*h*int(scale*scale))
		if n < 50 {
			t.Errorf("scale=%g: emoji-only label drew %d coloured pixels, want many: "+
				"the emoji is invisible", scale, n)
		}
	}
}

// TestMixedRunDrawsEmojiAndText checks that decomposing a run does not lose
// either half: the Latin glyphs still ink, and the emoji still blits.
func TestMixedRunDrawsEmojiAndText(t *testing.T) {
	p := emojiPainter(t)
	const w, h = 220, 64
	img := renderCPU(t, p, w, h, 1, func(c Canvas) {
		c.Clear(RGB(1, 1, 1))
		c.TextIn("", "hi "+emoji+" there", geom.Pt{X: 4, Y: 44}, 24, RGB(0, 0, 0))
	})
	chroma := countChromatic(img, img.Bounds())
	if chroma < 30 {
		t.Errorf("mixed run: %d coloured pixels, want many (the emoji did not draw)", chroma)
	}
	// The Latin must still be there. Count near-black pixels; a chromatic
	// emoji pixel is not near-black.
	dark := 0
	for y := range h {
		for x := range w {
			o := y*img.Stride + x*4
			if int(img.Pix[o])+int(img.Pix[o+1])+int(img.Pix[o+2]) < 200 {
				dark++
			}
		}
	}
	t.Logf("mixed run: %d chromatic, %d dark", chroma, dark)
	if dark < 30 {
		t.Errorf("mixed run: %d dark pixels, want many (the Latin spans did not draw)", dark)
	}
}

// TestEmojiAtlasPacksOneSheet pins the design constraint the measurement
// bought: every glyph at every size comes out of a single image value. One
// image per glyph costs an extra render pass and 2.2x the frame time past 65
// distinct glyphs, because the substrate's image texture cache holds 64.
func TestEmojiAtlasPacksOneSheet(t *testing.T) {
	p := emojiPainter(t)
	var g text.Glyph
	for _, cand := range p.ShapeIn("", emoji, 32).Glyphs {
		if _, ok := p.colorBox(cand.Font, cand.GID, 32); ok {
			g = cand
		}
	}
	if g.Font == nil {
		t.Fatal("no colour glyph")
	}
	seen := map[image.Rectangle]bool{}
	// Many sizes, so the shelf allocator has to wrap rows and the sheet has to
	// grow at least once.
	for px := 16; px <= 160; px += 2 {
		ppem := g.Font.ColorGlyphPpem(px)
		cell, ok := p.emojiCell(g.Font, g.GID, ppem)
		if !ok {
			t.Fatalf("px=%d ppem=%d: no cell", px, ppem)
		}
		if cell.Empty() {
			t.Fatalf("px=%d: empty cell", px)
		}
		sheet := p.emojiSheet()
		if sheet == nil {
			t.Fatal("no sheet")
		}
		if !cell.In(sheet.Bounds()) {
			t.Fatalf("px=%d: cell %v outside sheet %v", px, cell, sheet.Bounds())
		}
		seen[cell] = true
	}
	// Distinct strikes get distinct cells; sizes landing on one strike share.
	strikes := map[int]bool{}
	for px := 16; px <= 160; px += 2 {
		strikes[g.Font.ColorGlyphPpem(px)] = true
	}
	if len(seen) != len(strikes) {
		t.Errorf("%d distinct cells for %d distinct strikes: quantisation is not collapsing sizes",
			len(seen), len(strikes))
	}
	t.Logf("%d strikes, %d cells, sheet %v", len(strikes), len(seen), p.emojiSheet().Bounds())

	// Cells must not overlap: a shelf allocator bug shows up as two glyphs
	// drawing each other's pixels, which no pixel test of one glyph catches.
	list := make([]image.Rectangle, 0, len(seen))
	for r := range seen {
		list = append(list, r)
	}
	for i := range list {
		for j := i + 1; j < len(list); j++ {
			if list[i].Overlaps(list[j]) {
				t.Errorf("cells overlap: %v and %v", list[i], list[j])
			}
		}
	}
}

// TestEmojiAtlasGrowReplacesTheImageValue pins the contract that makes a grow
// safe. Every cache above the atlas keys images by pointer identity, so a grown
// sheet must be a new value — and the recorded cell rectangles must survive it,
// which doubling guarantees because it preserves every cell's origin.
func TestEmojiAtlasGrowReplacesTheImageValue(t *testing.T) {
	p := NewPainter()
	a := p.ensureEmojiAtlas()
	first := a.img
	cell, ok := a.alloc(40, 40, p)
	if !ok {
		t.Fatal("alloc")
	}
	// Fill the sheet past its initial size.
	for range (emojiAtlasInit / 41) * (emojiAtlasInit/41 + 2) {
		if _, ok := a.alloc(40, 40, p); !ok {
			t.Fatal("alloc during fill")
		}
	}
	if a.img == first {
		t.Fatal("the sheet never grew; the test no longer exercises a grow")
	}
	if a.img.Bounds().Dx() <= first.Bounds().Dx() {
		t.Errorf("grown sheet %v is not larger than %v", a.img.Bounds(), first.Bounds())
	}
	if !cell.In(a.img.Bounds()) {
		t.Errorf("cell %v allocated before the grow is outside the grown sheet %v", cell, a.img.Bounds())
	}
}

// TestColorBoxRejectsOutlineGlyphs guards the partition predicate. If an
// ordinary glyph were classified as a colour bitmap it would be routed to the
// atlas, find no bitmap, and draw nothing — i.e. all text would vanish.
func TestColorBoxRejectsOutlineGlyphs(t *testing.T) {
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}
	for _, g := range p.ShapeIn("", "Hello, world! 123", 18).Glyphs {
		if _, ok := p.colorBox(g.Font, g.GID, 18); ok {
			t.Fatalf("gid %d of an outline-only font classified as a colour bitmap", g.GID)
		}
	}
	// And plain text still draws, decomposition or not.
	img := renderCPU(t, p, 160, 40, 1, func(c Canvas) {
		c.Clear(RGB(1, 1, 1))
		c.TextIn("", "Hello", geom.Pt{X: 4, Y: 28}, 20, RGB(0, 0, 0))
	})
	dark := 0
	for y := range 40 {
		for x := range 160 {
			o := y*img.Stride + x*4
			if int(img.Pix[o])+int(img.Pix[o+1])+int(img.Pix[o+2]) < 200 {
				dark++
			}
		}
	}
	if dark < 30 {
		t.Errorf("plain text drew %d dark pixels", dark)
	}
}
