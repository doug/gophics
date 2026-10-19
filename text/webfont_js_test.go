//go:build js && wasm

// These run under node via go_js_wasm_exec. Node supplies the same whatwg
// fetch(), AbortController and data: URLs the browser does, so the fetch path
// executes for real; what it does not supply is window.queryLocalFonts, which
// makes it an honest stand-in for every browser but desktop Chromium and so
// the right place to check that the Local Font Access path degrades rather
// than panics.
//
// This is the external test package because it needs internal/testfont, which
// imports text — the only way to get a face covering a rune the primary lacks
// without vendoring a fixture font.
package text_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/internal/testfont"
	"github.com/doug/gophics/text"
)

// '✓' is U+2713, in the Dingbats block, so ScriptOf calls it Zsye. Go Regular
// has no glyph for it (internal/testfont's own test asserts that), which makes
// it the rune to fall back on.
const missing = '✓'

func primary(t *testing.T) *text.Font {
	t.Helper()
	f, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	if f.HasGlyph(missing) {
		t.Fatalf("Go Regular grew a glyph for U+%04X; pick another rune", missing)
	}
	return f
}

// coveringFont returns the bytes of a face whose only glyph is `missing`.
func coveringFont(t *testing.T) []byte {
	t.Helper()
	data, err := testfont.Remap(goregular.TTF, map[rune]rune{missing: 'V'})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

type sourceFunc struct {
	fn    func(text.Script) ([]byte, error)
	calls int
	seen  []text.Script
}

func (s *sourceFunc) FontForScript(_ context.Context, sc text.Script) ([]byte, error) {
	s.calls++
	s.seen = append(s.seen, sc)
	return s.fn(sc)
}

// quiet swallows the package's default log line so a test's own failure output
// is readable, and returns what was logged for tests that want to assert on it.
func quiet(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(old) })
	return &buf
}

// The point of the whole mechanism: a rune the chain cannot draw is drawn from
// a fetched face, and the face that gets picked is one that actually has the
// glyph — not merely one nominated for the script.
func TestWebFontCoversTheRune(t *testing.T) {
	quiet(t)
	data := coveringFont(t)
	src := &sourceFunc{fn: func(text.Script) ([]byte, error) { return data, nil }}

	sh := text.NewShaper(primary(t))
	landed := make(chan struct{}, 4)
	if err := sh.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}

	// The first frame is tofu and must be: the bytes are not there yet and
	// shaping is synchronous.
	first := sh.Line(string(missing), 16)
	if len(first.Glyphs) != 1 {
		t.Fatalf("first pass shaped %d glyphs", len(first.Glyphs))
	}
	if first.Glyphs[0].Font.HasGlyph(missing) {
		t.Fatal("first pass already had the glyph; the test is not testing the fetch")
	}

	<-landed

	second := sh.Line(string(missing), 16)
	if len(second.Glyphs) != 1 {
		t.Fatalf("second pass shaped %d glyphs", len(second.Glyphs))
	}
	if !second.Glyphs[0].Font.HasGlyph(missing) {
		t.Fatalf("the face chosen for U+%04X does not cover it", missing)
	}
	if second.Glyphs[0].Advance <= 0 {
		t.Errorf("fetched glyph has advance %v", second.Glyphs[0].Advance)
	}
	if want := text.ScriptEmoji; len(src.seen) == 0 || src.seen[0] != want {
		t.Errorf("source was asked for %v, want %q first", src.seen, want)
	}
}

// A face nominated for a script but lacking the rune must not be handed to the
// shaper: a face with no glyph for the rune yields a silently wrong glyph index
// instead of a reported miss.
func TestFetchedFaceWithoutTheRuneIsRejected(t *testing.T) {
	quiet(t)
	// Covers '⋮' and nothing else, so it is the wrong answer for '✓'.
	wrong, err := testfont.Remap(goregular.TTF, map[rune]rune{'⋮': ':'})
	if err != nil {
		t.Fatal(err)
	}
	src := &sourceFunc{fn: func(text.Script) ([]byte, error) { return wrong, nil }}

	sh := text.NewShaper(primary(t))
	var missed []text.Script
	sh.OnMissingGlyph(func(_ rune, sc text.Script) { missed = append(missed, sc) })
	landed := make(chan struct{}, 4)
	if err := sh.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	sh.Line(string(missing), 16)
	<-landed
	l := sh.Line(string(missing), 16)
	if l.Glyphs[0].Font.HasGlyph(missing) {
		t.Fatal("a face lacking the rune was used for it")
	}
	if len(missed) == 0 {
		t.Error("the rune is still missing and nothing said so")
	}
}

// A source that fails is asked once. Retrying per rune would turn one missing
// font into a request per glyph.
func TestFailedFetchIsRememberedNotRetried(t *testing.T) {
	buf := quiet(t)
	src := &sourceFunc{fn: func(text.Script) ([]byte, error) { return nil, errors.New("boom") }}
	sh := text.NewShaper(primary(t))
	landed := make(chan struct{}, 4)
	if err := sh.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	sh.Line(string(missing), 16)
	<-landed
	for range 5 {
		sh.Line(strings.Repeat(string(missing), 4), 16)
	}
	if src.calls != 1 {
		t.Errorf("source called %d times, want 1", src.calls)
	}
	if !bytes.Contains(buf.Bytes(), []byte("boom")) {
		t.Errorf("the failure was not reported: %q", buf.String())
	}
}

