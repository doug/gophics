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

// SelectionArea makes every Text in its subtree selectable as one continuous
// range: drag across titles, paragraphs, and list items to select, then copy
// with Cmd/Ctrl+C. It is framework-level — it paints its own highlight and
// copies to the platform clipboard — so it behaves identically on web,
// terminal, desktop, and mobile (the Flutter SelectionArea model), with no
// dependency on native/DOM text.
//
// Text widgets opt in automatically by reading the area's registry from context;
// no change to the child tree is needed.
//
// The area selects within its own box, so wrap the padding rather than sitting
// inside it: a drag that begins in a card's margin extends to the nearest line
// in a browser, and an area drawn tightly around the glyphs would never see
// that press.
type SelectionArea struct {
	Child Widget
	// SelectionColor is the highlight fill; zero alpha → a translucent blue.
	SelectionColor paint.Color
	// OnSelect reports the selected text whenever it changes, and "" when the
	// selection is cleared. For a toolbar that enables Copy only when there is
	// something to copy, a word count, or a quote button beside the text.
	OnSelect func(text string)
}

func (a SelectionArea) CreateState() State { return &selectionAreaState{} }

type selectionAreaState struct {
	StateBase[SelectionArea]
	ctx Ctx
	reg *selectionRegistry

	// Where the press landed, for anchoring the edit menu (OnLongPress and
	// OnPressEnd report no position), and the menu's dismiss.
	pressGlobal geom.Pt
	dismissMenu func()

	// Clicks are counted here rather than taken from the tap dispatcher: a
	// double-click *drag* selects by words from the second press, and a tap
	// is only reported on release — too late to choose the unit before the
	// drag has already begun.
	clicks       int
	lastPress    time.Time
	lastPressPos geom.Pt
	dragHandle   int // -1 none, else the grip being dragged
	reported     string
}

func (s *selectionAreaState) Init(ctx Ctx) {
	s.ctx = ctx
	s.reg = &selectionRegistry{}
	s.dragHandle = -1
}

// Dispose takes the edit menu down with the area: the menu is an overlay
// entry beside the tree, not a descendant, so nothing else would.
func (s *selectionAreaState) Dispose() { s.closeMenu() }

