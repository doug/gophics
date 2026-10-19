//go:build !js

package text

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-text/typesetting/font"
	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/language"
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
	s.system = &scriptedFontMap{fm: fm}
	return nil
}

// scriptedFontMap wraps fontscan.FontMap so that every query names the rune's
// writing system before asking for a face.
//
// This is not a refinement; without it the fallback cannot reach an emoji font
// at all. fontscan matches candidates on the family query *and* the script it
// was last told about, and its default is the empty script. Asking a freshly
// built FontMap for U+1F600 logs "No font matched for script [empty] and rune
// U+1F600 -> returning arbitrary face" and hands back whatever face happens to
// be first — one with no glyph for the rune, so shaping yields GID 0 and the
// painter draws .notdef from a Latin font. Measured on macOS: with no script
// set, U+1F600 resolves to a face where NominalGlyph reports ok=false; with
// language.LookupScript(r) set first, it resolves to Apple Color Emoji with
// GID 2096 and font.GlyphBitmap data. Setting a family query instead
// (SetQuery("sans-serif")) changes nothing — it is the script that selects.
//
// language.LookupScript is used rather than this package's own ScriptOf because
// fontscan matches against the UAX #24 tags it indexed the system's fonts
// under, and ScriptOf deliberately reports the synthetic "Zsye" for
// pictographs (see script.go), which no font declares coverage of.
type scriptedFontMap struct {
	fm *fontscan.FontMap
	// script is the last value handed to SetScript. SetScript rebuilds the
	// candidate list, so repeating it per rune would pay for a rebuild on
	// every character of a CJK paragraph. Runs are script-homogeneous and the
	// explicit fallback chain is consulted first, so in practice this
	// collapses to one SetScript per script per paragraph.
	script language.Script
	set    bool
}

// ResolveFace implements systemFontMap.
func (m *scriptedFontMap) ResolveFace(r rune) *font.Face {
	if sc := language.LookupScript(r); !m.set || sc != m.script {
		m.fm.SetScript(sc)
		m.script, m.set = sc, true
	}
	return m.fm.ResolveFace(r)
}
