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
// user cache dir). Scanning is slow the first time and cached afterward;
// loading the index is tens of milliseconds each call, so an owner with
// several Shapers should scan once and hand the result on with
// ShareSystemFonts rather than call this on each.
//
// The map is private to this Shaper and whoever it is shared with, never to
// the process. It cannot be otherwise: fontscan's FontMap hands out
// *font.Face values it caches, and harfbuzz writes into a Face's cmap cache
// while shaping, so two goroutines shaping through faces from one map race
// on the faces themselves — a lock around ResolveFace would not reach that.
// Sharing among the Shapers of one Painter is fine because they all run on
// that app's UI goroutine.
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