func (s *selectionAreaState) Build(ctx Ctx) Widget {
	col := s.W().SelectionColor
	if col.A == 0 {
		col = paint.Color{R: 0.36, G: 0.62, B: 0.98, A: 0.35}
	}
	r := s.reg
	r.selColor = col
	// The nearest enclosing Scroll's scroll-into-view service, if any.
	r.reveal, _ = ctx.Of[*scrollReveal]()
	return Provide[*selectionRegistry]{Value: r, Child: Interactive{
		Gestures: Gestures{
			// Text the pointer can act on says so before it is touched.
			Cursor: shell.CursorText,
			// Modality-aware drag ownership (Flutter parity):
			//   - mouse: a drag that begins on text selects (any direction),
			//     beating a deeper scroll; a drag on empty space scrolls.
			//   - touch: a drag scrolls, UNLESS a long-press already started a
			//     selection — then the drag extends it.
			DragPriority: func(touch bool) bool {
				if touch {
					return r.selecting
				}
				return r.pressedText
			},
			OnPress: func(p geom.Pt) {
				s.closeMenu()
				s.pressGlobal = ctx.Input().Pointer()
				// A press on a grip adjusts the selection rather than
				// replacing it — reaching for the grip is the only way to
				// fix a selection on a phone.
				if h := r.handleAt(p.Add(r.origin)); h >= 0 {
					s.dragHandle = h
					s.SetState(func() { r.pressedText, r.selecting = true, true })
					return
				}
				s.dragHandle = -1
				pt, onText := r.locate(p)
				if !onText {
					s.clicks = 0
					s.SetState(func() { r.has, r.pressedText, r.selecting = false, false, false })
					s.report()
					return
				}
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
					r.handles = false
					// Third click takes the paragraph, and a drag from here
					// runs by paragraphs. Both desktop and the phones do this.
					s.SetState(func() {
						r.selectUnitAt(pt, selParas)
						r.pressedText, r.selecting = true, true
					})
				case s.clicks == 2:
					r.handles = false
					s.SetState(func() {
						r.selectUnitAt(pt, selWords)
						r.pressedText, r.selecting = true, true
					})
				case r.has && ctx.Input().Mods()&shell.ModShift != 0:
					// Shift-click keeps the anchor where it was and moves the
					// far end to the click, as every desktop text view does.
					s.SetState(func() {
						r.focus, r.dragUnit, r.pressedText, r.selecting = pt, selChars, true, true
					})
				default:
					// A plain press records the caret and clears any prior
					// selection; the drag (mouse) or long-press (touch) turns
					// it into a visible selection.
					r.handles = false
					s.SetState(func() {
						r.anchor, r.focus, r.has, r.pressedText, r.selecting = pt, pt, true, true, false
						r.dragUnit = selChars
					})
				}
				s.report()
			},
			// Right-click selects the word under the pointer when nothing is
			// selected yet, then offers the menu — Cocoa's behaviour and
			// Windows's both.
			OnSecondaryTap: func(p geom.Pt) {
				s.closeMenu()
				s.pressGlobal = ctx.Input().Pointer()
				if pt, ok := r.locate(p); ok && r.selectedText() == "" {
					s.SetState(func() { r.selectUnitAt(pt, selWords) })
					s.report()
				}
				s.showMenu(ctx)
			},
			// Long-press starts a selection at the word under the pointer (the
			// touch entry point; on desktop it's a harmless bonus). A subsequent
			// drag then extends it via DragPriority.
			OnLongPress: func() {
				if !r.pressedText {
					return
				}
				s.SetState(func() { r.selectWord(); r.has, r.selecting, r.handles = true, true, true })
				s.report()
				// The word is selected; now offer something to do with it. On a
				// phone this is the only route to Copy — there is no Cmd+C.
				s.showMenu(ctx)
			},
			OnDrag: func(pos, _ geom.Pt) {
				// The selection is moving under the menu, so take it away and
				// bring it back when the finger lifts. Both platforms do this;
				// a menu anchored to a selection that is still growing is worse
				// than no menu.
				s.closeMenu()
				if s.dragHandle >= 0 {
					if pt, ok := r.locate(pos); ok {
						s.SetState(func() {
							s.dragHandle = r.moveHandleTo(s.dragHandle, pt)
							r.revealPending = true
						})
						s.report()
					}
					return
				}
				if pt, ok := r.locate(pos); ok {
					s.SetState(func() {
						if r.dragUnit != selChars {
							r.extendByUnit(pt)
						} else {
							r.focus, r.has = pt, true
						}
						r.revealPending = true
					})
					s.report()
				}
			},
			OnPressEnd: func() {
				s.dragHandle = -1
				s.report()
				if r.selecting && r.selectedText() != "" {
					s.showMenu(ctx)
				}
			},
			OnKey: func(k shell.Key) {
				if k.Kind != shell.KeyPress || !k.Mods.Command() {
					return
				}
				switch k.Code {
				case shell.KeyC:
					s.copySelection()
				case shell.KeyA:
					s.SetState(func() { r.selectAll() })
					s.report()
				}
			},
		},
		Child: selAnchor{reg: r, child: s.W().Child},
	}}
}

// report tells the app the selection changed, once per distinct value. Called
// at the end of each gesture that moves an endpoint rather than from Build,
// which would fire it again on every unrelated rebuild.
func (s *selectionAreaState) report() {
	f := s.W().OnSelect
	if f == nil {
		return
	}
	if txt := s.reg.selectedText(); txt != s.reported {
		s.reported = txt
		f(txt)
	}
}

// copySelection puts the selected text on the clipboard.
func (s *selectionAreaState) copySelection() {
	txt := s.reg.selectedText()
	if txt == "" {
		return
	}
	if cb := s.ctx.Clipboard(); cb != nil {
		_ = cb.ClipboardWrite(txt)
	}
}

// showMenu raises the edit menu for a read-only selection: Copy, and Select All
// where the area has more than the selection in it. There is no Cut or Paste —
// this text is not editable, and offering either would be a lie.
func (s *selectionAreaState) showMenu(ctx Ctx) {
	s.closeMenu()
	acts := editActionsFor(ctx, selectionOps{
		HasSelection: func() bool { return s.reg.selectedText() != "" },
		AllSelected:  s.reg.allSelected,
		Copy:         s.copySelection,
		SelectAll:    func() { s.SetState(func() { s.reg.selectAll() }) },
	})
	if len(acts) > 0 {
		s.dismissMenu = ShowEditMenu(ctx, s.pressGlobal, acts)
	}
}

