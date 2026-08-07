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
	"os"
	"path/filepath"

	"github.com/go-text/typesetting/di"
	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/fontscan"
	"github.com/go-text/typesetting/shaping"
	"golang.org/x/image/math/fixed"
)

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
}

// Shaper shapes text through a font fallback chain, optionally extended by
// the system's fonts (UseSystemFonts). It caches segmentation and wrapping
// state; it is not safe for concurrent use (UI goroutine only).
type Shaper struct {
	fonts  []*Font
	system *fontscan.FontMap
	byFace map[*font.Face]*Font
	hb     shaping.HarfbuzzShaper
	seg    shaping.Segmenter
	wrap   shaping.LineWrapper
}

// NewShaper returns a shaper over the given fallback chain; fonts[0] is the
// primary. At least one font is required before shaping.
func NewShaper(fonts ...*Font) *Shaper {
	return &Shaper{fonts: fonts}
}

// SetFonts replaces the fallback chain.
func (s *Shaper) SetFonts(fonts ...*Font) { s.fonts = fonts }

// UseSystemFonts extends the fallback chain with the platform's installed
// fonts (via fontscan): runes not covered by the explicit chain — CJK,
// emoji, symbols — resolve to a system font. cacheDir holds fontscan's
// index ("" uses the OS user cache dir). Scanning is slow the first time
// and cached afterward.
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
	s.system = fm
	return nil
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

func (s *Shaper) shapeRuns(runes []rune, size float32) []shaping.Output {
	inputs := s.seg.Split(shaping.Input{
		Text:      runes,
		RunEnd:    len(runes),
		Size:      fx(size),
		Direction: di.DirectionLTR,
	}, fontmap{s})
	outs := make([]shaping.Output, len(inputs))
	for i, in := range inputs {
		outs[i] = s.hb.Shape(in)
	}
	return outs
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
	return s.assemble(s.shapeRuns(runes, size), 0)
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
			outs := s.shapeRuns(para, size)
			if maxWidth <= 0 || maxWidth > 1e8 {
				l := s.assemble(outs, start)
				lines = append(lines, l)
			} else {
				wrapped, _ := s.wrap.WrapParagraphF(shaping.WrapConfig{}, fx(maxWidth), para,
					shaping.NewSliceIterator(outs))
				for _, wl := range wrapped {
					lines = append(lines, s.assemble(wl, start))
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
// bounds. runeOffset shifts cluster/rune indices into the full string.
func (s *Shaper) assemble(runs []shaping.Output, runeOffset int) Line {
	visual := visualOrder(runs)
	var l Line
	l.Start = 1<<31 - 1
	var pen float32
	for _, run := range visual {
		rtl := run.Direction == di.DirectionRTL
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
		_ = rtl
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

// visualOrder reorders logical runs for display: maximal sequences of RTL
// runs are reversed. This handles the common single-embedding-level bidi
// case; full UBA multi-level reordering arrives with paragraph-level
// direction support.
func visualOrder(runs []shaping.Output) []shaping.Output {
	out := make([]shaping.Output, 0, len(runs))
	i := 0
	for i < len(runs) {
		if runs[i].Direction != di.DirectionRTL {
			out = append(out, runs[i])
			i++
			continue
		}
		j := i
		for j < len(runs) && runs[j].Direction == di.DirectionRTL {
			j++
		}
		for k := j - 1; k >= i; k-- {
			out = append(out, runs[k])
		}
		i = j
	}
	return out
}
