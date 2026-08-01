package widget

import (
	"math"
	"strings"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/text"
)

// TextField is an editable single-line text input: click-to-caret, drag and
// shift-arrow selection, grapheme-aware editing, clipboard shortcuts, and
// caret-keep-visible scrolling. It renders only text, selection, and caret —
// wrap it in Decorated/Padding for chrome, using OnFocus for focus styling.
//
// It is a controlled component: Value is the source of truth. OnChange
// reports edits; the parent re-renders with the new Value (set Value to ""
// after OnSubmit to clear the field).
//
// Known limits (PLAN.md §6.1): LTR caret geometry, no IME composition UI
// yet (committed text works), single line.
type TextField struct {
	Value       string
	Placeholder string
	Size        float32 // 0 → 14
	// Multiline wraps text to the field width and edits across lines:
	// Enter inserts a newline (Cmd/Ctrl+Enter submits), Up/Down move the
	// caret between lines, and the field grows with its content.
	Multiline bool
	OnChange  func(string)
	OnSubmit  func(string)
	OnFocus   func(bool)

	TextColor        paint.Color
	PlaceholderColor paint.Color
	CaretColor       paint.Color
	SelectionColor   paint.Color
}

func (f TextField) size() float32 {
	if f.Size == 0 {
		return 14
	}
	return f.Size
}

// resolvedColors fills unset (fully transparent) colors with sensible defaults,
// so a TextField is usable without configuring every color. In particular the
// caret defaults to the text color — otherwise an unset CaretColor is invisible
// and the field appears to have no cursor.
func (f TextField) resolvedColors() (text, caret, sel, placeholder paint.Color) {
	text = f.TextColor
	if text.A == 0 {
		text = paint.Color{A: 1} // opaque black
	}
	caret = f.CaretColor
	if caret.A == 0 {
		caret = text
	}
	sel = f.SelectionColor
	if sel.A == 0 {
		sel = paint.Color{R: 0.60, G: 0.78, B: 1.0, A: 0.45}
	}
	placeholder = f.PlaceholderColor
	if placeholder.A == 0 {
		placeholder = text
		placeholder.A = 0.45
	}
	return
}

func (f TextField) CreateState() State { return &textFieldState{} }

type textFieldState struct {
	StateBase[TextField]
	ed      text.Editor
	ctx     Ctx // captured at Init, for ticker teardown
	focused bool
	scrollX float32
	// blink is seconds since the caret last moved or the field gained focus.
	// The caret shows for the first half of each period, so it stays solid
	// right after any activity, then blinks.
	blink float64
	// lastWidth is the box width from the last layout, used by multiline
	// caret navigation and hit testing.
	lastWidth float32

	// IME preedit state: composing text displayed inline at the caret
	// (underlined) until committed or cancelled.
	preedit       string
	preeditCursor int
}

// display returns the string to render: content with the preedit spliced
// in at the caret, plus the preedit's rune range for styling.
func (s *textFieldState) display() (str string, preStart, preEnd int) {
	if s.preedit == "" {
		return s.ed.Text(), 0, 0
	}
	runes := []rune(s.ed.Text())
	caret := s.ed.Caret
	pre := []rune(s.preedit)
	out := make([]rune, 0, len(runes)+len(pre))
	out = append(out, runes[:caret]...)
	out = append(out, pre...)
	out = append(out, runes[caret:]...)
	return string(out), caret, caret + len(pre)
}

func (s *textFieldState) change(ctx Ctx) {
	s.activity()
	if f := s.W(); f.OnChange != nil {
		f.OnChange(s.ed.Text())
	}
	s.SetState(nil)
}

// caretPeriod is the full blink cycle in seconds (~530ms per half — a common
// desktop cadence).
const caretPeriod = 1.06

// caretBlink advances the blink clock while the field is focused and motion is
// allowed, requesting its own repaints so the caret toggles. It reports
// inactive (returns false) so it does NOT count as a settling animation — a
// blinking caret must not make the app read as "perpetually animating" (that
// would spin any settle loop forever). When unfocused or under reduce-motion it
// stops requesting frames entirely and the caret is solid.
type caretBlink struct{ s *textFieldState }

func (c caretBlink) Tick(dt float64) bool {
	if !c.s.focused || c.s.ctx.ReduceMotion() {
		return false
	}
	c.s.blink += dt
	c.s.ctx.Invalidate() // keep frames coming for the blink, without claiming to animate
	return false
}

func (s *textFieldState) Init(ctx Ctx) {
	s.ctx = ctx
	ctx.AddTicker(caretBlink{s})
}

func (s *textFieldState) Dispose() { s.ctx.RemoveTicker(caretBlink{s}) }

