package text

import "log"

// missingScriptCap bounds how many distinct scripts one Shaper will ever
// report. Twelve is more than any real chain is missing and far fewer than a
// stream of mojibake can produce: text decoded with the wrong charset lands
// runes all over the plane, and without a ceiling the "helpful" diagnostic
// becomes the thing that makes the tab unusable.
const missingScriptCap = 12

// OnMissingGlyph replaces what a Shaper does the first time it finds a rune no
// font in its chain covers — the tofu case, where shaping falls back to the
// primary face and draws the wrong glyph or a box.
//
// fn is called once per script, not once per rune (see Script), from the
// goroutine that is shaping. Passing nil silences the report entirely.
//
// There is a report at all, rather than a silent fallback, because the failure
// is otherwise undiagnosable from inside the program: the shaper returns
// glyphs, the painter draws them, nothing errors, and the developer is left
// looking at a row of boxes with no string to search for. On the web that was
// the whole of the system-font story — UseSystemFonts there was a no-op that
// returned nil, so an app rendering Japanese had no way to learn that it never
// could. The default report names the rune, its script and the three things
// that fix it; an app that would rather route it somewhere (a telemetry sink, a
// developer overlay) passes its own fn.
func (s *Shaper) OnMissingGlyph(fn func(r rune, script Script)) {
	s.onMissing, s.missingSet = fn, true
}

// reportMissing is resolveFace's last resort, called when nothing in the chain
// and nothing in the system map has a glyph for r.
//
// Everything expensive here is behind the "first time for this script" gate,
// because this sits on the per-rune shaping path. For text the chain covers it
// is never reached at all; for text it does not, it runs its map lookup once
// per script and then only the lookup.
func (s *Shaper) reportMissing(r rune) {
	fn := s.onMissing
	if !s.missingSet {
		fn = logMissingGlyph
	}
	if fn == nil || unreportable(r) {
		return
	}
	if s.missingSeen == nil {
		s.missingSeen = make(map[Script]bool, 4)
	}
	sc := ScriptOf(r)
	if s.missingSeen[sc] || len(s.missingSeen) >= missingScriptCap {
		return
	}
	s.missingSeen[sc] = true
	fn(r, sc)
}

// unreportable reports whether r is a rune no font is expected to draw, so
// that "nothing covers it" is not news.
//
// Without this the default report fires on perfectly healthy Latin text: a
// zero-width space, a soft hyphen or a bidi isolate mark is absent from most
// faces by design, the shaper discards it, and nothing is wrong. Reporting
// those would train the developer to ignore the one message that matters.
func unreportable(r rune) bool {
	switch {
	case r < 0x20, r == 0x7F: // C0 controls and DEL
		return true
	case r >= 0x80 && r <= 0x9F: // C1 controls
		return true
	case r == 0x00AD: // soft hyphen
		return true
	case r >= 0x200B && r <= 0x200F: // ZWSP/ZWNJ/ZWJ, LRM/RLM
		return true
	case r >= 0x2028 && r <= 0x202E: // line/paragraph separators, bidi embedding
		return true
	case r >= 0x2060 && r <= 0x206F: // word joiner, invisible operators, deprecated bidi
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors
		return true
	case r == 0xFEFF: // BOM / ZWNBSP
		return true
	case r >= 0xE0000 && r <= 0xE01EF: // tag characters, variation selectors supplement
		return true
	}
	return false
}

// logMissingGlyph is the default report: one line, to the standard logger that
// the rest of gophics already logs through, naming the rune so it can be
// searched for and the remedies so the reader does not have to find them.
//
// The three remedies are deliberately spelled out per platform rather than
// generically, because the right one differs: on desktop the machine already
// has the face and only has to be asked; in a browser it does not and cannot
// be made to hand it over, so the font has to come over the network.
func logMissingGlyph(r rune, script Script) {
	log.Printf("gophics/text: no font in the fallback chain covers U+%04X %q (script %s) — "+
		"it will be drawn from the primary face, which is where tofu comes from. "+
		"Register a face that covers it (app.Config.Fallbacks, paint.Painter.LoadFallbackFont), "+
		"use the machine's fonts (app.Config.SystemFonts; desktop only), or in a browser "+
		"fetch one on demand (text.Shaper.UseWebFonts). Silence with text.Shaper.OnMissingGlyph(nil).",
		r, r, script)
}
