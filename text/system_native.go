//go:build !js

package text

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-text/typesetting/fontscan"
)

// UseSystemFonts extends the fallback chain with the platform's installed fonts
// (via fontscan): runes not covered by the explicit chain — CJK, emoji, symbols
// — resolve to a system font. cacheDir holds fontscan's index ("" uses the OS
// user cache dir). Scanning is slow the first time and cached afterward.
//
// This lives in a native-only file so the heavy fontscan package (OS font
// scanning) is not linked into the wasm binary, where there are no system fonts
// to scan; see the js stub in system_js.go.
func (s *Shaper) UseSystemFonts(cacheDir string) error {
	if cacheDir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return fmt.Errorf("text: no cache dir: %w", err)
		}
		cacheDir = filepath.Join(base, "gophics", "fontscan")
	}
	fm := fontscan.NewFontMap(nil)
	if err := fm.UseSystemFonts(cacheDir); err != nil {
		return fmt.Errorf("text: system fonts: %w", err)
	}
	s.system = fm
	return nil
}
