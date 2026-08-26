package paint

import (
	"strings"
	"testing"
	"unicode/utf8"

	"golang.org/x/image/font/gofont/goregular"
)

// Wrapping survives arbitrary text.
//
// Text is where a UI toolkit meets input it did not choose: pasted content,
// user names, a truncated log line, a file that turned out not to be UTF-8.
// Shaping and line breaking walk runes, cluster boundaries and break
// opportunities, and an index that runs off the end of any of those is a panic
// in the middle of a frame — which takes the app with it, not just the label.
//
// Two things are checked. It must not panic on anything, including invalid
// UTF-8. And a wrapped line must fit the width it was given, unless it holds a
// single token that cannot be broken — a 200-character word has nowhere to go,
// and reporting it as an overflow rather than a wrap is correct.
func FuzzWrapText(f *testing.F) {
	f.Add("hello world", float32(50))
	f.Add("", float32(10))
	f.Add(strings.Repeat("a", 300), float32(40))
	f.Add("áé combining", float32(30))
	f.Add("日本語のテキスト", float32(25))
	f.Add("mixed العربية and latin", float32(60))
	f.Add("\xff\xfe invalid bytes", float32(30))
	f.Add("tabs\tand\nnewlines\r\n", float32(20))
	f.Add("emoji 👩‍👩‍👧‍👦 family", float32(35))

	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		f.Fatal(err)
	}

	f.Fuzz(func(t *testing.T, s string, width float32) {
		// Absurd widths are the caller's business, not a reason to crash, but
		// they make the fit assertion meaningless — so bound what is asserted,
		// not what is run.
		lines := p.WrapTextIn("", s, 14, width)

		// The width claim is asserted on ASCII only, and deliberately.
		//
		// Deciding whether a line "could have broken" needs a line-breaking
		// algorithm, and strings.Fields is not one: it calls a combining mark
		// after a space a separate word, where UAX #14 gives the mark its
		// base's class and offers no break there. Two of the three findings
		// from this fuzzer were that disagreement rather than a wrapping bug.
		// On ASCII the two oracles agree, and that is where the real fault was
		// found. Everything below still runs on every input — a panic or an
		// invalid line is a fault whatever the script.
		if width > 4 && width < 4000 && isASCII(s) {
			for _, ln := range lines {
				fields := strings.Fields(ln)
				if len(fields) < 2 {
					continue // one token: nowhere to break, overflow is honest
				}
				// A line whose *first* token already exceeds the width could
				// not have been helped by breaking either — that token has to
				// go somewhere. Only a line that had a usable break and did
				// not take it is a fault.
				if p.MeasureWidthIn("", fields[0], 14) > width {
					continue
				}
				if w := p.MeasureWidthIn("", ln, 14); w > width+1 {
					t.Errorf("line %q measures %v, past the %v it was given, "+
						"though it could have broken after %q", ln, w, width, fields[0])
				}
			}
		}

		// Wrapping cannot invent or destroy content: every line has to be
		// valid UTF-8 once the input was, or a cluster was cut in half.
		if utf8.ValidString(s) {
			for _, ln := range lines {
				if !utf8.ValidString(ln) {
					t.Errorf("valid input produced an invalid line %q", ln)
				}
			}
		}

		// The paragraph path walks the same machinery with shaping attached.
		p.ParagraphIn("", s, 14, width)
	})
}

func isASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return false
		}
	}
	return true
}
