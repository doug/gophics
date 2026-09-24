// Package text is gophics's text stack: shaping, bidi, font fallback, and
// line breaking, built on go-text/typesetting (the shared pure-Go
// HarfBuzz-family stack used by Gio, Fyne, and Ebitengine). It exists
// because the rendering backend's own shaper handles Latin only: gophics
// shapes here and hands positioned glyph outlines to the paint backend.
//
// Coordinates follow gophics conventions: logical pixels, y down, line
// origin at the left end of the baseline.
package text

import (
	"bytes"
	"fmt"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
	"golang.org/x/text/unicode/bidi"
)

// Direction is a paragraph's base writing direction. It decides two things a
// per-run bidi analysis cannot: how runs of opposite direction are ordered
// relative to each other, and which edge a line is anchored to.
type Direction uint8

const (
	// DirAuto derives the base direction from the text itself — the first
	// strong character wins (UAX #9 rules P2/P3), which is what a browser
	// does for dir="auto". This is the zero value, so a Shaper handles
	// Arabic and Hebrew correctly without being told about them.
	DirAuto Direction = iota
	// DirLTR forces a left-to-right base direction.
	DirLTR
	// DirRTL forces a right-to-left base direction — the setting a UI in an
	// RTL locale wants even for the strings that happen to be Latin.
	DirRTL
)

// baseDirection resolves a paragraph's base direction the way UAX #9 does:
// scan for the first strong character, skipping anything inside an isolate,
// and fall back to LTR when the text has no strong character at all (digits
// and punctuation alone decide nothing).
func baseDirection(runes []rune) di.Direction {
	depth := 0
	for _, r := range runes {
		p, _ := bidi.LookupRune(r)
		switch p.Class() {
		case bidi.LRI, bidi.RLI, bidi.FSI:
			depth++
		case bidi.PDI:
			if depth > 0 {
				depth--
			}
		case bidi.L:
			if depth == 0 {
				return di.DirectionLTR
			}
		case bidi.R, bidi.AL:
			if depth == 0 {
				return di.DirectionRTL
			}
		}
	}
	return di.DirectionLTR
}

// systemFontMap is the platform system-font fallback (satisfied by
// fontscan.FontMap on desktop). Abstracting it keeps the heavy fontscan
// package — OS font scanning that only desktop uses — out of the wasm binary;
// see UseSystemFonts in system_native.go / system_js.go.
type systemFontMap interface {
	ResolveFace(r rune) *font.Face
}

// Font is a parsed font usable for shaping and outline extraction.
type Font struct {
	face *font.Face
}

// Parse parses TTF/OTF font data.
func Parse(data []byte) (*Font, error) {
	face, err := font.ParseTTF(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("text: parse font: %w", err)
	}
	return &Font{face: face}, nil
}

// Extents returns the font's line metrics at the given size, y-down:
// ascent above the baseline, descent below (both positive), plus line gap.
func (f *Font) Extents(size float32) (ascent, descent, gap float32) {
	ext, ok := f.face.FontHExtents()
	if !ok {
		return size * 0.8, size * 0.2, 0
	}
	scale := size / float32(f.face.Upem())
	return ext.Ascender * scale, -ext.Descender * scale, ext.LineGap * scale
}

// HasGlyph reports whether the font maps r to a glyph.
func (f *Font) HasGlyph(r rune) bool {
	_, ok := f.face.NominalGlyph(r)
	return ok
}

// NominalGID returns the unshaped cmap glyph for r (for tests/tools).
func (f *Font) NominalGID(r rune) (uint32, bool) {
	gid, ok := f.face.NominalGlyph(r)
	return uint32(gid), ok
}

// PathSink receives a glyph outline as path commands, y down.
type PathSink interface {
	MoveTo(x, y float32)
	LineTo(x, y float32)
	QuadTo(cx, cy, x, y float32)
	CubeTo(c1x, c1y, c2x, c2y, x, y float32)
	Close()
}