// closeMenu dismisses an open edit menu, if any.
func (s *selectionAreaState) closeMenu() {
	if s.dismissMenu != nil {
		s.dismissMenu()
		s.dismissMenu = nil
	}
}

// selPoint is a position in the selection model: a fragment index (in paint
// order) and a linear rune offset within it.
type selPoint struct {
	frag int
	off  int
}

// selFrag is one registered text fragment for the current frame.
type selFrag struct {
	origin    geom.Pt // absolute
	rect      geom.Rect
	lines     []string
	lineStart []int // linear rune offset at each line's start
	linearLen int
	font      string
	size      float32
	lineH     float32
	ascent    float32
	descent   float32
	painter   *paint.Painter
}

// selectionRegistry coordinates a single selection across all the text
// fragments painted inside a SelectionArea. It implements layout.SelectionSink.
type selectionRegistry struct {
	origin        geom.Pt    // the area's absolute origin (pointer coords rebase to it)
	frags         []*selFrag // rebuilt every paint, in paint (reading) order
	anchor, focus selPoint
	has           bool
	pressedText   bool // last press landed on text → drag should select, not scroll
	selecting     bool // a touch long-press has started a selection (drag extends it)
	selColor      paint.Color

	// dragUnit is what the drag in progress selects by; unitA/unitZ are the
	// unit the gesture started on, kept whole when the drag doubles back over
	// it.
	dragUnit     selUnit
	unitA, unitZ selPoint

	// handles is set when a finger made the selection, which is when the grips
	// are drawn. A mouse user adjusts with shift-click and would find two dots
	// under their text puzzling.
	handles bool

	// reveal is the enclosing Scroll's scroll-into-view service, if any, and
	// revealPending asks for the moving end of the selection to be brought
	// into view at the next paint. Dragging a selection past the edge of a
	// scrolling page scrolls it on every platform; without this the selection
	// stops at whatever happens to be on screen.
	reveal        *scrollReveal
	revealPending bool
}

// doReveal scrolls the moving end of the selection into view. Called from the
// anchor box's paint, where the fragment origins are fresh and absolute.
func (r *selectionRegistry) doReveal() {
	if !r.revealPending {
		return
	}
	r.revealPending = false
	if r.reveal == nil || !r.reveal.have {
		return
	}
	p, ok := r.caretPt(r.focus)
	if !ok {
		return
	}
	if r.reveal.horizontal() {
		cx := p.X - r.reveal.origin.X
		r.reveal.reveal(cx, cx)
		return
	}
	// caretPt is the bottom of the line; reveal the whole line rather than
	// its baseline, so the text being selected is what comes into view.
	top := p.Y
	if fr := r.frags[r.focus.frag]; fr.lineH > 0 {
		top -= fr.lineH
	}
	r.reveal.reveal(top-r.reveal.origin.Y, p.Y-r.reveal.origin.Y)
}

// caretPt is the absolute position of the selection edge at pt: the bottom of
// the line at that offset, which is where a grip hangs from.
func (r *selectionRegistry) caretPt(pt selPoint) (geom.Pt, bool) {
	if pt.frag < 0 || pt.frag >= len(r.frags) {
		return geom.Pt{}, false
	}
	fr := r.frags[pt.frag]
	if len(fr.lines) == 0 {
		return geom.Pt{}, false
	}
	li := len(fr.lines) - 1
	for i, ln := range fr.lines {
		if pt.off <= fr.lineStart[i]+len([]rune(ln)) {
			li = i
			break
		}
	}
	runes := []rune(fr.lines[li])
	col := min(max(pt.off-fr.lineStart[li], 0), len(runes))
	x := fr.origin.X + fr.painter.MeasureWidthIn(fr.font, string(runes[:col]), fr.size)
	y := fr.origin.Y + float32(li)*fr.lineH + fr.ascent + fr.descent
	return geom.Pt{X: x, Y: y}, true
}