// WOFF2 is the format every font CDN serves by default and the one format
// go-text/typesetting cannot read. Reporting it as such rather than as "invalid
// font" is the difference between a one-line fix and an afternoon.
func TestWOFF2IsNamedNotJustRejected(t *testing.T) {
	buf := quiet(t)
	src := &sourceFunc{fn: func(text.Script) ([]byte, error) {
		return append([]byte("wOF2"), make([]byte, 64)...), nil
	}}
	sh := text.NewShaper(primary(t))
	landed := make(chan struct{}, 4)
	if err := sh.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	sh.Line(string(missing), 16)
	<-landed
	sh.Line(string(missing), 16)
	if !bytes.Contains(buf.Bytes(), []byte("WOFF2")) {
		t.Errorf("WOFF2 bytes were not identified: %q", buf.String())
	}
}

// While a fetch is in flight the rune is not yet known to be uncovered, and
// telling a developer who has configured UseWebFonts correctly to try
// UseWebFonts is how a useful message becomes one that is always ignored. Once
// the attempt has failed, the rune really is uncovered and the report is due.
func TestPendingFetchSuppressesTheTofuReportUntilItFails(t *testing.T) {
	quiet(t)
	release := make(chan struct{})
	src := &sourceFunc{fn: func(text.Script) ([]byte, error) {
		<-release
		return nil, errors.New("gave up")
	}}
	sh := text.NewShaper(primary(t))
	var missed []text.Script
	sh.OnMissingGlyph(func(_ rune, sc text.Script) { missed = append(missed, sc) })
	landed := make(chan struct{}, 4)
	if err := sh.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}

	// Frames while the fetch is outstanding.
	for range 3 {
		sh.Line(string(missing), 16)
	}
	if len(missed) != 0 {
		t.Errorf("reported %v while a fetch was still in flight", missed)
	}

	close(release)
	<-landed
	sh.Line(string(missing), 16)
	if len(missed) != 1 || missed[0] != text.ScriptEmoji {
		t.Errorf("after the fetch failed the report was %v, want one %q", missed, text.ScriptEmoji)
	}
}

func TestUseWebFontsRejectsANilSource(t *testing.T) {
	if err := text.NewShaper(primary(t)).UseWebFonts(context.Background(), nil, nil); err == nil {
		t.Error("a nil source was accepted")
	}
}

// The fetch source end to end, over the browser's own fetch(), with the font
// bytes served from a data: URL.
func TestFetchFontSourceLoadsOverFetch(t *testing.T) {
	quiet(t)
	url := "data:font/ttf;base64," + base64.StdEncoding.EncodeToString(coveringFont(t))
	src := text.FetchFontSource(map[text.Script]string{text.ScriptEmoji: url})

	sh := text.NewShaper(primary(t))
	landed := make(chan struct{}, 4)
	if err := sh.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	sh.Line(string(missing), 16)
	<-landed
	l := sh.Line(string(missing), 16)
	if !l.Glyphs[0].Font.HasGlyph(missing) {
		t.Fatalf("the fetched face does not cover U+%04X", missing)
	}
}

func TestFetchFontSourceNamesAnUnconfiguredScript(t *testing.T) {
	src := text.FetchFontSource(map[text.Script]string{"Hani": "/fonts/han.ttf"})
	_, err := src.FontForScript(context.Background(), "Thai")
	if err == nil {
		t.Fatal("an unconfigured script was accepted")
	}
	if !strings.Contains(err.Error(), "Thai") {
		t.Errorf("error does not name the script: %v", err)
	}
}

// A 404 is an error, not an empty font: fetch.Get turns a non-2xx into a
// *StatusError, and the mechanism must not install zero bytes.
func TestFetchFontSourceSurvivesABadURL(t *testing.T) {
	quiet(t)
	src := text.FetchFontSource(map[text.Script]string{text.ScriptEmoji: "http://127.0.0.1:1/nope.ttf"})
	if _, err := src.FontForScript(context.Background(), text.ScriptEmoji); err == nil {
		t.Error("an unreachable URL reported success")
	}
}

// Node has no window.queryLocalFonts, which is also true of Firefox, Safari and
// every mobile browser. The requirement is that it says so and keeps going.
func TestLocalFontSourceDegradesWithoutTheAPI(t *testing.T) {
	src := text.LocalFontSource(nil)
	_, err := src.FontForScript(context.Background(), "Hani")
	if err == nil {
		t.Fatal("LocalFontSource reported success without the API")
	}
	if !strings.Contains(err.Error(), "queryLocalFonts") {
		t.Errorf("error does not name the missing API: %v", err)
	}
	// And the whole Shaper keeps working — tofu, not a panic.
	quiet(t)
	sh := text.NewShaper(primary(t))
	landed := make(chan struct{}, 4)
	if err := sh.UseWebFonts(context.Background(), src, func() { landed <- struct{}{} }); err != nil {
		t.Fatal(err)
	}
	sh.Line("Hello "+string(missing), 16)
	<-landed
	if l := sh.Line("Hello", 16); len(l.Glyphs) == 0 || l.Width <= 0 {
		t.Errorf("covered text broke after a refused local-font lookup: %+v", l)
	}
}

func TestLocalFontSourceRejectsAnUnlistedScript(t *testing.T) {
	// Zzzz is in no candidate list, built-in or otherwise.
	_, err := text.LocalFontSource(nil).FontForScript(context.Background(), text.ScriptUnknown)
	if err == nil {
		t.Fatal("an unlisted script was accepted")
	}
}

// The no-op that started this: UseSystemFonts returned nil on the web, so an
// app was told system fonts were on and then rendered tofu for ever.
func TestUseSystemFontsReportsThatItCannot(t *testing.T) {
	err := text.NewShaper(primary(t)).UseSystemFonts("")
	if err == nil {
		t.Fatal("UseSystemFonts still returns nil on js; it is a silent no-op again")
	}
	if !strings.Contains(err.Error(), "UseWebFonts") {
		t.Errorf("the error does not point at the alternative: %v", err)
	}
}