// AppendGlyphPath emits gid's outline at the given size into sink, with the
// glyph origin (baseline pen position) at (dx, dy). Bitmap and SVG glyphs
// (color emoji) are not yet supported and emit nothing.
func (f *Font) AppendGlyphPath(sink PathSink, gid uint32, size float32, dx, dy float32) {
	data := f.face.GlyphData(font.GID(gid))
	outline, ok := data.(font.GlyphOutline)
	if !ok {
		return
	}
	scale := size / float32(f.face.Upem())
	open := false
	x := func(p ot.SegmentPoint) float32 { return dx + p.X*scale }
	y := func(p ot.SegmentPoint) float32 { return dy - p.Y*scale }
	for _, seg := range outline.Segments {
		switch seg.Op {
		case ot.SegmentOpMoveTo:
			if open {
				sink.Close()
			}
			sink.MoveTo(x(seg.Args[0]), y(seg.Args[0]))
			open = true
		case ot.SegmentOpLineTo:
			sink.LineTo(x(seg.Args[0]), y(seg.Args[0]))
		case ot.SegmentOpQuadTo:
			sink.QuadTo(x(seg.Args[0]), y(seg.Args[0]), x(seg.Args[1]), y(seg.Args[1]))
		case ot.SegmentOpCubeTo:
			sink.CubeTo(x(seg.Args[0]), y(seg.Args[0]), x(seg.Args[1]), y(seg.Args[1]),
				x(seg.Args[2]), y(seg.Args[2]))
		}
	}
	if open {
		sink.Close()
	}
}

// Glyph is one positioned glyph on a line.
type Glyph struct {
	Font *Font
	GID  uint32
	// Cluster is the rune index (into the shaped string) of the cluster
	// this glyph belongs to — the unit for caret placement and hit testing.
	Cluster int
	// X, Y position the glyph origin relative to the line origin (baseline).
	X, Y float32
	// Advance is the pen advance contributed by this glyph.
	Advance float32
}

// Line is one shaped, positioned line of text.
type Line struct {
	Glyphs []Glyph
	// Width is the total advance width.
	Width float32
	// Ascent/Descent/Gap are the maxima over the fonts used (y-down:
	// both ascent and descent positive).
	Ascent, Descent, Gap float32
	// Start/End are the rune range of the source string on this line.
	Start, End int
	// RTL reports that the line's base direction is right-to-left, so it
	// should be anchored to the right edge of its box when the caller is
	// aligning to the "start" of the text rather than to a fixed side.
	RTL bool
}

// Shaper shapes text through a font fallback chain, optionally extended by
// the system's fonts (UseSystemFonts). It caches segmentation and wrapping
// state; it is not safe for concurrent use (UI goroutine only).
type Shaper struct {
	fonts  []*Font
	system systemFontMap
	byFace map[*font.Face]*Font
	dir    Direction
	hb     shaping.HarfbuzzShaper
	seg    shaping.Segmenter
	wrap   shaping.LineWrapper
}

// ShareSystemFonts gives s the system font map other loaded with
// UseSystemFonts (or nothing, if other has none), so one scan serves every
// Shaper of an owner. The two must then be used from the same goroutine: the
// map's faces carry shaping caches that are not safe to write concurrently.
func (s *Shaper) ShareSystemFonts(other *Shaper) {
	if other == nil {
		return
	}
	s.system = other.system
}

// NewShaper returns a shaper over the given fallback chain; fonts[0] is the
// primary. At least one font is required before shaping.
func NewShaper(fonts ...*Font) *Shaper {
	return &Shaper{fonts: fonts}
}

// SetFonts replaces the fallback chain.
func (s *Shaper) SetFonts(fonts ...*Font) { s.fonts = fonts }

// SetDirection sets the base writing direction for subsequent shaping.
// The default, DirAuto, reads it off each paragraph's own content.
func (s *Shaper) SetDirection(d Direction) { s.dir = d }

// baseDir resolves the configured direction against the text being shaped.
func (s *Shaper) baseDir(runes []rune) di.Direction {
	switch s.dir {
	case DirLTR:
		return di.DirectionLTR
	case DirRTL:
		return di.DirectionRTL
	}
	return baseDirection(runes)
}

// Primary returns the primary font, or nil.
func (s *Shaper) Primary() *Font {
	if len(s.fonts) == 0 {
		return nil
	}
	return s.fonts[0]
}

// fontmap adapts a Shaper to shaping.Fontmap. It exists so the dependency's
// ResolveFace(rune) *font.Face lives on an unexported adapter, not on Shaper's
// public API — the Shaper never leaks *font.Face to gophics callers.
type fontmap struct{ s *Shaper }