// handleCentres returns the absolute positions of the two grips, and whether
// there is a finger-made selection to show them for.
func (r *selectionRegistry) handleCentres() (lo, hi geom.Pt, ok bool) {
	if !r.handles || !r.has {
		return geom.Pt{}, geom.Pt{}, false
	}
	a, b := r.ordered()
	if a == b {
		return geom.Pt{}, geom.Pt{}, false
	}
	pa, oka := r.caretPt(a)
	pb, okb := r.caretPt(b)
	if !oka || !okb {
		return geom.Pt{}, geom.Pt{}, false
	}
	return pa, pb, true
}

// handleAt reports which grip the absolute point grabs: -1 none, 0 the start
// of the selection, 1 the end.
func (r *selectionRegistry) handleAt(abs geom.Pt) int {
	lo, hi, ok := r.handleCentres()
	if !ok {
		return -1
	}
	dl, dh := sqDist(abs, lo), sqDist(abs, hi)
	grab := float32(selHandleGrab * selHandleGrab)
	switch {
	case dl <= grab && dl <= dh:
		return 0
	case dh <= grab:
		return 1
	}
	return -1
}

// moveHandleTo drags grip h to pt, keeping the other end where it is, and
// reports which grip is being dragged after the move — pulling one end past
// the other swaps them rather than collapsing the selection.
func (r *selectionRegistry) moveHandleTo(h int, pt selPoint) int {
	a, b := r.ordered()
	fixed := b
	if h == 1 {
		fixed = a
	}
	if pt == fixed {
		return h // a zero-width selection would drop the grips mid-drag
	}
	r.anchor, r.focus = fixed, pt
	if beforePt(pt, fixed) {
		return 0
	}
	return 1
}

// paintHandles draws the grips at the ends of a finger-made selection: a
// filled dot with a short stem up to the text, the shape both phones use,
// which reads as "grab me" at a glance.
func (r *selectionRegistry) paintHandles(c paint.Canvas) {
	lo, hi, ok := r.handleCentres()
	if !ok {
		return
	}
	col := r.selColor
	col.A = 1
	for _, g := range []geom.Pt{lo, hi} {
		c.Line(geom.Pt{X: g.X, Y: g.Y - selHandleRadius*2}, geom.Pt{X: g.X, Y: g.Y}, 1.5, col)
		c.FillRRect(geom.Rect{
			Min: geom.Pt{X: g.X - selHandleRadius, Y: g.Y - selHandleRadius},
			Max: geom.Pt{X: g.X + selHandleRadius, Y: g.Y + selHandleRadius},
		}, selHandleRadius, col)
	}
}

func sqDist(a, b geom.Pt) float32 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx + dy*dy
}

// selUnit is what a drag selects by: single characters, or — after a double
// or triple click — whole words or whole paragraphs, so a moving pointer
// never leaves half a word behind.
type selUnit uint8

const (
	selChars selUnit = iota
	selWords
	selParas
)

// unitRange is the span of the unit containing pt. A fragment is one Text or
// Rich widget, which is what a reader means by a paragraph and what a
// browser's triple click takes.
func (r *selectionRegistry) unitRange(pt selPoint, u selUnit) (selPoint, selPoint) {
	if pt.frag < 0 || pt.frag >= len(r.frags) {
		return pt, pt
	}
	fr := r.frags[pt.frag]
	switch u {
	case selWords:
		lo, hi := fr.wordAt(pt.off)
		return selPoint{pt.frag, lo}, selPoint{pt.frag, hi}
	case selParas:
		return selPoint{pt.frag, 0}, selPoint{pt.frag, fr.linearLen}
	}
	return pt, pt
}

// selectUnitAt selects the whole unit under pt and remembers it, so a drag
// that doubles back still covers the unit it started on.
func (r *selectionRegistry) selectUnitAt(pt selPoint, u selUnit) {
	a, z := r.unitRange(pt, u)
	r.anchor, r.focus = a, z
	r.unitA, r.unitZ = a, z
	r.dragUnit = u
	r.has = true
}

// selectAll takes every fragment in the area.
func (r *selectionRegistry) selectAll() {
	if len(r.frags) == 0 {
		return
	}
	last := len(r.frags) - 1
	r.anchor = selPoint{0, 0}
	r.focus = selPoint{last, r.frags[last].linearLen}
	r.dragUnit = selChars
	r.has = true
}