// activity resets the blink so the caret is solid right after typing or moving,
// then resumes blinking after an idle half-period.
func (s *textFieldState) activity() { s.blink = 0 }

// caretVisible reports whether to draw the caret this frame: only when focused
// without a selection; solid under reduce-motion, else on for the first half of
// each blink period.
func (s *textFieldState) caretVisible() bool {
	if !s.focused || s.ed.HasSelection() {
		return false
	}
	if s.ctx.ReduceMotion() {
		return true
	}
	return math.Mod(s.blink, caretPeriod) < caretPeriod/2
}

func (s *textFieldState) line(ctx Ctx) text.Line {
	return ctx.Painter().Shape(s.ed.Text(), s.W().size())
}

// paraLines returns the wrapped lines of the current content at the last
// laid-out width (multiline mode).
func (s *textFieldState) paraLines(ctx Ctx) []text.Line {
	w := s.lastWidth
	if w <= 0 {
		w = 1e9
	}
	return ctx.Painter().Paragraph(s.ed.Text(), s.W().size(), w)
}

// lineOf returns the index of the wrapped line containing rune index idx.
func lineOf(lines []text.Line, idx int) int {
	for i, l := range lines {
		if idx <= l.End || i == len(lines)-1 {
			return i
		}
	}
	return 0
}

// moveVertical moves the caret to the adjacent wrapped line, keeping x.
func (s *textFieldState) moveVertical(ctx Ctx, dir int, extend bool) {
	lines := s.paraLines(ctx)
	if len(lines) == 0 {
		return
	}
	li := lineOf(lines, s.ed.Caret)
	x := lines[li].CaretX(s.ed.Caret - lines[li].Start)
	li += dir
	if li < 0 || li >= len(lines) {
		return
	}
	target := lines[li]
	idx := target.Start + target.IndexAt(x)
	if idx > target.End {
		idx = target.End
	}
	s.ed.MoveTo(idx, extend)
	s.SetState(nil)
}

// indexAtPt maps a box-local point to a rune index.
func (s *textFieldState) indexAtPt(ctx Ctx, p geom.Pt) int {
	f := s.W()
	if !f.Multiline {
		return s.line(ctx).IndexAt(p.X + s.scrollX)
	}
	lines := s.paraLines(ctx)
	if len(lines) == 0 {
		return 0
	}
	m := ctx.Painter().Metrics(f.size())
	li := int(p.Y / m.LineHeight())
	if li < 0 {
		li = 0
	}
	if li >= len(lines) {
		li = len(lines) - 1
	}
	l := lines[li]
	idx := l.Start + l.IndexAt(p.X)
	if idx > l.End {
		idx = l.End
	}
	return idx
}

