package paint

import (
	"image"
	"image/draw"
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/text"
)

// Colour emoji, drawn as blits from one shared atlas.
//
// A colour emoji carries no outline, so the outline paths that draw every other
// glyph draw nothing for it. The fix decomposes a text run at replay time: the
// outline glyphs keep their existing path, and each colour glyph is blitted
// from a sheet of decoded bitmaps through the ordinary image primitive, in
// glyph order, so the two interleave correctly against each other and against
// every other shape in the frame.
//
// # Why one sheet and not one image per glyph
//
// Measured on this tree (Apple M1 Ultra, Metal): the GPU substrate's image
// texture cache holds 64 entries. With 64 distinct images a frame renders in
// one render pass; with 65 it takes two and frame time more than doubles
// (1.56 ms to 3.24 ms), because the cache evicts every frame and never warms.
// At 256 distinct images it is 8.5 ms — a dropped frame at 120 Hz. Packed into
// one sheet the same 256 glyphs stay at one pass and 3.2 ms. An emoji picker is
// exactly the screen that crosses 64, so the sheet is a design constraint
// rather than a tuning knob: whatever the glyph count, the whole sheet is one
// cache entry.
//
// # Why nothing is evicted
//
// Deliberately. An app's emoji working set is bounded by its content, and
// partial eviction cannot be expressed in a packed sheet anyway — freeing one
// cell leaves a hole no later cell is shaped to fill. The sheet grows by
// doubling and, at the cap, is thrown away whole and rebuilt (see grow). That
// is a worse frame once, against an LRU that would have to repack and
// re-upload the sheet to free anything at all.
//
// # Physical pixels
//
// Cells are physical (device) pixels and are keyed on the ppem they were
// decoded at, not on the logical size: two logical sizes that land on the same
// bitmap strike share one cell, and the same logical size at 1x and 2x does
// not. The destination rectangle handed to DrawSprite is logical, as every
// Canvas coordinate is. Getting this backwards is invisible at 1x.

// emojiKey identifies one packed cell. ppem is in device pixels and is already
// quantised to a strike the font has (text.Font.ColorGlyphPpem), so a smooth
// zoom reuses cells instead of minting one per frame.
type emojiKey struct {
	font *text.Font
	gid  uint32
	ppem int
}

// emojiAtlas is the packed sheet. img is one image value with a stable
// identity: the painter's own texture cache, the scene differ and the
// substrate's image cache all key images by pointer identity, so replacing the
// value on a grow invalidates every one of them at once, which is exactly what
// a grow needs.
type emojiAtlas struct {
	img   *image.RGBA
	cells map[emojiKey]image.Rectangle
	// Shelf allocator: cells advance along the current row and the row
	// advances by the tallest cell in it. Emoji cells are square and uniform
	// per size, so shelf packing wastes very little for twenty lines of code.
	x, y, rowH int
}

const (
	// emojiAtlasInit is the starting sheet edge in device pixels. 512 holds
	// about 150 cells at a 40-ppem strike, which covers an ordinary screenful
	// of chat without a grow.
	emojiAtlasInit = 512
	// emojiAtlasMax caps the sheet at 16 MB of RGBA. Past it the sheet is
	// rebuilt from empty rather than grown, because a 4096-edge sheet is 64 MB
	// of resident pixels plus a GPU copy, which is more than a glyph cache
	// should ever cost.
	emojiAtlasMax = 2048
	// emojiCellPad separates cells by a transparent pixel. The image pipeline
	// samples bilinearly with no control over the filter, so a cell abutting
	// its neighbour bleeds that neighbour's edge in at a fractional
	// destination position.
	emojiCellPad = 1
)

// colorGlyphKey caches the answer to "is this glyph a colour bitmap, and where
// does it paint" per (font, gid, logical size).
type colorGlyphKey struct {
	font *text.Font
	gid  uint32
	size float32
}

type colorGlyphBox struct {
	r  geom.Rect
	ok bool
}

// colorBoxLimit bounds the predicate cache. It is generous because the entries
// are tiny and the negative answers are the ones that matter: the run
// partitioning below asks about every glyph of every run on every frame on the
// GPU path, so "this is ordinary Latin" has to be a map hit.
const colorBoxLimit = 1 << 13

