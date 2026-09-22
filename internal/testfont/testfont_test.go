package testfont

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/text"
)

func TestRemapCoversOnlyTheNamedRunes(t *testing.T) {
	data, err := Remap(goregular.TTF, map[rune]rune{'✓': 'V', '⋮': ':'})
	if err != nil {
		t.Fatal(err)
	}
	f, err := text.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	base, _ := text.Parse(goregular.TTF)
	if base.HasGlyph('✓') {
		t.Fatal("Go Regular grew a check mark; pick another rune")
	}
	if !f.HasGlyph('✓') || !f.HasGlyph('⋮') {
		t.Fatal("remapped font lacks the runes it was given")
	}
	if f.HasGlyph('a') || f.HasGlyph(' ') {
		t.Fatal("remapped font still covers the base's runes")
	}
	want, _ := base.NominalGID('V')
	if got, _ := f.NominalGID('✓'); got != want {
		t.Fatalf("✓ -> gid %d, want V's %d", got, want)
	}
	if l := text.NewShaper(f).Line("✓", 16); len(l.Glyphs) != 1 || l.Glyphs[0].GID != want || l.Glyphs[0].Advance <= 0 {
		t.Fatalf("shaped ✓ = %+v", l.Glyphs)
	}
}
