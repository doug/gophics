package text

import (
	"bytes"
	"image"
	"image/png"

	"github.com/doug/gophics/geom"
	"github.com/go-text/typesetting/font"
)

// Colour emoji, and the one family of fonts this does not cover.
//
// A colour emoji is not an outline, so AppendGlyphPath emits nothing for it and
// a painter that only knows outlines draws a blank. The three methods below are
// the other half: they report that a glyph is a colour bitmap, where it goes,
// and what its pixels are, so the painter can blit it beside the outlines it
// fills.
//
// # Non-goal: COLR fonts
//
// This handles bitmap colour glyphs — the sbix table (Apple Color Emoji) and
// CBDT/EBDT (Noto Color Emoji as shipped on Android, and the older desktop
// Noto). It does *not* handle COLR, which is not pixels at all but a paint
// graph: a tree of layers, solid and gradient fills, composite modes and
// transforms that a renderer walks and draws. font.Face.GlyphData checks COLR
// *first*, before any bitmap table, and returns font.GlyphColor for such a
// glyph — so a font that has both answers COLR, and ColorGlyphImage reports
// ok=false for it.
//
// The user-visible consequence, stated here rather than left to be discovered:
// **Windows' system emoji font (Segoe UI Emoji) is COLR, and recent Noto Color
// Emoji is COLR too, so colour emoji do not render on a stock Windows build.**
// They render on macOS and iOS (sbix) and on Android (CBDT). A COLR renderer is
// separate and larger work — it needs gradient and compositing support per
// glyph layer, not a blit — and until it exists a Windows app that wants emoji
// must register a bitmap or monochrome-outline emoji face of its own.
//
// On wasm there are no system fonts at all (see UseSystemFonts in
// system_js.go), so a web app must fetch an emoji face explicitly with
// UseWebFonts or register one with SetFonts. That is not an emoji bug.

// ColorGlyphBox reports where gid's colour bitmap paints at the given logical
// size, relative to the pen position on the baseline, y down — so the top of
// the glyph is negative, matching InkBounds elsewhere in gophics. ok is false
// for every glyph this cannot blit: outlines, SVG glyphs, COLR paint graphs
// (see the package note above), and bitmap formats other than PNG.
//
// This is separate from ColorGlyphImage because the two callers need different
// halves. Ink bounds and damage need the box and must not pay to decode a PNG;
// drawing needs the pixels and knows the device scale that decides which strike
// to decode. The box is the same either way: it comes from the font's glyph
// extents, which for a bitmap glyph are read out of the bitmap table (and, for
// Apple Color Emoji, measured as exactly one em square sitting on the baseline
// and independent of ppem).
func (f *Font) ColorGlyphBox(gid uint32, size float32) (geom.Rect, bool) {
	if !f.isColorBitmap(gid) {
		return geom.Rect{}, false
	}
	ext, ok := f.face.GlyphExtents(font.GID(gid))
	if !ok {
		// No extents for a glyph that does have a bitmap: fall back to the em
		// square on the baseline, which is what every emoji font in practice
		// reports anyway. Returning false here instead would make the glyph
		// invisible, which is the failure this whole file exists to fix.
		return geom.Rect{Min: geom.Pt{X: 0, Y: -size}, Max: geom.Pt{X: size, Y: 0}}, true
	}
	scale := size / float32(f.face.Upem())
	// GlyphExtents is y-up with YBearing at the top of the glyph and a
	// negative Height running downward; gophics is y-down.
	x0 := ext.XBearing * scale
	y0 := -ext.YBearing * scale
	r := geom.Rect{
		Min: geom.Pt{X: x0, Y: y0},
		Max: geom.Pt{X: x0 + ext.Width*scale, Y: y0 - ext.Height*scale},
	}
	if r.Min.X > r.Max.X {
		r.Min.X, r.Max.X = r.Max.X, r.Min.X
	}
	if r.Min.Y > r.Max.Y {
		r.Min.Y, r.Max.Y = r.Max.Y, r.Min.Y
	}
	if r.IsEmpty() {
		return geom.Rect{Min: geom.Pt{X: 0, Y: -size}, Max: geom.Pt{X: size, Y: 0}}, true
	}
	return r, true
}

