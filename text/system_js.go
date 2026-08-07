//go:build js

package text

// UseSystemFonts is a no-op on the web: a wasm page has no OS font directory to
// scan, and pulling in fontscan would bloat the binary for nothing. Web apps
// register the fonts they need explicitly (NewShaper / SetFonts). Runes with no
// glyph in the explicit chain fall back to the primary font.
func (s *Shaper) UseSystemFonts(string) error { return nil }