func (f fontmap) ResolveFace(r rune) *font.Face { return f.s.resolveFace(r) }

// resolveFace picks a face for r: first explicit font with a glyph for it, then
// the system font map (if enabled), else the primary.
func (s *Shaper) resolveFace(r rune) *font.Face {
	for _, f := range s.fonts {
		if f.HasGlyph(r) {
			return f.face
		}
	}
	if s.system != nil {
		if face := s.system.ResolveFace(r); face != nil {
			return face
		}
	}
	return s.fonts[0].face
}

func fx(v float32) fixed.Int26_6 { return fixed.Int26_6(v * 64) }

func unfx(v fixed.Int26_6) float32 { return float32(v) / 64 }

func (s *Shaper) shapeRuns(runes []rune, size float32, base di.Direction) []shaping.Output {
	inputs := s.seg.Split(shaping.Input{
		Text:   runes,
		RunEnd: len(runes),
		Size:   fx(size),
		// The segmenter runs the bidi algorithm with this as the paragraph
		// direction, so it also decides where neutral characters (spaces,
		// punctuation, digits) attach when the text has no strong character
		// to anchor them.
		Direction: base,
	}, fontmap{s})
	if base == di.DirectionLTR {
		inputs = splitNumbers(inputs, runes)
	}
	outs := make([]shaping.Output, len(inputs))
	for i, in := range inputs {
		outs[i] = s.hb.Shape(in)
	}
	return outs
}

// splitNumbers cuts a left-to-right run that follows right-to-left text where
// its leading number ends, so the number can be reordered on its own.
//
// The segmenter splits runs by direction only, and a number inside Arabic or
// Hebrew text in a left-to-right paragraph has the same direction as the Latin
// that follows it — but not the same embedding level. UAX #9 gives the number
// level 2 (it belongs to the RTL phrase around it) and the Latin level 0, and
// rule L2 needs that boundary to reverse the phrase as one block. Without the
// cut the number is shaped together with the Latin, and the Arabic words on
// either side of it end up in the wrong order.
func splitNumbers(inputs []shaping.Input, runes []rune) []shaping.Input {
	out := make([]shaping.Input, 0, len(inputs))
	for _, in := range inputs {
		if in.Direction == di.DirectionLTR {
			if strong := strongBefore(runes, in.RunStart); strong != bidi.L {
				n := numericPrefix(runes[in.RunStart:in.RunEnd], strong == bidi.AL)
				if n > 0 && n < in.RunEnd-in.RunStart {
					head, tail := in, in
					head.RunEnd = in.RunStart + n
					tail.RunStart = head.RunEnd
					out = append(out, head, tail)
					continue
				}
			}
		}
		out = append(out, in)
	}
	return out
}

// strongBefore returns the class of the nearest strong character (L, R or AL)
// before rune index i, or L when there is none: the start of an LTR paragraph
// counts as L (UAX #9's sos).
func strongBefore(runes []rune, i int) bidi.Class {
	for i--; i >= 0; i-- {
		p, _ := bidi.LookupRune(runes[i])
		switch c := p.Class(); c {
		case bidi.L, bidi.R, bidi.AL:
			return c
		}
	}
	return bidi.L
}

