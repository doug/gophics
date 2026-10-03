package widget

import (
	"slices"
	"strings"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/layoutbox"
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

	// Where the press landed, for anchoring the edit menu (OnLongPress and
	// OnPressEnd report no position), and the menu's dismiss.
	pressGlobal geom.Pt
	dismissMenu func()

	// Clicks are counted at press time, not taken from the tap dispatcher: a
	// double-click drag selects by words from the second press, and a tap is
	// only reported on release, after the drag has begun.
	clicks       int
	lastPress    time.Time
	lastPressPos geom.Pt
	unit         selUnit
	unitLo       int
	unitHi       int
	// handles is set when a finger made the selection, which is when the grips
	// are drawn; dragHandle is the one being dragged, -1 for none.
	handles    bool
	dragHandle int
}

type selRef struct{ box *selectableBox }

// unitRange is the span of the unit containing off: the offset itself, the
// word around it, or the whole line a triple click takes.
func (s *selectableState) unitRange(off int, u selUnit) (int, int) {
	b := s.ref.box
	if b == nil {
		return off, off
	}
	switch u {
	case selWords:
		return b.wordAt(off)
	case selParas:
		return b.lineAt(off)
	}
	return off, off
}

// selectUnitAt selects the unit under off and remembers it, so a drag that
// doubles back still covers the unit it started on.
func (s *selectableState) selectUnitAt(off int, u selUnit) {
	lo, hi := s.unitRange(off, u)
	s.anchor, s.focus = lo, hi
	s.unitLo, s.unitHi, s.unit = lo, hi, u
}

// extendByUnit grows the selection to cover the unit under off as well as the
// one the gesture started on, so a word is never cut in half mid-drag.
func (s *selectableState) extendByUnit(off int) {
	lo, hi := s.unitRange(off, s.unit)
	if lo < s.unitLo {
		s.anchor, s.focus = s.unitHi, lo
	} else {
		s.anchor, s.focus = s.unitLo, hi
	}
}

// moveHandleTo drags grip h to off, keeping the other end anchored, and
// reports which grip is being dragged after the move — pulling one end past
// the other swaps them rather than collapsing the selection.
func (s *selectableState) moveHandleTo(h, off int) int {
	lo, hi := s.sel()
	fixed := hi
	if h == 1 {
		fixed = lo
	}
	if off == fixed {
		return h // a zero-width selection would drop the grips mid-drag
	}
	s.anchor, s.focus = fixed, off
	if off < fixed {
		return 0
	}
	return 1
}

// selectAll takes the whole text, for Cmd/Ctrl+A and the menu.
func (s *selectableState) selectAll() {
	if b := s.ref.box; b != nil {
		s.anchor, s.focus, s.unit = 0, b.linearLen(), selChars
	}
}

func (s *selectableState) allSelected() bool {
	b := s.ref.box
	if b == nil {
		return false
	}
	lo, hi := s.sel()
	return lo == 0 && hi >= b.linearLen() && hi > 0
}

func (s *selectableState) Init(ctx Ctx) { s.ctx = ctx; s.ref = &selRef{}; s.dragHandle = -1 }

