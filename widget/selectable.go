package widget

import (
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
)

// SelectableText renders static text the user can select by dragging and copy
// with Cmd/Ctrl+C — for labels, article bodies, and any read-only text worth
// lifting out. Selection is highlighted; copy joins wrapped lines with "\n".
type SelectableText struct {
	S     string
	Font  string  // named family ("" = default)
	Size  float32 // 0 → 14
	Color paint.Color
	Wrap  bool
	// SelectionColor is the highlight fill; zero alpha → a translucent blue.
	SelectionColor paint.Color
}

func (t SelectableText) CreateState() State { return &selectableState{} }

type selectableState struct {
	StateBase[SelectableText]
	ctx           Ctx
	ref           *selRef
	anchor, focus int // linear rune offsets over the wrapped-line model
}

type selRef struct{ box *selectableBox }

func (s *selectableState) Init(ctx Ctx) { s.ctx = ctx; s.ref = &selRef{} }

func (s *selectableState) sel() (lo, hi int) {
	if s.anchor <= s.focus {
		return s.anchor, s.focus
	}
	return s.focus, s.anchor
}

func (s *selectableState) Build(ctx Ctx) Widget {
	t := s.W()
	lo, hi := s.sel()
	return Interactive{
		Handler: Handler{
			OnPress: func(p geom.Pt) {
				if s.ref.box != nil {
					o := s.ref.box.offsetAt(p)
					s.SetState(func() { s.anchor, s.focus = o, o })
				}
			},
			OnDrag: func(pos, _ geom.Pt) {
				if s.ref.box != nil {
					o := s.ref.box.offsetAt(pos)
					s.SetState(func() { s.focus = o })
				}
			},
			OnKey: func(k shell.Key) {
				if k.Kind == shell.KeyPress && k.Mods.Command() && k.Code == shell.KeyC {
					s.copy()
				}
			},
			// Double-tap selects the word under the pointer (OnPress set
			// s.focus to the tapped offset on the way in).
			OnDoubleTap: func() {
				if s.ref.box != nil {
					lo, hi := s.ref.box.wordAt(s.focus)
					s.SetState(func() { s.anchor, s.focus = lo, hi })
				}
			},
		},
		Child: selText{
			text: t.S, font: t.Font, size: t.size(), color: t.Color,
			wrap: t.Wrap, selColor: t.selectionColor(), lo: lo, hi: hi, ref: s.ref,
		},
	}
}

func (s *selectableState) copy() {
	if s.ref.box == nil {
		return
	}
	lo, hi := s.sel()
	if txt := s.ref.box.selectedText(lo, hi); txt != "" {
		if cb := s.ctx.Clipboard(); cb != nil {
			_ = cb.ClipboardWrite(txt)
		}
	}
}

func (t SelectableText) size() float32 {
	if t.Size == 0 {
		return 14
	}
	return t.Size
}

func (t SelectableText) selectionColor() paint.Color {
	if t.SelectionColor.A > 0 {
		return t.SelectionColor
	}
	return paint.Color{R: 0.36, G: 0.62, B: 0.98, A: 0.35}
}

// selText is the render widget wrapping selectableBox.
type selText struct {
	text, font      string
	size            float32
	color, selColor paint.Color
	wrap            bool
	lo, hi          int
	ref             *selRef
}

func (w selText) createBox(ctx Ctx) layout.Box {
	return &selectableBox{Painter: ctx.Painter()}
}
func (w selText) updateBox(_ Ctx, b layout.Box) {
	sb := b.(*selectableBox)
	sb.text, sb.font, sb.size = w.text, w.font, w.size
	sb.color, sb.selColor, sb.wrap = w.color, w.selColor, w.wrap
	sb.lo, sb.hi = w.lo, w.hi
	if w.ref != nil {
		w.ref.box = sb
	}
}
func (w selText) childWidgets() []Widget          { return nil }
func (w selText) attach(layout.Box, []layout.Box) {}

// selectableBox lays out wrapped text and paints a selection highlight under
// the runes in [lo, hi). Offsets are linear over the wrapped-line model: each
// line contributes len(runes)+1 (a virtual newline between lines).
type selectableBox struct {
	layout.Base
	Painter  *paint.Painter
	text     string
	font     string
	size     float32
	color    paint.Color
	selColor paint.Color
	wrap     bool
	lo, hi   int

	lines     []string
	lineStart []int
	baseline  float32
	lineH     float32
	descent   float32
	sz        geom.Size
}