// numericPrefix returns how many leading runes of a run resolve to a number
// (EN or AN) under UAX #9's weak-type rules W1–W6, for a run whose nearest
// preceding strong character is R or AL — afterAL says which, because after
// an Arabic letter European digits become Arabic numbers (W2), and Arabic
// numbers do not absorb currency signs and other terminators (W5). Everything
// after the number resolves to L or to a neutral at the paragraph level, so
// the prefix is exactly the part that sits one level deeper.
func numericPrefix(runes []rune, afterAL bool) int {
	cls := make([]bidi.Class, len(runes))
	for i, r := range runes {
		p, _ := bidi.LookupRune(r)
		c := p.Class()
		switch c {
		case bidi.NSM: // W1: a non-spacing mark takes the class before it
			c = bidi.ON
			if i > 0 {
				c = cls[i-1]
			}
		case bidi.EN:
			if afterAL {
				c = bidi.AN // W2
			}
		}
		cls[i] = c
	}
	// W4: a single separator between two numbers of one kind joins them.
	for i := 1; i+1 < len(cls); i++ {
		switch cls[i] {
		case bidi.ES:
			if cls[i-1] == bidi.EN && cls[i+1] == bidi.EN {
				cls[i] = bidi.EN
			}
		case bidi.CS:
			if cls[i-1] == cls[i+1] && (cls[i-1] == bidi.EN || cls[i-1] == bidi.AN) {
				cls[i] = cls[i-1]
			}
		}
	}
	// W5: terminators adjacent to a European number become part of it.
	for i := 0; i < len(cls); {
		if cls[i] != bidi.ET {
			i++
			continue
		}
		j := i
		for j < len(cls) && cls[j] == bidi.ET {
			j++
		}
		if (i > 0 && cls[i-1] == bidi.EN) || (j < len(cls) && cls[j] == bidi.EN) {
			for k := i; k < j; k++ {
				cls[k] = bidi.EN
			}
		}
		i = j
	}
	// W6 turns every separator still standing into a neutral, which is where
	// the number ends.
	n := 0
	for n < len(cls) && (cls[n] == bidi.EN || cls[n] == bidi.AN || cls[n] == bidi.BN) {
		n++
	}
	return n
}

// runLevel returns the UAX #9 embedding level of a shaped run within a
// paragraph of the given base direction. Runs against the base direction sit
// one level above it; a number that the bidi algorithm attached to preceding
// RTL text in an LTR paragraph sits two above (see splitNumbers). Explicit
// embeddings and isolates are not modelled, so levels never exceed 2.
func runLevel(run *shaping.Output, runes []rune, base di.Direction) int {
	if run.Direction == di.DirectionRTL {
		return 1
	}
	if base == di.DirectionRTL {
		return 2
	}
	off, n := run.Runes.Offset, run.Runes.Count
	if strong := strongBefore(runes, off); strong != bidi.L && numericPrefix(runes[off:off+n], strong == bidi.AL) == n {
		return 2
	}
	return 0
}

// Line shapes str as a single line (no wrapping; newlines are not special).
func (s *Shaper) Line(str string, size float32) Line {
	runes := []rune(str)
	if len(runes) == 0 || len(s.fonts) == 0 {
		if p := s.Primary(); p != nil {
			a, d, g := p.Extents(size)
			return Line{Ascent: a, Descent: d, Gap: g}
		}
		return Line{}
	}
	base := s.baseDir(runes)
	return s.assemble(s.shapeRuns(runes, size, base), 0, base, runes)
}

// Paragraph shapes and wraps str to maxWidth (Inf or <= 0 disables
// wrapping). Newlines force breaks. Line Start/End are rune indices into
// str; a trailing newline yields an empty final line.
func (s *Shaper) Paragraph(str string, size, maxWidth float32) []Line {
	if len(s.fonts) == 0 {
		return nil
	}
	runes := []rune(str)
	var lines []Line
	start := 0
	for start <= len(runes) {
		end := start
		for end < len(runes) && runes[end] != '\n' {
			end++
		}
		para := runes[start:end]
		if len(para) == 0 {
			l := s.Line("", size)
			l.Start, l.End = start, start
			lines = append(lines, l)
		} else {
			// Base direction is resolved per paragraph, not per string: UAX #9
			// scopes P2/P3 to the paragraph, so a Hebrew line and a Latin line
			// in one text block each get their own answer.
			base := s.baseDir(para)
			outs := s.shapeRuns(para, size, base)
			if maxWidth <= 0 || maxWidth > 1e8 {
				l := s.assemble(outs, start, base, para)
				lines = append(lines, l)
			} else {
				wrapped, _ := s.wrap.WrapParagraphF(shaping.WrapConfig{}, fx(maxWidth), para,
					shaping.NewSliceIterator(outs))
				for _, wl := range wrapped {
					lines = append(lines, s.assemble(wl, start, base, para))
				}
			}
		}
		start = end + 1
		if end == len(runes) {
			break
		}
	}
	return lines
}