// allSelected reports whether the selection already covers every fragment, so
// the menu can leave Select All out rather than offer a no-op.
func (r *selectionRegistry) allSelected() bool {
	if !r.has || len(r.frags) == 0 {
		return false
	}
	a, b := r.ordered()
	last := len(r.frags) - 1
	return a.frag == 0 && a.off == 0 && b.frag == last && b.off >= r.frags[last].linearLen
}

// extendByUnit grows the selection to cover the unit under pt as well as the
// one the gesture started on, whichever side of it pt is.
func (r *selectionRegistry) extendByUnit(pt selPoint) {
	a, z := r.unitRange(pt, r.dragUnit)
	if beforePt(a, r.unitA) {
		r.anchor, r.focus = r.unitZ, a
	} else {
		r.anchor, r.focus = r.unitA, z
	}
	r.has = true
}

// beforePt reports whether a comes before b in reading order.
func beforePt(a, b selPoint) bool {
	return a.frag < b.frag || (a.frag == b.frag && a.off < b.off)
}

// selectWord expands the current caret (anchor) to the word around it — the
// touch long-press entry point.
func (r *selectionRegistry) selectWord() { r.selectUnitAt(r.anchor, selWords) }

// beginFrame resets the per-frame fragment list. Called by the anchor box at
// the start of each paint, before descendants register.
func (r *selectionRegistry) beginFrame(origin geom.Pt) {
	r.origin = origin
	r.frags = r.frags[:0]
}

// RegisterText implements layout.SelectionSink.
func (r *selectionRegistry) RegisterText(f layout.TextFragment) (int, int, paint.Color) {
	fr := &selFrag{
		origin: f.Origin, lines: f.Lines, font: f.Font, size: f.Size,
		lineH: f.LineH, ascent: f.Ascent, descent: f.Descent, painter: f.Painter,
	}
	off := 0
	var maxW float32
	for _, ln := range f.Lines {
		fr.lineStart = append(fr.lineStart, off)
		off += len([]rune(ln)) + 1
		if w := f.Painter.MeasureWidthIn(f.Font, ln, f.Size); w > maxW {
			maxW = w
		}
	}
	if off > 0 {
		off-- // drop the trailing virtual newline
	}
	fr.linearLen = off
	h := f.Ascent + f.Descent + float32(len(f.Lines)-1)*f.LineH
	fr.rect = geom.Rect{Min: f.Origin, Max: geom.Pt{X: f.Origin.X + maxW, Y: f.Origin.Y + h}}

	idx := len(r.frags)
	r.frags = append(r.frags, fr)
	lo, hi := r.rangeFor(idx)
	return lo, hi, r.selColor
}

// ordered returns the selection endpoints in reading order.
func (r *selectionRegistry) ordered() (selPoint, selPoint) {
	a, b := r.anchor, r.focus
	if a.frag > b.frag || (a.frag == b.frag && a.off > b.off) {
		return b, a
	}
	return a, b
}

// rangeFor returns the [lo, hi) linear rune range to highlight in fragment idx.
func (r *selectionRegistry) rangeFor(idx int) (int, int) {
	if !r.has {
		return 0, 0
	}
	a, b := r.ordered()
	if idx < a.frag || idx > b.frag || idx >= len(r.frags) {
		return 0, 0
	}
	lo, hi := 0, r.frags[idx].linearLen
	if idx == a.frag {
		lo = a.off
	}
	if idx == b.frag {
		hi = b.off
	}
	if lo >= hi {
		return 0, 0
	}
	return lo, hi
}

// locate maps an area-local point to a selection position, choosing the
// fragment under the point or, failing that, the vertically nearest one (so a
// drag past the text still extends to the closest run).
func (r *selectionRegistry) locate(local geom.Pt) (selPoint, bool) {
	abs := local.Add(r.origin)
	best := -1
	var bestDist float32 = 1e18
	for i, fr := range r.frags {
		if fr.rect.Contains(abs) {
			best = i
			break
		}
		d := vDist(fr.rect, abs)
		if d < bestDist {
			bestDist, best = d, i
		}
	}
	if best < 0 {
		return selPoint{}, false
	}
	fr := r.frags[best]
	return selPoint{best, fr.offsetAt(geom.Pt{X: abs.X - fr.origin.X, Y: abs.Y - fr.origin.Y})}, true
}

