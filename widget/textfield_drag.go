package widget

import (
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
)

// What happens to a text field while a drag is in flight: the selection
// keeps scrolling when the pointer sits past the box's edge, a mouse can
// carry the selected text to a new place, and a finger dragging the caret
// gets a magnifier over the character it is covering.

// autoScrollEvery is how often a pointer held outside the box advances the
// selection one more step: twenty steps a second reads as a steady scroll.
const autoScrollEvery = 0.05

// autoScroller keeps a drag going while the pointer is outside the field.
// The pointer cannot reach text that is scrolled out of view, so every
// native field scrolls toward it for as long as it is held there; without
// this a selection stops dead at the edge. It is a frame ticker rather than
// a wall-clock timer so it runs in the app's own time, headless included.
type autoScroller struct {
	s      *textFieldState
	pos    geom.Pt // the last drag position, field-local
	active bool
	acc    float64
	added  bool
}

func (a *autoScroller) Tick(dt float64) bool {
	if !a.active {
		return false
	}
	a.acc += dt
	for a.acc >= autoScrollEvery {
		a.acc -= autoScrollEvery
		a.s.dragStep(a.s.ctx, a.pos, true)
	}
	return true
}

// dragTo applies a drag to p, and when p lies outside the box keeps applying
// it on a timer until the pointer comes back in or lifts.
func (s *textFieldState) dragTo(ctx Ctx, p geom.Pt) {
	s.dragStep(ctx, p, false)
	a := s.auto
	if a == nil {
		a = &autoScroller{s: s}
		s.auto = a
	}
	a.pos = p
	if !s.outside(p) {
		a.active = false
		return
	}
	if !a.active {
		a.active, a.acc = true, 0
		if !a.added {
			ctx.AddTicker(a)
			a.added = true
		}
	}
}

// outside reports whether a field-local point is past the edge the field
// can scroll toward: left or right of a single-line box, above or below a
// wrapped one.
func (s *textFieldState) outside(p geom.Pt) bool {
	if s.W().Multiline {
		return p.Y < 0 || p.Y > s.lastHeight
	}
	return p.X < 0 || p.X > s.lastWidth
}

func (s *textFieldState) stopAuto() {
	if s.auto != nil {
		s.auto.active = false
	}
}

// dragStep moves whatever the drag is moving to p: a selection grip, the
// drop point of text being carried, or the selection's free end. tick marks
// a step the auto-scroller is taking with the pointer held still, which
// must make progress even when p maps to the same index as last time —
// a pointer a pixel past the edge would otherwise scroll a pixel and stop.
func (s *textFieldState) dragStep(ctx Ctx, p geom.Pt, tick bool) {
	f := s.W()
	if tick && f.Multiline && (p.Y < 0 || p.Y > s.lastHeight) && s.dragHandle < 0 && !s.dragText {
		// Past the top or bottom of a wrapped field: one line further per
		// step, in the column the pointer is in.
		dir := 1
		if p.Y < 0 {
			dir = -1
		}
		s.goalX, s.goalOK = p.X, true
		s.moveVertical(ctx, dir, true)
		if s.dragUnit != dragChars {
			s.dragByUnit(s.ed.Caret())
			s.SetState(nil)
		}
		s.showLoupe(ctx)
		return
	}
	idx := s.indexAtPt(ctx, p)
	if tick && !f.Multiline {
		cur := s.ed.Caret()
		if s.dragText {
			cur = s.dropIdx
		}
		if idx == cur {
			if p.X < 0 {
				idx--
			} else {
				idx++
			}
			idx = max(0, min(idx, len([]rune(s.ed.Text()))))
		}
	}
	switch {
	case s.dragHandle >= 0:
		if s.moveHandleTo(idx) {
			s.SetState(nil)
		}
	case s.dragText:
		if idx != s.dropIdx {
			s.dropIdx = idx
			s.SetState(nil)
		}
		return // no magnifier: a mouse carries text, a finger does not
	case s.dragUnit != dragChars:
		s.dragByUnit(idx)
		s.SetState(nil)
	default:
		s.ed.MoveTo(idx, true)
		s.SetState(nil)
	}
	s.showLoupe(ctx)
}

// endDrag is the pointer lifting, whatever the drag was doing.
func (s *textFieldState) endDrag(ctx Ctx) {
	s.stopAuto()
	s.hideLoupe()
	if s.dragText {
		s.dropText(ctx)
	}
}

// beginTextDrag decides whether a press is picking up the selected text. A
// mouse press inside the selection does not collapse it, because it may be
// the start of dragging that text somewhere else — Cocoa and Windows both
// wait for the release to find out, and place the caret then if no drag
// came. A finger's press inside its selection was handled before this.
func (s *textFieldState) beginTextDrag(ctx Ctx, p geom.Pt) bool {
	in := ctx.Input()
	if in.PointerIsTouch() || !s.ed.HasSelection() || s.clicks != 1 || in.Mods()&shell.ModShift != 0 || !s.W().editable() {
		return false
	}
	idx := s.indexAtPt(ctx, p)
	a, b := s.ed.Selection()
	if idx <= a || idx >= b {
		return false
	}
	s.dragText, s.dropIdx = true, -1
	return true
}