// Dispose takes the edit menu down with the text: the menu is an overlay
// entry beside the tree, not a descendant, so nothing else would.
func (s *selectableState) Dispose() { s.closeMenu() }

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
		Gestures: Gestures{
			// Text the pointer can act on says so before it is touched.
			Cursor: shell.CursorText,
			OnPress: func(p geom.Pt) {
				s.closeMenu()
				s.pressGlobal = ctx.Input().Pointer()
				if s.ref.box == nil {
					return
				}
				// A press on a grip adjusts the selection rather than
				// replacing it — the only way to fix one on a phone.
				if h := s.ref.box.handleAt(p); h >= 0 {
					s.dragHandle = h
					return
				}
				s.dragHandle = -1
				s.handles = false
				o := s.ref.box.offsetAt(p)
				now := time.Now()
				window := time.Duration(ctx.el.owner.Gestures.Resolved().DoubleTap * float64(time.Second))
				if now.Sub(s.lastPress) <= window && near(p, s.lastPressPos, 8) {
					s.clicks++
				} else {
					s.clicks = 1
				}
				s.lastPress, s.lastPressPos = now, p
				switch {
				case s.clicks >= 3:
					s.SetState(func() { s.selectUnitAt(o, selParas) })
				case s.clicks == 2:
					s.SetState(func() { s.selectUnitAt(o, selWords) })
				case ctx.Input().Mods()&shell.ModShift != 0:
					// Shift-click keeps the anchor and moves the far end.
					s.SetState(func() { s.focus, s.unit = o, selChars })
				default:
					s.SetState(func() { s.anchor, s.focus, s.unit = o, o, selChars })
				}
			},
			// Right-click selects the word under the pointer when nothing is
			// selected, then offers the menu.
			OnSecondaryTap: func(p geom.Pt) {
				s.closeMenu()
				s.pressGlobal = ctx.Input().Pointer()
				if b := s.ref.box; b != nil {
					if lo, hi := s.sel(); lo == hi {
						s.SetState(func() { s.selectUnitAt(b.offsetAt(p), selWords) })
					}
				}
				s.showMenu(ctx)
			},
			OnDrag: func(pos, _ geom.Pt) {
				s.closeMenu() // the selection is still moving under it
				if s.ref.box == nil {
					return
				}
				if s.dragHandle >= 0 {
					o := s.ref.box.offsetAt(pos)
					s.SetState(func() { s.dragHandle = s.moveHandleTo(s.dragHandle, o) })
					return
				}
				{
					o := s.ref.box.offsetAt(pos)
					s.SetState(func() {
						if s.unit != selChars {
							s.extendByUnit(o)
						} else {
							s.focus = o
						}
					})
				}
			},
			// Long-press takes the word and offers Copy. Without it this text
			// is selectable on a phone and copyable only with a Command key
			// the device does not have.
			OnLongPress: func() {
				if s.ref.box != nil {
					lo, hi := s.ref.box.wordAt(s.focus)
					s.SetState(func() { s.anchor, s.focus, s.handles = lo, hi, true })
				}
				s.showMenu(ctx)
			},
			OnPressEnd: func() {
				s.dragHandle = -1
				if lo, hi := s.sel(); lo != hi {
					s.showMenu(ctx)
				}
			},
			OnKey: func(k shell.Key) {
				if k.Kind != shell.KeyPress || !k.Mods.Command() {
					return
				}
				switch k.Code {
				case shell.KeyC:
					s.copy()
				case shell.KeyA:
					s.SetState(s.selectAll)
				}
			},
			// Double-tap selects the word under the pointer (OnPress set
			// s.focus to the tapped offset on the way in).
			OnDoubleTap: func() {
				if s.ref.box != nil {
					s.SetState(func() { s.selectUnitAt(s.focus, selWords) })
				}
			},
		},
		Child: selText{
			text: t.S, font: t.Font, size: t.size(), color: t.Color,
			wrap: t.Wrap, selColor: t.selectionColor(), lo: lo, hi: hi,
			handles: s.handles, ref: s.ref,
		},
	}
}

// showMenu offers Copy for the current selection. Read-only text, so there is
// no Cut or Paste.
func (s *selectableState) showMenu(ctx Ctx) {
	s.closeMenu()
	acts := editActionsFor(ctx, selectionOps{
		HasSelection: func() bool { lo, hi := s.sel(); return lo != hi },
		AllSelected:  s.allSelected,
		Copy:         s.copy,
		SelectAll:    func() { s.SetState(s.selectAll) },
	})
	if len(acts) > 0 {
		s.dismissMenu = ShowEditMenu(ctx, s.pressGlobal, acts)
	}
}

