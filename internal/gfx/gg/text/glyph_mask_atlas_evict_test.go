package text

import (
	"fmt"
	"testing"
)

// A full atlas must make room, not refuse.
//
// Put evicted when the entry count reached MaxEntries and never when the
// pages ran out of space, so an atlas that filled spatially first — which is
// what a long scroll through varied text does — refused every glyph from that
// moment on. Permanently: nothing freed a page, so nothing ever recovered.
// On a phone that reads as text turning to garbage mid-scroll and staying
// that way, since the caller draws whatever it has when Put returns an error.
func TestAtlasEvictsWhenOutOfSpace(t *testing.T) {
	cfg := DefaultGlyphMaskAtlasConfig()
	cfg.MaxAtlases = 1
	cfg.Size = 64 // a handful of glyphs fills this
	a, err := NewGlyphMaskAtlas(cfg)
	if err != nil {
		t.Fatal(err)
	}

	mask := make([]byte, 16*16)
	for i := range mask {
		mask[i] = 0xff
	}

	// One glyph per frame, which is what scrolling does: each frame brings
	// new text and last frame's goes off screen. Within a single frame nothing
	// is evictable — every entry is on screen — so a full atlas refusing
	// mid-frame is correct, and only the clock moving makes room.
	const glyphs = 64
	admitted := 0
	for i := 0; i < glyphs; i++ {
		a.AdvanceFrame()
		a.AdvanceFrame()
		a.AdvanceFrame()
		k := GlyphMaskKey{GlyphID: uint16(i), SizeQ4: 192}
		if _, err := a.Put(k, mask, 16, 16, 0, 0); err != nil {
			t.Fatalf("glyph %d refused: %v — a full atlas must evict to make "+
				"room, or text stops rendering the moment it fills", i, err)
		}
		admitted++
	}
	if admitted != glyphs {
		t.Fatalf("admitted %d of %d", admitted, glyphs)
	}

	// Whatever survived must own its space exclusively: two live entries
	// pointing at one region is how garbled text looks.
	seen := map[string]uint16{}
	for i := 0; i < glyphs; i++ {
		k := GlyphMaskKey{GlyphID: uint16(i), SizeQ4: 192}
		r, ok := a.Get(k)
		if !ok {
			continue // evicted, which is the correct outcome for most of them
		}
		id := fmt.Sprintf("page%d@%d,%d", r.AtlasIndex, r.X, r.Y)
		if prev, dup := seen[id]; dup {
			t.Fatalf("glyphs %d and %d both claim %s — a live entry points at "+
				"space that was reallocated", prev, k.GlyphID, id)
		}
		seen[id] = k.GlyphID
	}
}

// A glyph too large for an empty page must fail rather than evict forever.
func TestAtlasRefusesAnUnfittableGlyph(t *testing.T) {
	cfg := DefaultGlyphMaskAtlasConfig()
	cfg.MaxAtlases = 1
	cfg.Size = 64 // the minimum a config may declare
	a, err := NewGlyphMaskAtlas(cfg)
	if err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 128*128)
	if _, err := a.Put(GlyphMaskKey{GlyphID: 1, SizeQ4: 192}, big, 128, 128, 0, 0); err == nil {
		t.Fatal("a 128px glyph was accepted into a 64px atlas; the eviction loop " +
			"must give up when there is nothing left to evict rather than spin")
	}
}

// A glyph the frame being drawn has already used must never be evicted.
//
// Its coordinates are already in a recorded quad; handing that space to
// another glyph makes the quad sample the wrong pixels, which is text drawn
// as other text — the corruption that survived the first fix, appearing after
// a while of ordinary use and never clearing.
func TestAtlasWillNotEvictAGlyphInUseThisFrame(t *testing.T) {
	cfg := DefaultGlyphMaskAtlasConfig()
	cfg.MaxAtlases = 1
	cfg.Size = 64
	a, err := NewGlyphMaskAtlas(cfg)
	if err != nil {
		t.Fatal(err)
	}
	mask := make([]byte, 16*16)
	for i := range mask {
		mask[i] = 0xff
	}

	// Fill the atlas within one frame, recording where each glyph landed.
	placed := map[uint16]GlyphMaskRegion{}
	for i := 0; i < 64; i++ {
		k := GlyphMaskKey{GlyphID: uint16(i), SizeQ4: 192}
		r, err := a.Put(k, mask, 16, 16, 0, 0)
		if err != nil {
			break // full, and correctly refusing rather than evicting
		}
		placed[uint16(i)] = r
	}
	if len(placed) < 2 {
		t.Fatalf("only %d glyphs fit; too few to test", len(placed))
	}

	// Every glyph placed this frame must still be where it was put.
	for id, want := range placed {
		got, ok := a.Get(GlyphMaskKey{GlyphID: id, SizeQ4: 192})
		if !ok {
			t.Fatalf("glyph %d was evicted during the frame that drew it", id)
		}
		if got != want {
			t.Fatalf("glyph %d moved from %+v to %+v during its own frame — "+
				"a quad recorded earlier now samples another glyph", id, want, got)
		}
	}
}