// colorBox reports whether (font, gid) is a colour bitmap glyph and, if so,
// the box it paints in at the given logical size, relative to the pen position
// on the baseline (y down, so the top is negative).
//
// Memoized because the no is the hot answer. text.Font answers it by asking the
// COLR and bitmap tables directly, which costs almost nothing for a Latin face
// that has neither — but "almost nothing" times every glyph of every run of
// every frame is still worth a map.
func (p *Painter) colorBox(f *text.Font, gid uint32, size float32) (geom.Rect, bool) {
	if f == nil {
		return geom.Rect{}, false
	}
	k := colorGlyphKey{f, gid, size}
	if b, ok := p.colorBoxes[k]; ok {
		return b.r, b.ok
	}
	if len(p.colorBoxes) >= colorBoxLimit {
		evictHalf(p.colorBoxes)
	}
	r, ok := f.ColorGlyphBox(gid, size)
	p.colorBoxes[k] = colorGlyphBox{r, ok}
	return r, ok
}

// emojiPpem quantises a logical size at a device scale to a bitmap strike the
// font actually carries. Zero means the font has no strikes, so there is
// nothing to blit.
func emojiPpem(f *text.Font, size, scale float32) int {
	if scale <= 0 {
		scale = 1
	}
	px := int(math.Round(float64(size * scale)))
	if px < 1 {
		px = 1
	}
	return f.ColorGlyphPpem(px)
}

// emojiCell returns the sheet region holding (font, gid) decoded at ppem
// device pixels, decoding and packing it on first use. ok is false when the
// glyph has no bitmap this can decode.
//
// The sheet it indexes into is emojiSheet(); call that *after* this, because a
// pack may have replaced the image value.
func (p *Painter) emojiCell(f *text.Font, gid uint32, ppem int) (image.Rectangle, bool) {
	if f == nil || ppem <= 0 {
		return image.Rectangle{}, false
	}
	k := emojiKey{f, gid, ppem}
	if p.emoji != nil {
		if r, ok := p.emoji.cells[k]; ok {
			// An empty rect is the remembered failure: a glyph whose bitmap
			// would not decode must not be re-decoded on every frame.
			return r, !r.Empty()
		}
	}
	img, ok := f.ColorGlyphImage(gid, ppem)
	if !ok {
		p.rememberEmojiMiss(k)
		return image.Rectangle{}, false
	}
	b := img.Bounds()
	if b.Empty() {
		p.rememberEmojiMiss(k)
		return image.Rectangle{}, false
	}
	a := p.ensureEmojiAtlas()
	cell, ok := a.alloc(b.Dx(), b.Dy(), p)
	if !ok {
		p.rememberEmojiMiss(k)
		return image.Rectangle{}, false
	}
	draw.Draw(a.img, cell, img, b.Min, draw.Src)
	a.cells[k] = cell
	// The sheet's pixels have changed under a stable image value, which is
	// precisely the case ImageChanged exists for: without this the cached
	// texture still holds the sheet as it was before this glyph was packed, so
	// the new glyph draws as whatever was in that cell before — transparent,
	// i.e. nothing. The cost is re-uploading the whole sheet, and it is paid
	// only on the frame a glyph first appears.
	p.ImageChanged(a.img)
	return cell, true
}

// rememberEmojiMiss records a glyph whose bitmap could not be packed, so the
// attempt is made once rather than once per frame.
func (p *Painter) rememberEmojiMiss(k emojiKey) {
	p.ensureEmojiAtlas().cells[k] = image.Rectangle{}
}

// emojiSheet is the atlas image to pass as DrawSprite's atlas, or nil when
// nothing has been packed.
func (p *Painter) emojiSheet() image.Image {
	if p.emoji == nil {
		return nil
	}
	return p.emoji.img
}

func (p *Painter) ensureEmojiAtlas() *emojiAtlas {
	if p.emoji == nil {
		p.emoji = &emojiAtlas{
			img:   image.NewRGBA(image.Rect(0, 0, emojiAtlasInit, emojiAtlasInit)),
			cells: map[emojiKey]image.Rectangle{},
		}
	}
	return p.emoji
}

// alloc reserves a w×h region, growing or resetting the sheet when the current
// one cannot hold it. It returns the region in sheet coordinates.
func (a *emojiAtlas) alloc(w, h int, p *Painter) (image.Rectangle, bool) {
	cw, ch := w+emojiCellPad, h+emojiCellPad
	for {
		sheet := a.img.Bounds()
		if cw > sheet.Dx() || ch > sheet.Dy() {
			if !a.grow(cw, ch, p) {
				return image.Rectangle{}, false
			}
			continue
		}
		if a.x+cw > sheet.Dx() {
			a.x = 0
			a.y += a.rowH
			a.rowH = 0
		}
		if a.y+ch > sheet.Dy() {
			if !a.grow(cw, ch, p) {
				return image.Rectangle{}, false
			}
			continue
		}
		r := image.Rect(a.x, a.y, a.x+w, a.y+h)
		a.x += cw
		a.rowH = max(a.rowH, ch)
		return r, true
	}
}

