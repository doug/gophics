package text

import "github.com/go-text/typesetting/language"

// Script is the writing system a rune belongs to, as its ISO 15924 four-letter
// tag: "Latn", "Cyrl", "Grek", "Hani", "Arab", "Hebr", "Deva", "Thai".
//
// Script, not rune, is the unit font fallback works in. A face either covers a
// script or it does not, so a chain missing Han is missing all 90,000 of its
// runes; a diagnostic keyed on the rune would repeat itself a thousand times
// and an on-demand font fetch keyed on the rune would issue a thousand
// identical requests, where one answer per script settles the matter.
//
// The tag is a string rather than an enum because it is already the name the
// font world uses — it needs no translation on its way into a URL path, a CSS
// `unicode-range` comment or a log line — and because it stays correct for
// scripts gophics has never heard of: a rune from a Unicode block added after
// this file was written classifies itself.
type Script string

const (
	// ScriptCommon ("Zyyy") is the tag for runes belonging to no single
	// script: digits, most punctuation, arrows, mathematical operators and
	// box drawing. A face chosen for it is a symbol font, not a text font.
	ScriptCommon Script = "Zyyy"
	// ScriptUnknown ("Zzzz") is the tag for unassigned code points and for
	// anything the table cannot place. A fallback keyed on it will not find
	// a font, which is the correct outcome.
	ScriptUnknown Script = "Zzzz"
	// ScriptEmoji ("Zsye", "Symbols, emoji style") is reported for runes
	// that are script-neutral but pictographic, so that emoji do not land in
	// the same bucket as "→" and ask a symbol font to draw a face.
	//
	// Note that resolving a face for Zsye is not the same as rendering it.
	// The common system emoji fonts carry no outlines, so AppendGlyphPath
	// emits nothing for them — blank, not tofu. What draws instead depends on
	// the table: a bitmap face (Apple Color Emoji's sbix, Android's CBDT Noto
	// Color Emoji) is blitted through ColorGlyphImage, a COLR face (Windows'
	// Segoe UI Emoji) is not drawn at all, and a monochrome outline face such
	// as Noto Emoji goes down the ordinary outline path. See coloremoji.go.
	ScriptEmoji Script = "Zsye"
)

// ScriptOf reports the script of r.
//
// The classification itself is go-text/typesetting's UAX #24 table, which the
// segmenter already carries in every gophics binary, so this costs no new data
// — the point of deriving the answer there rather than keeping a second table
// here.
func ScriptOf(r rune) Script {
	s := language.LookupScript(r)
	// A script-neutral rune (Common) or one that inherits its script from a
	// neighbour (Inherited, e.g. a variation selector) gets no useful name
	// out of the table, and pictographs are the half of that set worth
	// separating — they are the ones an app is most likely to be missing a
	// font for.
	if !s.Strong() && pictographic(r) {
		return ScriptEmoji
	}
	return Script(s.String())
}

// pictographic reports whether r is emoji-ish.
//
// This is coarse on purpose: whole blocks, not the Emoji_Presentation property,
// which would mean vendoring a second Unicode table to split "✓" from "✔️".
// The only consumer is the choice of a font bucket, and the blocks below are
// overwhelmingly pictographic — a dingbat served out of an emoji font is the
// right answer anyway, which is what makes the imprecision affordable.
func pictographic(r rune) bool {
	switch {
	case r >= 0x1F000 && r <= 0x1FAFF: // emoji, pictographs, regional indicators
		return true
	case r >= 0x2600 && r <= 0x27BF: // Miscellaneous Symbols, Dingbats
		return true
	case r >= 0x2B00 && r <= 0x2BFF: // Miscellaneous Symbols and Arrows (⭐ ⬛ ⭕)
		return true
	case r == 0xFE0F || r == 0x20E3: // VS16 and the combining keycap
		return true
	}
	return false
}