func (b *selectableBox) Layout(cs layout.Constraints) geom.Size {
	m := b.Painter.MetricsIn(b.font, b.size)
	b.baseline, b.lineH, b.descent = m.Ascent, m.LineHeight(), m.Descent

	if b.wrap && cs.BoundedW() {
		b.lines = b.Painter.WrapTextIn(b.font, b.text, b.size, cs.Max.W)
	} else {
		b.lines = []string{b.text}
	}
	// Cumulative linear offsets at each line start.
	b.lineStart = b.lineStart[:0]
	off := 0
	var w float32
	for _, ln := range b.lines {
		b.lineStart = append(b.lineStart, off)
		off += len([]rune(ln)) + 1
		if lw := b.Painter.MeasureWidthIn(b.font, ln, b.size); lw > w {
			w = lw
		}
	}
	h := m.Ascent + m.Descent + float32(len(b.lines)-1)*b.lineH
	b.sz = cs.Constrain(geom.Size{W: w, H: h})
	return b.sz
}

func (b *selectableBox) Size() geom.Size { return b.sz }

// offsetAt maps a local point to a linear rune offset (clamped).
func (b *selectableBox) offsetAt(p geom.Pt) int {
	if len(b.lines) == 0 {
		return 0
	}
	li := int(p.Y / b.lineH)
	if li < 0 {
		li = 0
	}
	if li >= len(b.lines) {
		li = len(b.lines) - 1
	}
	runes := []rune(b.lines[li])
	col := len(runes)
	for i := 1; i <= len(runes); i++ {
		w := b.Painter.MeasureWidthIn(b.font, string(runes[:i]), b.size)
		prev := b.Painter.MeasureWidthIn(b.font, string(runes[:i-1]), b.size)
		if p.X < (prev+w)/2 { // past the glyph's midpoint selects the next
			col = i - 1
			break
		}
	}
	return b.lineStart[li] + col
}

// wordAt returns the linear range of the whitespace-delimited word containing
// (or immediately before) the offset — for double-tap word selection. A tap
// on whitespace returns an empty range.
func (b *selectableBox) wordAt(off int) (int, int) {
	for li := len(b.lines) - 1; li >= 0; li-- {
		start := b.lineStart[li]
		if off < start {
			continue
		}
		runes := []rune(b.lines[li])
		col := off - start
		if col > len(runes) {
			col = len(runes)
		}
		word := func(r rune) bool { return r != ' ' && r != '\t' }
		lo, hi := col, col
		for lo > 0 && word(runes[lo-1]) {
			lo--
		}
		for hi < len(runes) && word(runes[hi]) {
			hi++
		}
		return start + lo, start + hi
	}
	return off, off
}

// selectedText returns the runes in [lo, hi) joined across lines with "\n".
func (b *selectableBox) selectedText(lo, hi int) string {
	if lo >= hi {
		return ""
	}
	var out []string
	for li, ln := range b.lines {
		runes := []rune(ln)
		start := b.lineStart[li]
		a := lo - start
		z := hi - start
		if a < 0 {
			a = 0
		}
		if z > len(runes) {
			z = len(runes)
		}
		if a < z {
			out = append(out, string(runes[a:z]))
		} else if start >= lo && start < hi {
			out = append(out, "")
		}
	}
	return strings.Join(out, "\n")
}

func (b *selectableBox) Paint(c paint.Canvas, at geom.Pt) {
	for li, ln := range b.lines {
		runes := []rune(ln)
		start := b.lineStart[li]
		top := at.Y + float32(li)*b.lineH
		base := top + b.baseline
		// Selection highlight for this line.
		if b.hi > b.lo {
			a := max(b.lo-start, 0)
			z := min(b.hi-start, len(runes))
			if a < z {
				x0 := b.Painter.MeasureWidthIn(b.font, string(runes[:a]), b.size)
				x1 := b.Painter.MeasureWidthIn(b.font, string(runes[:z]), b.size)
				c.FillRect(geom.Rect{
					Min: geom.Pt{X: at.X + x0, Y: top},
					Max: geom.Pt{X: at.X + x1, Y: top + b.lineH},
				}, b.selColor)
			}
		}
		c.TextIn(b.font, ln, geom.Pt{X: at.X, Y: base}, b.size, b.color)
	}
}

func (b *selectableBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X >= 0 && p.Y >= 0 && p.X < b.sz.W && p.Y < b.sz.H {
		*hits = append(*hits, layout.Hit{Box: b, Pos: p})
	}
}