func (s *textFieldState) Build(ctx Ctx) Widget {
	f := s.W()
	if f.Value != s.ed.Text() {
		s.ed.SetText(f.Value)
		s.ed.End(false)
	}

	onKey := func(k shell.Key) {
		if k.Kind != shell.KeyPress {
			return
		}
		s.activity() // keep the caret solid while interacting
		shift := k.Mods&shell.ModShift != 0
		switch k.Code {
		case shell.KeyLeft:
			s.ed.Move(-1, shift)
			s.SetState(nil)
		case shell.KeyRight:
			s.ed.Move(1, shift)
			s.SetState(nil)
		case shell.KeyUp:
			if f.Multiline {
				s.moveVertical(ctx, -1, shift)
			}
		case shell.KeyDown:
			if f.Multiline {
				s.moveVertical(ctx, 1, shift)
			}
		case shell.KeyHome:
			s.ed.Home(shift)
			s.SetState(nil)
		case shell.KeyEnd:
			s.ed.End(shift)
			s.SetState(nil)
		case shell.KeyBackspace:
			s.ed.DeleteBackward()
			s.change(ctx)
		case shell.KeyDelete:
			s.ed.DeleteForward()
			s.change(ctx)
		case shell.KeyTab:
			// Multiline fields indent; single-line Tab is reserved for focus
			// traversal (not yet implemented), so it's a no-op there.
			if f.Multiline {
				s.ed.Insert("\t")
				s.change(ctx)
			}
		case shell.KeyEnter:
			if f.Multiline && !k.Mods.Command() {
				s.ed.Insert("\n")
				s.change(ctx)
				return
			}
			if f.OnSubmit != nil {
				f.OnSubmit(s.ed.Text())
			}
		case shell.KeyEscape:
			s.ed.MoveTo(s.ed.Caret, false) // collapse selection
			s.SetState(nil)
		case shell.KeyA:
			if k.Mods.Command() {
				s.ed.SelectAll()
				s.SetState(nil)
			}
		case shell.KeyC:
			if k.Mods.Command() && s.ed.HasSelection() {
				if cb := ctx.Clipboard(); cb != nil {
					_ = cb.ClipboardWrite(s.ed.SelectedText())
				}
			}
		case shell.KeyX:
			if k.Mods.Command() && s.ed.HasSelection() {
				if cb := ctx.Clipboard(); cb != nil {
					_ = cb.ClipboardWrite(s.ed.SelectedText())
				}
				s.ed.Insert("")
				s.change(ctx)
			}
		case shell.KeyV:
			if k.Mods.Command() {
				if cb := ctx.Clipboard(); cb != nil {
					if t, err := cb.ClipboardRead(); err == nil && t != "" {
						s.ed.Insert(sanitize(t))
						s.change(ctx)
					}
				}
			}
		}
	}

	onText := func(t string) {
		if f.Multiline {
			t = sanitizeMultiline(t)
		} else {
			t = sanitize(t)
		}
		if t != "" {
			s.ed.Insert(t)
			s.change(ctx)
		}
	}

	onComposition := func(c shell.Composition) {
		switch c.Kind {
		case shell.CompositionStart:
			s.preedit, s.preeditCursor = "", 0
		case shell.CompositionUpdate:
			s.preedit, s.preeditCursor = c.Preedit, c.Cursor
		case shell.CompositionEnd:
			s.preedit, s.preeditCursor = "", 0
			if c.Committed != "" {
				s.ed.Insert(sanitize(c.Committed))
				s.change(ctx)
				return
			}
		}
		s.SetState(nil)
	}

	return Interactive{
		Handler: Handler{
			OnPress: func(p geom.Pt) {
				s.activity()
				s.ed.MoveTo(s.indexAtPt(ctx, p), false)
				s.SetState(nil)
			},
			OnDrag: func(p, _ geom.Pt) {
				s.activity()
				s.ed.MoveTo(s.indexAtPt(ctx, p), true)
				s.SetState(nil)
			},
			OnDoubleTap: func() {
				// OnPress already placed the caret at the click; select the word
				// around it.
				s.activity()
				s.ed.SelectWordAt(s.ed.Caret)
				s.SetState(nil)
			},
			OnText:        onText,
			OnKey:         onKey,
			OnComposition: onComposition,
			OnFocus: func(v bool) {
				s.focused = v
				s.activity() // caret solid on focus, then blinks
				if f.OnFocus != nil {
					f.OnFocus(v)
				}
				s.SetState(nil)
			},
		},
		Child: fieldView{state: s},
	}
}

func sanitize(t string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, t)
}

func sanitizeMultiline(t string) string {
	return strings.Map(func(r rune) rune {
		if (r < ' ' && r != '\n') || r == 0x7f {
			return -1
		}
		return r
	}, t)
}

// fieldView is the render widget painting the field content from state.
type fieldView struct {
	state *textFieldState
}

func (v fieldView) createBox(ctx Ctx) layout.Box {
	return &fieldBox{state: v.state, painter: ctx.Painter()}
}
func (v fieldView) updateBox(ctx Ctx, b layout.Box) {
	fb := b.(*fieldBox)
	fb.state, fb.painter = v.state, ctx.Painter()
}
func (v fieldView) childWidgets() []Widget          { return nil }
func (v fieldView) attach(layout.Box, []layout.Box) {}

type fieldBox struct {
	state   *textFieldState
	painter *paint.Painter
	size    geom.Size
}

func (b *fieldBox) Layout(cs layout.Constraints) geom.Size {
	f := b.state.W()
	m := b.painter.Metrics(f.size())
	want := geom.Size{
		W: b.painter.MeasureWidth(b.state.ed.Text(), f.size()),
		H: m.Ascent + m.Descent,
	}
	// A text field fills its available width (clicks in the empty area
	// must land on the field); it shrink-wraps only when unbounded.
	if cs.BoundedW() {
		want.W = cs.Max.W
	}
	if f.Multiline && cs.BoundedW() {
		lines := b.painter.Paragraph(b.state.ed.Text(), f.size(), cs.Max.W)
		if n := len(lines); n > 1 {
			want.H += float32(n-1) * m.LineHeight()
		}
	}
	b.size = cs.Constrain(want)
	b.state.lastWidth = b.size.W

	if f.Multiline {
		b.state.scrollX = 0
		return b.size
	}
	// Keep the caret visible: adjust scrollX so it lies within the box.
	caretX := b.painter.Shape(b.state.ed.Text(), f.size()).CaretX(b.state.ed.Caret)
	if caretX-b.state.scrollX > b.size.W-2 {
		b.state.scrollX = caretX - b.size.W + 2
	}
	if caretX-b.state.scrollX < 0 {
		b.state.scrollX = caretX
	}
	if b.state.scrollX < 0 {
		b.state.scrollX = 0
	}
	return b.size
}

