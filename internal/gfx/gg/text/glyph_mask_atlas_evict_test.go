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

	const glyphs = 64
	admitted := 0
	for i := 0; i < glyphs; i++ {
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