// closeMenu dismisses an open edit menu, if any.
func (s *selectableState) closeMenu() {
	if s.dismissMenu != nil {
		s.dismissMenu()
		s.dismissMenu = nil
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
	handles         bool
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
	sb.handles = w.handles
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
	layoutbox.Base
	Painter  *paint.Painter
	text     string
	font     string
	size     float32
	color    paint.Color
	selColor paint.Color
	wrap     bool
	lo, hi   int
	handles  bool // a finger made this selection, so it gets grips

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
	li := max(int(p.Y/b.lineH), 0)
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
	for li, v := range slices.Backward(b.lines) {
		start := b.lineStart[li]
		if off < start {
			continue
		}
		runes := []rune(v)
		col := min(off-start, len(runes))
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

// lineAt returns the span of the wrapped line containing off — what a triple
// click takes.
func (b *selectableBox) lineAt(off int) (int, int) {
	for li, v := range slices.Backward(b.lines) {
		start := b.lineStart[li]
		if off < start {
			continue
		}
		return start, start + len([]rune(v))
	}
	return off, off
}

// linearLen is the offset one past the last rune, over the wrapped-line model
// the selection offsets are expressed in.
func (b *selectableBox) linearLen() int {
	if len(b.lines) == 0 {
		return 0
	}
	last := len(b.lines) - 1
	return b.lineStart[last] + len([]rune(b.lines[last]))
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
	b.paintHandles(c, at)
}

// caretPt is the local position of the selection edge at off: the bottom of
// the line it falls on, which is where a grip hangs from.
func (b *selectableBox) caretPt(off int) geom.Pt {
	if len(b.lines) == 0 {
		return geom.Pt{}
	}
	li := len(b.lines) - 1
	for i, ln := range b.lines {
		if off <= b.lineStart[i]+len([]rune(ln)) {
			li = i
			break
		}
	}
	runes := []rune(b.lines[li])
	col := min(max(off-b.lineStart[li], 0), len(runes))
	return geom.Pt{
		X: b.Painter.MeasureWidthIn(b.font, string(runes[:col]), b.size),
		Y: float32(li)*b.lineH + b.baseline + b.descent,
	}
}

// handleCentres returns the two grip positions in local coordinates, and
// whether there is a finger-made selection to show them for.
func (b *selectableBox) handleCentres() (lo, hi geom.Pt, ok bool) {
	if !b.handles || b.hi <= b.lo {
		return geom.Pt{}, geom.Pt{}, false
	}
	return b.caretPt(b.lo), b.caretPt(b.hi), true
}

// handleAt reports which grip the local point grabs: -1 none, 0 the start of
// the selection, 1 the end.
func (b *selectableBox) handleAt(p geom.Pt) int {
	lo, hi, ok := b.handleCentres()
	if !ok {
		return -1
	}
	dl, dh := sqDist(p, lo), sqDist(p, hi)
	grab := float32(selHandleGrab * selHandleGrab)
	switch {
	case dl <= grab && dl <= dh:
		return 0
	case dh <= grab:
		return 1
	}
	return -1
}

// paintHandles draws the grips under a finger-made selection, the same dot and
// stem the field and the selection area use.
func (b *selectableBox) paintHandles(c paint.Canvas, at geom.Pt) {
	lo, hi, ok := b.handleCentres()
	if !ok {
		return
	}
	col := b.selColor
	col.A = 1
	for _, p := range []geom.Pt{lo, hi} {
		g := at.Add(p)
		c.Line(geom.Pt{X: g.X, Y: g.Y - selHandleRadius*2}, geom.Pt{X: g.X, Y: g.Y}, 1.5, col)
		c.FillRRect(geom.Rect{
			Min: geom.Pt{X: g.X - selHandleRadius, Y: g.Y - selHandleRadius},
			Max: geom.Pt{X: g.X + selHandleRadius, Y: g.Y + selHandleRadius},
		}, selHandleRadius, col)
	}
}

// Semantics reports the text as plain text, the same as Text does. Without
// this a SelectableText is invisible to assistive technology and to apptest:
// the Interactive around it declares no role (it has no OnTap and takes no
// text), so nothing in the subtree produced a node, and a screen reader
// skipped the one label on the screen that was worth copying.
func (b *selectableBox) Semantics() layout.SemInfo {
	return layout.SemInfo{Role: layout.RoleText, Label: b.text}
}

func (b *selectableBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X >= 0 && p.Y >= 0 && p.X < b.sz.W && p.Y < b.sz.H {
		*hits = append(*hits, layout.Hit{Box: b, Pos: p})
	}
}