// ColorGlyphPpem quantises a wanted size in device pixels to a pixels-per-em
// value the font actually has a bitmap strike for, or returns 0 when the font
// has no strikes.
//
// Callers must quantise, for two reasons that are both about the cache above
// this one. A glyph atlas keyed on the exact device size mints a fresh cell on
// every frame of a zoom animation, because 33, 34 and 35 device pixels all
// decode the same 40-ppem strike; and asking twice for two sizes that land on
// one strike decodes the same PNG twice. Apple Color Emoji carries strikes at
// ppem 20, 26, 32, 40, 48, 52, 64, 96 and 160, so the quantisation is coarse
// and the saving is large.
//
// The rule mirrors the font library's own strike choice (sbix.chooseStrike):
// the smallest strike at least as large as the request, and the largest strike
// when the request exceeds them all. Mirroring it is what makes the value
// idempotent — passing the result back in selects the same strike — which is
// what a cache key needs.
func (f *Font) ColorGlyphPpem(px int) int {
	sizes := f.face.BitmapSizes()
	if len(sizes) == 0 {
		return 0
	}
	if px < 1 {
		px = 1
	}
	best, largest := 0, 0
	for _, s := range sizes {
		p := int(max(s.XPpem, s.YPpem))
		if p <= 0 {
			continue
		}
		largest = max(largest, p)
		if p >= px && (best == 0 || p < best) {
			best = p
		}
	}
	if best == 0 {
		return largest
	}
	return best
}

// ColorGlyphImage decodes gid's colour bitmap at the given pixels-per-em,
// which must come from ColorGlyphPpem. ok is false for the same set of glyphs
// ColorGlyphBox rejects, and for a PNG the standard library cannot decode.
//
// ppem is not advisory: it selects the strike. A font asked at ppem 0 — which
// is what a face gophics never told a size to reports — hands back its
// *largest* strike, measured at 160x160 and 23,780 bytes of PNG for Apple
// Color Emoji where the 20-ppem strike is 20x20 and 1,093 bytes. That is 20x
// the decode and, worse, an 8x minification through the image pipeline's fixed
// bilinear sampler with no mipmaps, which aliases visibly. Passing the right
// ppem is the difference between a crisp emoji and a muddy one.
//
// The decode is image/png from the standard library. The font library locates
// and identifies the bitmap but does not decode it: GlyphBitmap.Data is raw
// PNG bytes.
//
// Only safe on the goroutine that owns the Shaper. Setting the strike mutates
// the face, and faces are shared — fontscan hands the same *font.Face to every
// size, and harfbuzz already writes into a face's cmap cache while shaping, so
// UseSystemFonts' documented single-goroutine invariant is what makes this
// sound. The SetPpem call is kept adjacent to the GlyphData call below so no
// shaping can interleave between them.
func (f *Font) ColorGlyphImage(gid uint32, ppem int) (image.Image, bool) {
	if ppem <= 0 || ppem > 0xFFFF || gid > 0xFFFF {
		return nil, false
	}
	if _, isCOLR := f.face.GlyphDataColor(uint16(gid)); isCOLR {
		return nil, false
	}
	// SetPpem also resets the face's whole glyph-extents cache, so read the
	// extents a caller might want *before* this point (ColorGlyphBox does) and
	// group a size's glyphs together rather than interleaving sizes.
	p := uint16(ppem)
	f.face.SetPpem(p, p)
	bm, ok := f.face.GlyphDataBitmap(uint16(gid))
	if !ok || bm.Format != font.PNG || len(bm.Data) == 0 {
		return nil, false
	}
	img, err := png.Decode(bytes.NewReader(bm.Data))
	if err != nil {
		return nil, false
	}
	return img, true
}

// isColorBitmap reports whether gid is a glyph this file can blit. It is the
// single place the "COLR wins over bitmap" ordering is observed, so the two
// public answers cannot disagree about which glyphs are in scope.
//
// Deliberately asked without touching ppem: a strike answers at whatever ppem
// the face currently carries, and whether a glyph *has* bitmap data does not
// depend on which strike answers. It also asks the two tables directly rather
// than through GlyphData, because GlyphData falls through to the outline
// tables and would decode and allocate a full outline for every Latin glyph
// this is asked about — and ink bounds asks about all of them.
func (f *Font) isColorBitmap(gid uint32) bool {
	// Glyph ids are 16-bit in every OpenType table, and the tables are indexed
	// by uint16; a wider value cannot name a glyph, and truncating it would
	// name the wrong one.
	if gid > 0xFFFF {
		return false
	}
	if _, isCOLR := f.face.GlyphDataColor(uint16(gid)); isCOLR {
		return false
	}
	bm, ok := f.face.GlyphDataBitmap(uint16(gid))
	return ok && bm.Format == font.PNG
}
