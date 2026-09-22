package paint

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/internal/testfont"
	"github.com/doug/gophics/text"
)

// TestFamilyChainOrder pins the resolution order every family shares: its
// own face, the fallbacks in load order, then the default family. The order
// of registration must not matter — a family loaded after the fallbacks and
// after the default gets the same chain as one loaded before.
func TestFamilyChainOrder(t *testing.T) {
	symA, err := testfont.Remap(goregular.TTF, map[rune]rune{'✓': 'V', '⋮': ':'})
	if err != nil {
		t.Fatal(err)
	}
	symB, err := testfont.Remap(goregular.TTF, map[rune]rune{'⋮': 'X', '↩': 'Y'})
	if err != nil {
		t.Fatal(err)
	}
	p := NewPainter()
	for _, step := range []func() error{
		func() error { return p.LoadFallbackFont(symA) },
		func() error { return p.LoadFont(goregular.TTF) },
		func() error { return p.LoadFontFamily("bold", gobold.TTF) },
		func() error { return p.LoadFallbackFont(symB) },
		func() error { return p.LoadFontFamily("late", symB) },
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	face := func(family, s string) (uint32, *text.Font) {
		l := p.ShapeIn(family, s, 14)
		if len(l.Glyphs) != 1 {
			t.Fatalf("%q in %q: %d glyphs", s, family, len(l.Glyphs))
		}
		return l.Glyphs[0].GID, l.Glyphs[0].Font
	}
	_, def := face("", "a")
	_, bold := face("bold", "a")
	_, fbA := face("", "✓")
	_, fbB := face("", "↩")
	if def == bold || fbA == fbB || fbA == def || fbB == def {
		t.Fatal("test faces are not distinct")
	}
	// ⋮ is in both fallbacks; load order decides.
	if _, f := face("", "⋮"); f != fbA {
		t.Fatal("default family: fallbacks not consulted in load order")
	}
	if _, f := face("bold", "⋮"); f != fbA {
		t.Fatal("bold family: fallbacks not consulted in load order")
	}
	// The "late" family is a second parse of symB — its own *text.Font,
	// distinct from the fallback's — and it wins for ⋮ over fbA.
	_, lateOwn := face("late", "↩")
	if lateOwn == fbB || lateOwn == def {
		t.Fatal("late family did not shape ↩ with its own face")
	}
	if _, f := face("late", "⋮"); f != lateOwn {
		t.Fatal("a family's own face must come before the fallbacks")
	}
	// ...and Latin, which no fallback has, comes from the default.
	if gid, f := face("late", "a"); f != def || gid == 0 {
		t.Fatal("a family lacking a rune must fall through to the default font")
	}
	// A rune no face has is the family's own .notdef, not a panic.
	if gid, _ := face("bold", "☃"); gid != 0 {
		t.Fatalf("☃ gid %d, want .notdef", gid)
	}
}