// grow doubles the sheet, copying the packed cells so their recorded rectangles
// stay valid — doubling preserves every cell's origin, which is why the cells
// map survives a grow untouched.
//
// Past emojiAtlasMax it instead discards everything and starts again at the
// initial size. That is a deliberate cliff, not an oversight: see the "why
// nothing is evicted" note above. It reports false only when even an empty
// sheet at the cap could not hold the cell, which would take a single glyph
// wider than 2048 device pixels.
func (a *emojiAtlas) grow(needW, needH int, p *Painter) bool {
	old := a.img
	w, h := old.Bounds().Dx(), old.Bounds().Dy()
	nw, nh := w, h
	for nw < needW || nh < needH || a.y+needH > nh {
		if nw >= emojiAtlasMax && nh >= emojiAtlasMax {
			if needW > emojiAtlasMax || needH > emojiAtlasMax {
				return false
			}
			a.img = image.NewRGBA(image.Rect(0, 0, emojiAtlasInit, emojiAtlasInit))
			clear(a.cells)
			a.x, a.y, a.rowH = 0, 0, 0
			p.ImageChanged(old)
			return true
		}
		nw = min(nw*2, emojiAtlasMax)
		nh = min(nh*2, emojiAtlasMax)
	}
	grown := image.NewRGBA(image.Rect(0, 0, nw, nh))
	draw.Draw(grown, old.Bounds(), old, old.Bounds().Min, draw.Src)
	a.img = grown
	// The old value is dead. Drop its cached texture rather than leaving a
	// sheet-sized buffer pinned in the painter's map until it happens to be
	// evicted.
	p.ImageChanged(old)
	return true
}

// drawColorGlyph blits one colour glyph. box is the glyph's placement relative
// to the pen, pos is the line origin (baseline) and g carries the glyph's
// offset within the line — all logical pixels; src is the sheet region, in
// device pixels.
func (c *ggCanvas) drawColorGlyph(src image.Rectangle, box geom.Rect, pos geom.Pt, g text.Glyph) {
	sheet := c.p.emojiSheet()
	if sheet == nil {
		return
	}
	// Tint stays zero: a colour glyph carries its own colours and multiplying
	// them by the text colour would turn every emoji into a silhouette. The
	// text colour is simply not part of this glyph.
	c.DrawSprite(sheet, Sprite{
		Src: src,
		Dst: box.Translate(geom.Pt{X: pos.X + g.X, Y: pos.Y + g.Y}),
	})
}

// EmojiReporter answers, before anything is drawn, whether colour emoji will
// actually appear — the one question a Measurer cannot. Ink bounds report that
// *something* paints, and a missing-glyph box paints too, so measuring is no
// way to tell a grinning face from a tofu rectangle.
//
// It is an opt-in capability because the answer is a property of the font chain
// behind the canvas, which not every Canvas has.
type EmojiReporter interface {
	// HasColorEmoji reports whether s, shaped at size through the named family
	// ("" = the default family) and its fallbacks, reaches at least one colour
	// glyph this canvas can draw.
	//
	// False is the answer to act on rather than a diagnosis: it covers a build
	// with no emoji face at all, a web build with no system fonts, and a
	// Windows one whose system emoji font is COLR — which shapes perfectly and
	// draws nothing (see the package note above). In all three the caller
	// should show something other than an emoji.
	HasColorEmoji(font, s string, size float32) bool
}

// Emoji returns c's colour-emoji capability, or nil when c cannot answer at
// all. The painter canvas, the GPU canvas over it, and the scene recorder a
// widget's Draw is handed can all answer.
//
//	if e := paint.Emoji(c); e != nil && e.HasColorEmoji("", s, 48) {
//		c.TextIn("", s, at, 48, col)
//	} else {
//		c.TextIn("", "no colour emoji face on this build", at, 13, muted)
//	}
//
// Treat nil and false the same way at the call site: both mean "do not draw an
// emoji here". They are distinct only in why, and only a canvas from outside
// this module answers nil.
func Emoji(c Canvas) EmojiReporter {
	e, _ := c.(EmojiReporter)
	return e
}

// HasColorEmoji implements EmojiReporter. It shapes s rather than inspecting
// the face list because fallback is per-rune: whether an emoji reaches a colour
// face is decided during shaping, by the same resolution that will run again
// when the run is drawn.
func (p *Painter) HasColorEmoji(font, s string, size float32) bool {
	for _, g := range p.ShapeIn(font, s, size).Glyphs {
		if _, ok := p.colorBox(g.Font, g.GID, size); ok {
			return true
		}
	}
	return false
}

// HasColorEmoji forwards to the Painter, which owns the font chain.
func (c *ggCanvas) HasColorEmoji(font, s string, size float32) bool {
	return c.p.HasColorEmoji(font, s, size)
}
