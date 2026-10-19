package text

import "testing"

func TestScriptOfNamesTheWritingSystem(t *testing.T) {
	for _, c := range []struct {
		r    rune
		want Script
	}{
		{'a', "Latn"},
		{'Ω', "Grek"},
		{'д', "Cyrl"},
		{'中', "Hani"},
		{'あ', "Hira"},
		{'カ', "Kana"},
		{'한', "Hang"},
		{'ع', "Arab"},
		{'א', "Hebr"},
		{'ह', "Deva"},
		{'ก', "Thai"},
		// Script-neutral but not pictographic: an arrow belongs with the
		// digits and the punctuation, not with the emoji.
		{'→', ScriptCommon},
		{'0', ScriptCommon},
		{'😀', ScriptEmoji},
		{'❤', ScriptEmoji},
		{'⭐', ScriptEmoji},
		{0x1F1FA, ScriptEmoji}, // regional indicator U
		// Unassigned: the table must not invent a script for it, because a
		// fallback that went looking for one would always fail.
		{0x0EFF, ScriptUnknown},
	} {
		if got := ScriptOf(c.r); got != c.want {
			t.Errorf("ScriptOf(U+%04X %q) = %q, want %q", c.r, c.r, got, c.want)
		}
	}
}

// The runes a font is not expected to draw must not be reported as missing, or
// the one message that matters drowns in healthy Latin text.
func TestUnreportableSkipsTheInvisibles(t *testing.T) {
	for _, r := range []rune{
		'\n', '\t', 0x00, 0x7F, 0x85, 0x00AD,
		0x200B, 0x200D, 0x200E, 0x2028, 0x202A, 0x2060, 0xFE0F, 0xFEFF, 0xE0065,
	} {
		if !unreportable(r) {
			t.Errorf("U+%04X should not be reported as a missing glyph", r)
		}
	}
	for _, r := range []rune{' ', 'a', '中', '😀', '→'} {
		if unreportable(r) {
			t.Errorf("U+%04X %q is a real glyph and must stay reportable", r, r)
		}
	}
}