// selectedText returns the selected runes across fragments, lines and
// fragments joined with newlines.
func (r *selectionRegistry) selectedText() string {
	if !r.has {
		return ""
	}
	a, b := r.ordered()
	var parts []string
	for i := a.frag; i <= b.frag && i < len(r.frags); i++ {
		fr := r.frags[i]
		lo, hi := 0, fr.linearLen
		if i == a.frag {
			lo = a.off
		}
		if i == b.frag {
			hi = b.off
		}
		if t := fr.textRange(lo, hi); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

// offsetAt maps a fragment-local point to a linear rune offset (clamped).
func (fr *selFrag) offsetAt(p geom.Pt) int {
	if len(fr.lines) == 0 {
		return 0
	}
	li := int(p.Y / fr.lineH)
	li = min(max(li, 0), len(fr.lines)-1)
	runes := []rune(fr.lines[li])
	col := len(runes)
	for i := 1; i <= len(runes); i++ {
		w := fr.painter.MeasureWidthIn(fr.font, string(runes[:i]), fr.size)
		prev := fr.painter.MeasureWidthIn(fr.font, string(runes[:i-1]), fr.size)
		if p.X < (prev+w)/2 { // past the glyph midpoint selects the next
			col = i - 1
			break
		}
	}
	return fr.lineStart[li] + col
}

// wordAt returns the linear range of the whitespace-delimited word containing
// the offset (for long-press word selection).
func (fr *selFrag) wordAt(off int) (int, int) {
	for li, v := range slices.Backward(fr.lines) {
		start := fr.lineStart[li]
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

// textRange returns the runes in [lo, hi) across the fragment's lines, joined
// with newlines.
func (fr *selFrag) textRange(lo, hi int) string {
	if lo >= hi {
		return ""
	}
	var out []string
	for li, ln := range fr.lines {
		runes := []rune(ln)
		start := fr.lineStart[li]
		a := max(lo-start, 0)
		z := min(hi-start, len(runes))
		if a < z {
			out = append(out, string(runes[a:z]))
		}
	}
	return strings.Join(out, "\n")
}

// vDist is the vertical distance from a point to a rect (0 if within its band).
func vDist(r geom.Rect, p geom.Pt) float32 {
	switch {
	case p.Y < r.Min.Y:
		return r.Min.Y - p.Y
	case p.Y > r.Max.Y:
		return p.Y - r.Max.Y
	default:
		return 0
	}
}

// selAnchor is a single-child render widget whose box captures the area's
// absolute origin and resets the registry at the start of each paint.
type selAnchor struct {
	reg   *selectionRegistry
	child Widget
}

func (w selAnchor) createBox(Ctx) layout.Box            { return &selAnchorBox{reg: w.reg} }
func (w selAnchor) updateBox(_ Ctx, b layout.Box)       { b.(*selAnchorBox).reg = w.reg }
func (w selAnchor) childWidgets() []Widget              { return []Widget{w.child} }
func (w selAnchor) soleChild() Widget                   { return w.child }
func (w selAnchor) attach(b layout.Box, k []layout.Box) { b.(*selAnchorBox).child = first(k) }

type selAnchorBox struct {
	layoutbox.Base
	reg   *selectionRegistry
	child layout.Box
}

func (b *selAnchorBox) Layout(cs layout.Constraints) geom.Size {
	var sz geom.Size
	if b.child != nil {
		sz = b.child.Layout(cs)
	} else {
		sz = cs.Constrain(geom.Size{})
	}
	return b.Done(cs, sz)
}

func (b *selAnchorBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.reg != nil {
		b.reg.beginFrame(at)
	}
	if b.child != nil {
		b.child.Paint(c, at)
	}
	// After the child, so the grips sit over the text they belong to.
	if b.reg != nil {
		b.reg.paintHandles(c)
		b.reg.doReveal()
	}
}

func (b *selAnchorBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.child != nil {
		b.child.AddHits(p, hits)
	}
}

func (b *selAnchorBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.child != nil {
		visit(b.child, geom.Pt{})
	}
}