func (b *fieldBox) Size() geom.Size { return b.size }

func (b *fieldBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.state.W().Multiline {
		b.paintMultiline(c, at)
		return
	}
	f := b.state.W()
	txC, caretC, selC, phC := f.resolvedColors()
	sz := f.size()
	m := b.painter.Metrics(sz)
	display, preStart, preEnd := b.state.display()
	line := b.painter.Shape(display, sz)
	origin := geom.Pt{X: at.X - b.state.scrollX, Y: at.Y}
	baseline := at.Y + m.Ascent
	composing := preEnd > preStart

	c.PushClip(geom.Rect{Min: at, Max: at.Add(b.size.Pt())})

	if start, end := b.state.ed.Selection(); start != end && !composing {
		x0 := origin.X + line.CaretX(start)
		x1 := origin.X + line.CaretX(end)
		c.FillRect(geom.Rect{
			Min: geom.Pt{X: x0, Y: at.Y},
			Max: geom.Pt{X: x1, Y: at.Y + b.size.H},
		}, selC)
	}

	if len(display) == 0 && f.Placeholder != "" {
		c.Text(f.Placeholder, geom.Pt{X: origin.X, Y: baseline}, sz, phC)
	} else {
		c.Text(display, geom.Pt{X: origin.X, Y: baseline}, sz, txC)
	}

	if composing {
		// Underline the preedit segment (IME convention).
		x0 := origin.X + line.CaretX(preStart)
		x1 := origin.X + line.CaretX(preEnd)
		y := baseline + m.Descent*0.6
		c.Line(geom.Pt{X: x0, Y: y}, geom.Pt{X: x1, Y: y}, 1.5, caretC)
	}

	if b.state.caretVisible() {
		caretIdx := b.state.ed.Caret
		if composing {
			caretIdx = preStart + b.state.preeditCursor
		}
		x := origin.X + line.CaretX(caretIdx)
		c.Line(geom.Pt{X: x, Y: at.Y}, geom.Pt{X: x, Y: at.Y + b.size.H}, 1.5, caretC)
	}

	c.PopClip()
}

// paintMultiline draws wrapped lines with per-line selection rects and the
// caret on its wrapped line. (Preedit rendering in multiline mode reuses
// the committed-text path; inline preedit arrives with focused IME work.)
func (b *fieldBox) paintMultiline(c paint.Canvas, at geom.Pt) {
	f := b.state.W()
	txC, caretC, selC, phC := f.resolvedColors()
	sz := f.size()
	m := b.painter.Metrics(sz)
	txt := b.state.ed.Text()
	lines := b.painter.Paragraph(txt, sz, b.size.W)
	lineH := m.LineHeight()

	c.PushClip(geom.Rect{Min: at, Max: at.Add(b.size.Pt())})

	selStart, selEnd := b.state.ed.Selection()
	runes := []rune(txt)
	for i, l := range lines {
		top := at.Y + float32(i)*lineH
		baseline := top + m.Ascent

		if selStart != selEnd && selStart < l.End && selEnd > l.Start {
			a, z := max(selStart, l.Start), min(selEnd, l.End)
			x0 := at.X + l.CaretX(a-l.Start)
			x1 := at.X + l.CaretX(z-l.Start)
			c.FillRect(geom.Rect{
				Min: geom.Pt{X: x0, Y: top},
				Max: geom.Pt{X: x1, Y: top + lineH},
			}, selC)
		}

		lineText := strings.TrimRight(string(runes[l.Start:l.End]), "\n")
		c.Text(lineText, geom.Pt{X: at.X, Y: baseline}, sz, txC)
	}

	if len(runes) == 0 && f.Placeholder != "" {
		c.Text(f.Placeholder, geom.Pt{X: at.X, Y: at.Y + m.Ascent}, sz, phC)
	}

	if b.state.caretVisible() && len(lines) > 0 {
		li := lineOf(lines, b.state.ed.Caret)
		x := at.X + lines[li].CaretX(b.state.ed.Caret-lines[li].Start)
		top := at.Y + float32(li)*lineH
		c.Line(geom.Pt{X: x, Y: top}, geom.Pt{X: x, Y: top + lineH}, 1.5, caretC)
	}

	c.PopClip()
}

// Semantics reports the field's value and focus for assistive technology.
func (b *fieldBox) Semantics() layout.SemInfo {
	return layout.SemInfo{
		Role:    layout.RoleTextField,
		Label:   b.state.W().Placeholder,
		Value:   b.state.ed.Text(),
		Focused: b.state.focused,
	}
}

func (b *fieldBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H {
		*hits = append(*hits, layout.Hit{Box: b, Pos: p})
	}
}