// assemble positions the runs of one line in visual order and computes
// bounds. runeOffset shifts cluster/rune indices into the full string; para is
// the paragraph the runs were shaped from (their Runes.Offset indexes it).
func (s *Shaper) assemble(runs []shaping.Output, runeOffset int, base di.Direction, para []rune) Line {
	levels := make([]int, len(runs))
	for i := range runs {
		levels[i] = runLevel(&runs[i], para, base)
	}
	visual := visualOrder(runs, levels)
	var l Line
	l.RTL = base == di.DirectionRTL
	l.Start = 1<<31 - 1
	var pen float32
	for _, run := range visual {
		f := s.fontFor(run.Face)
		for _, g := range run.Glyphs {
			l.Glyphs = append(l.Glyphs, Glyph{
				Font:    f,
				GID:     uint32(g.GlyphID),
				Cluster: runeOffset + g.TextIndex(),
				X:       pen + unfx(g.XOffset),
				Y:       -unfx(g.YOffset),
				Advance: unfx(g.Advance),
			})
			pen += unfx(g.Advance)
		}
		if a := unfx(run.LineBounds.Ascent); a > l.Ascent {
			l.Ascent = a
		}
		if d := -unfx(run.LineBounds.Descent); d > l.Descent {
			l.Descent = d
		}
		if g := unfx(run.LineBounds.Gap); g > l.Gap {
			l.Gap = g
		}
		if o := runeOffset + run.Runes.Offset; o < l.Start {
			l.Start = o
		}
		if e := runeOffset + run.Runes.Offset + run.Runes.Count; e > l.End {
			l.End = e
		}
	}
	if len(l.Glyphs) == 0 {
		l.Start = runeOffset
		l.End = runeOffset
	}
	l.Width = pen
	return l
}

func (s *Shaper) fontFor(face *font.Face) *Font {
	for _, f := range s.fonts {
		if f.face == face {
			return f
		}
	}
	// System-resolved faces get lazily created wrappers (stable per face,
	// so glyph rendering and caching work unchanged).
	if f, ok := s.byFace[face]; ok {
		return f
	}
	if s.system != nil {
		if s.byFace == nil {
			s.byFace = map[*font.Face]*Font{}
		}
		f := &Font{face: face}
		s.byFace[face] = f
		return f
	}
	return s.Primary()
}

// visualOrder reorders logical runs into the order they are painted, left to
// right, applying UAX #9 rule L2: from the highest embedding level down to the
// lowest odd one, reverse every maximal sequence of runs at that level or
// higher. levels[i] is the level of runs[i] (see runLevel).
//
// For an LTR base with only levels 0 and 1 that is just "reverse each maximal
// RTL sequence". For an RTL base (levels 1 and 2) it is two passes: reverse
// each maximal LTR sequence in place — which keeps several adjacent LTR runs
// in their own relative order — and then reverse the whole line, because every
// run sits at level 1 or above. That is what makes an Arabic sentence with an
// English phrase in it read correctly: the phrase stays LTR internally while
// the sentence around it runs right to left.
//
// A number inside RTL text in an LTR paragraph is the three-level case: the
// number (level 2) reverses alone, which is a no-op, and then the RTL runs on
// both sides of it reverse together with it as one block, so the phrase keeps
// its reading order around the number. Reversing each RTL run on its own would
// leave the two halves of the phrase swapped.
func visualOrder(runs []shaping.Output, levels []int) []shaping.Output {
	out := make([]shaping.Output, len(runs))
	copy(out, runs)
	lv := make([]int, len(levels))
	copy(lv, levels)

	highest, lowest := 0, 0
	for i, l := range lv {
		if i == 0 || l < lowest {
			lowest = l
		}
		highest = max(highest, l)
	}
	// The lowest odd level: with nothing at an odd level (all-LTR runs on an
	// RTL base) nothing reverses, and the runs keep their logical order.
	for level := highest; level >= lowest|1; level-- {
		for i := 0; i < len(out); {
			if lv[i] < level {
				i++
				continue
			}
			j := i
			for j < len(out) && lv[j] >= level {
				j++
			}
			reverseRuns(out[i:j], lv[i:j])
			i = j
		}
	}
	return out
}

func reverseRuns(runs []shaping.Output, levels []int) {
	for i, j := 0, len(runs)-1; i < j; i, j = i+1, j-1 {
		runs[i], runs[j] = runs[j], runs[i]
		levels[i], levels[j] = levels[j], levels[i]
	}
}