// dropText finishes a text drag: moves the selection to the drop point, or
// copies it there with Alt/Option held, as one undo step. A press that
// never moved was a click inside the selection, and places the caret.
func (s *textFieldState) dropText(ctx Ctx) {
	s.dragText = false
	drop := s.dropIdx
	s.dropIdx = -1
	if drop < 0 {
		s.ed.MoveTo(s.indexAtPt(ctx, s.pressLocal), false)
		s.SetState(nil)
		return
	}
	a, b := s.ed.Selection()
	if drop >= a && drop <= b {
		s.SetState(nil) // dropped on itself
		return
	}
	runes := []rune(s.ed.Text())
	moved := runes[a:b]
	copy := ctx.Input().Mods()&shell.ModAlt != 0
	// Rewrite the one span that covers both the source and the drop point,
	// so the whole rearrangement is a single replacement and a single undo.
	lo, hi := min(a, drop), max(b, drop)
	seg := runes[lo:hi]
	at := drop - lo
	var out []rune
	if copy {
		out = append(append(append(out, seg[:at]...), moved...), seg[at:]...)
	} else {
		rest := append(append([]rune(nil), seg[:a-lo]...), seg[b-lo:]...)
		if drop > b {
			at -= b - a
		}
		out = append(append(append(out, rest[:at]...), moved...), rest[at:]...)
	}
	s.ed.MoveTo(lo, false)
	s.ed.MoveTo(hi, true)
	s.ed.Replace(string(out))
	s.ed.MoveTo(lo+at, false)
	s.ed.MoveTo(lo+at+len(moved), true)
	s.change(ctx)
}

// The magnifier: a finger placing the caret covers the very character it is
// aiming at, so both mobile platforms float a magnified copy of the text
// above the finger while it drags — iOS's loupe, Android's magnifier.
const (
	loupeW     = 128
	loupeH     = 44
	loupeScale = 1.5
	loupeGap   = 16
)

// showLoupe raises the magnifier, or moves it, for a finger dragging the
// caret or a selection grip. A mouse never sees it.
func (s *textFieldState) showLoupe(ctx Ctx) {
	if !ctx.Input().PointerIsTouch() {
		return
	}
	ov, ok := ctx.Of[Overlay]()
	if !ok {
		return
	}
	at := ctx.Input().Pointer()
	x := at.X - loupeW/2
	if x < 8 {
		x = 8
	}
	y := at.Y - loupeH - loupeGap
	if min := ctx.SafeInsets().Top + 8; y < min {
		y = min
	}
	w := Padding{
		Insets: geom.Insets{Left: x, Top: y},
		Child:  Align{X: 0, Y: 0, Child: s.loupeWidget(ctx)},
	}
	if s.loupeOn {
		s.loupeTok.Update(w)
		return
	}
	s.loupeTok = ov.Show(w)
	s.loupeOn = true
}

func (s *textFieldState) hideLoupe() {
	if s.loupeOn {
		s.loupeOn = false
		s.loupeTok.Dismiss()
	}
}

// loupeWidget is the glass: the caret's visual line at loupeScale, centred
// on the caret (which, during a grip drag, is the end being moved), with
// the selection under it. It snapshots the text now because the overlay
// paints on its own schedule.
func (s *textFieldState) loupeWidget(ctx Ctx) Widget {
	f := s.W()
	txC, caretC, selC, _ := f.resolvedColors()
	bg, border := paint.RGB(0.99, 0.99, 1), paint.RGB(0.72, 0.73, 0.76)
	if ctx.DarkMode() {
		bg, border = paint.RGB(0.13, 0.14, 0.16), paint.RGB(0.36, 0.37, 0.40)
	}
	str := s.shown()
	runes := []rune(str)
	start, end, ok := s.visualLine(ctx)
	if !ok {
		start, end = 0, len(runes)
	}
	lineStr := strings.TrimRight(string(runes[start:end]), "\n")
	caret := min(max(s.ed.Caret(), start), end) - start
	selA, selB := s.ed.Selection()
	selA, selB = max(selA, start)-start, min(selB, end)-start
	sz := f.size() * loupeScale
	pr := ctx.Painter()
	draw := func(c paint.Canvas, size geom.Size) {
		m := pr.MetricsIn("", sz)
		line := pr.ShapeIn("", lineStr, sz)
		h := m.Ascent + m.Descent
		top := (size.H - h) / 2
		ox := size.W/2 - line.CaretX(caret)
		if selA < selB {
			c.FillRect(geom.Rect{
				Min: geom.Pt{X: ox + line.CaretX(selA), Y: top},
				Max: geom.Pt{X: ox + line.CaretX(selB), Y: top + h},
			}, selC)
		}
		c.TextIn("", lineStr, geom.Pt{X: ox, Y: top + m.Ascent}, sz, txC)
		drawCaret(c, ox+line.CaretX(caret), top, top+h, caretC)
	}
	return Decorated{
		Color: bg, Radius: loupeH / 2,
		BorderColor: border, BorderWidth: 1,
		Child: Canvas{W: loupeW, H: loupeH, Clip: true, Draw: draw},
	}
}
