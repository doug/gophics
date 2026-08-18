package ui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
)

// The app ships no icon assets, so every symbol on screen is a character in the
// Go fonts — and those fonts are narrower than they look. A gear, a bullseye, a
// check mark and a north-east arrow are all absent, and a missing glyph does
// not fail, warn, or fall back: it draws an empty box. The first tab bar built
// here was three tofu squares.
//
// Rather than trust a list someone remembers to update, this reads the
// package's own source, pulls every non-ASCII character out of its string
// literals, and asks the fonts whether they can draw it.
func TestEveryGlyphInTheUIExistsInTheFonts(t *testing.T) {
	fonts := map[string][]byte{
		"regular":    goregular.TTF,
		"bold":       gobold.TTF,
		"italic":     goitalic.TTF,
		"bolditalic": gobolditalic.TTF,
		"mono":       gomono.TTF,
	}
	parsed := map[string]*sfnt.Font{}
	for name, ttf := range fonts {
		f, err := sfnt.Parse(ttf)
		if err != nil {
			t.Fatalf("parsing the %s face: %v", name, err)
		}
		parsed[name] = f
	}

	for r, where := range uiLiteralRunes(t) {
		var missing []string
		for name, f := range parsed {
			var b sfnt.Buffer
			idx, err := f.GlyphIndex(&b, r)
			if err != nil || idx == 0 {
				missing = append(missing, name)
			}
		}
		if len(missing) > 0 {
			t.Errorf("%q (U+%04X) at %s has no glyph in: %s — it will draw as an empty box",
				string(r), r, where, strings.Join(missing, ", "))
		}
	}
}

// stringLit matches a Go interpreted string literal.
var stringLit = regexp.MustCompile(`"(?:[^"\\]|\\.)*"`)

// uiLiteralRunes returns the non-ASCII characters appearing in string literals
// across the package, mapped to where they were found.
func uiLiteralRunes(t *testing.T) map[rune]string {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	out := map[rune]string{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(".", name))
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			// Comments are prose for people, not text for the renderer, and
			// they are full of em dashes.
			if code := stripComment(line); code != "" {
				for _, lit := range stringLit.FindAllString(code, -1) {
					for _, r := range lit {
						if r > unicode.MaxASCII {
							if _, seen := out[r]; !seen {
								out[r] = name + ":" + itoa(i+1)
							}
						}
					}
				}
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("found no non-ASCII characters at all — the scan is not working")
	}
	return out
}

// stripComment drops a trailing line comment, taking care not to cut inside a
// string literal (an article URL contains "//").
func stripComment(line string) string {
	inString, escaped := false, false
	for i := 0; i < len(line)-1; i++ {
		c := line[i]
		switch {
		case escaped:
			escaped = false
		case c == '\\' && inString:
			escaped = true
		case c == '"':
			inString = !inString
		case c == '/' && line[i+1] == '/' && !inString:
			return line[:i]
		}
	}
	if strings.HasPrefix(strings.TrimSpace(line), "//") {
		return ""
	}
	return line
}
