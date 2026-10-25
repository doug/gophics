package ui

import (
	"runtime"
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Colour emoji ------------------------------------------------------------

// emojiRow is the sample sheet. Every glyph here is emoji-presentation by
// default, so none of them can quietly render as a monochrome outline and make
// the sheet look like a font bug when it is really a Unicode default.
var emojiRow = []string{"🚀", "🎉", "🌍", "🐙", "🍜", "🎸", "🧭", "🐢"}

const (
	emojiBig    = 38
	emojiInline = 18
	// emojiStrike is drawn at every size in emojiStrikeSizes. One glyph rather
	// than four different ones, because the row is about the font's bitmap
	// strikes and not about the glyphs: the same picture at four sizes shows
	// the painter picking a strike, where four pictures would just be a row.
	emojiStrike = "🐙"
)

var emojiStrikeSizes = []float32{14, 20, 28, 40}

// emojiSheetHeight is fixed because a Canvas needs its height before Draw runs,
// and the fallback has to occupy the same box as the sheet it replaces — a
// section that changed height depending on the machine's fonts would reflow the
// whole page under the reader.
const emojiSheetHeight = 200

// emojiSection draws colour emoji if this build can, and says why not if it
// cannot.
//
// It exists because "nil is a real answer" is the framework's central idea and
// this is the one place a reader can watch it decide something visible. Emoji
// support is not a property of gophics but of the font chain underneath it:
// present on macOS and iOS (sbix) and Android (CBDT), absent on a stock Windows
// build whose system emoji font is COLR, and absent in a browser, which will
// not hand a WebAssembly page the bytes of an installed font. So the section
// asks paint.Emoji before it draws, and shows the honest answer either way
// rather than a row of tofu boxes.
type emojiSection struct{}

func (emojiSection) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	return sectionColumn(
		groupLabel("Bitmap colour glyphs"),
		theme.Body("A colour emoji is a picture, not an outline, so it is blitted "+
			"from the font's bitmap table instead of filled as a path. gophics "+
			"draws the two bitmap formats — sbix, which Apple Color Emoji uses, "+
			"and CBDT, which Android's Noto Color Emoji uses."),
		widget.Sized{H: 8},
		theme.Body("It does not draw COLR, which is a paint graph of layers and "+
			"gradients rather than pixels. That is the whole of the difference "+
			"between a platform where this section shows emoji and one where it "+
			"explains itself: Windows' Segoe UI Emoji and recent Noto are COLR."),
		widget.Sized{H: 10},
		widget.Canvas{H: emojiSheetHeight, Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
			drawEmojiSheet(c, size, th)
		}},

		groupLabel("Asking before drawing"),
		theme.Body("paint.Emoji(c) is an opt-in capability, like paint.Blends: it "+
			"reports whether a sample will reach a colour glyph on this canvas's "+
			"font chain. Measuring cannot answer that — a missing glyph has ink "+
			"too, which is exactly what a tofu box is — so a widget that wants to "+
			"fall back to words has to ask."),
		widget.Sized{H: 8},
		theme.Body("This section is built on that answer. The app sets "+
			"Config.SystemFonts so the chain can reach the machine's emoji face; "+
			"on the web the equivalent is Config.WebFonts, which fetches a face "+
			"per script on demand."),
		widget.Sized{H: 4},
	)
}

// drawEmojiSheet draws three rows — a sheet, an inline run, and one glyph at
// four sizes — or the fallback, in the same box.
func drawEmojiSheet(c paint.Canvas, size geom.Size, th theme.Theme) {
	sheet := strings.Join(emojiRow, " ")
	e := paint.Emoji(c)
	if e == nil || !e.HasColorEmoji("", sheet, emojiBig) {
		drawNoEmojiFace(c, size, th)
		return
	}

	// One TextIn for the whole row rather than eight: the row is a single
	// shaped run, so it also shows the emoji advancing like glyphs and the
	// spaces between them advancing like spaces.
	centeredRun(c, sheet, emojiBig, 48, size.W, th.Text)
	centeredLabel(c, "one run, eight glyphs", geom.Pt{X: size.W / 2, Y: 66}, size.W, th)

	centeredRun(c, "an emoji 🐙 sits on the baseline", emojiInline, 104, size.W, th.Text)
	centeredLabel(c, "mixed run: emoji and outlines, one baseline",
		geom.Pt{X: size.W / 2, Y: 122}, size.W, th)

	// Sizes share a baseline so the strike changes are comparable: the font
	// stores several bitmap strikes and the painter decodes the one nearest the
	// device pixel size, which is why these stay sharp instead of blurring up
	// from one.
	const strikeGap = 8
	w := -float32(strikeGap) // no gap after the last one
	for _, s := range emojiStrikeSizes {
		w += c.MeasureWidthIn("", emojiStrike, s) + strikeGap
	}
	x := (size.W - w) / 2
	if x < 0 {
		x = 0
	}
	for _, s := range emojiStrikeSizes {
		c.TextIn("", emojiStrike, geom.Pt{X: x, Y: 172}, s, th.Text)
		x += c.MeasureWidthIn("", emojiStrike, s) + strikeGap
	}
	centeredLabel(c, "one glyph, four sizes, four strikes",
		geom.Pt{X: size.W / 2, Y: 190}, size.W, th)
}

// drawNoEmojiFace fills the sheet's box with the reason there is nothing to
// show. The reason is specific to the platform because "no emoji" has three
// different causes and only one of them is something the reader could fix.
func drawNoEmojiFace(c paint.Canvas, size geom.Size, th theme.Theme) {
	reason := "No colour-bitmap emoji face in the fallback chain."
	fix := "Register one with Config.Fallbacks, or set Config.SystemFonts."
	switch runtime.GOOS {
	case "js":
		reason = "A browser will not give a WebAssembly page an installed font."
		fix = "Fetch a face per script with Config.WebFonts."
	case "windows":
		reason = "Segoe UI Emoji is a COLR font, which this build does not rasterize."
		fix = "Register a bitmap or monochrome emoji face with Config.Fallbacks."
	}
	y := size.H / 2
	centeredRun(c, reason, 13, y-6, size.W, th.Text)
	centeredRun(c, fix, 13, y+14, size.W, th.Muted)
	centeredLabel(c, "paint.Emoji reported no colour face — so this is words, not tofu",
		geom.Pt{X: size.W / 2, Y: y + 42}, size.W, th)
}

// centeredRun draws s centred horizontally with its baseline at y. It differs
// from centeredLabel in taking a size and colour rather than being the small
// muted caption, and in not left-aligning when it overflows: these rows are
// drawn inside a clip, so a long line is better cropped evenly on both sides
// than pushed off one edge.
func centeredRun(c paint.Canvas, s string, size, y, maxW float32, col paint.Color) {
	w := c.MeasureWidthIn("", s, size)
	c.TextIn("", s, geom.Pt{X: (maxW - w) / 2, Y: y}, size, col)
}
