package paint

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"
)

// A wrapped line does not keep the carriage return it broke on.
//
// The wrapper leaves its break rune at the end of the line and WrapTextIn
// trims it, but it only knew about ' ' and '\n'. Text arriving as CRLF — from
// a Windows editor, a file, a network payload — breaks after the '\r', and a
// carriage return left on the line is shaped as a visible glyph: 3.89pt at
// 14pt in the Go fonts, because there is no glyph for it and .notdef has
// width. The line's content fits the width it was measured against and then
// renders wider than it, so CRLF text spills out of a box that the same text
// with plain LF sits inside.
//
// Found by FuzzWrapText in about a second, on "n 0\r".
func TestWrappedLinesDropTheirCarriageReturns(t *testing.T) {
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct{ name, in string }{
		{"lone CR", "n 0\r"},
		{"CRLF", "first\r\nsecond"},
		{"trailing CRLF", "only\r\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, ln := range p.WrapTextIn("", tc.in, 14, 200) {
				for _, r := range ln {
					if r == '\r' {
						t.Errorf("line %q kept a carriage return; it renders as a glyph and overflows", ln)
					}
				}
			}
		})
	}

	// The concrete overflow, stated as width: the line has to fit what it was
	// given, where before it exceeded it by exactly the width of the CR.
	const w = 20
	for _, ln := range p.WrapTextIn("", "n 0\r", 14, w) {
		if got := p.MeasureWidthIn("", ln, 14); got > w {
			t.Errorf("line %q measures %v, past the %v it was wrapped to", ln, got, w)
		}
	}
}
