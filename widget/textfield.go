package widget

import (
	"math"
	"strings"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/text"
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
// IME composition is handled: preedit text is spliced in at the caret and
// underlined until the input method commits it.
//
// Known limits (PLAN.md §6.1): LTR caret geometry, single line.
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
	ctx     Ctx // captured at Init, for blink scheduling and teardown
	focused bool
	// imeText/imeSelA/imeSelB are what the platform IME was last told the
	// field contains; see syncIME.
	imeText          string
	imeSelA, imeSelB int
	scrollX          float32
	// blink is seconds since the caret last moved or the field gained focus.
	// The caret shows for the first half of each period, so it stays solid
	// right after any activity, then blinks. It is advanced by the flip timer
	// (startBlink), which requests a frame only when visibility changes.
	blink float64
	// blinkTimer wakes the UI at the next caret flip (nil while not blinking);
	// blinkGen invalidates fires from stopped or superseded timers.
	blinkTimer *time.Timer
	blinkGen   int
	// lastWidth is the box width from the last layout, used by multiline
	// caret navigation and hit testing.
	lastWidth float32

	// reveal is the enclosing Scroll's caret-into-view service (nil when the
	// field is not inside a Scroll); revealPending asks the next paint to scroll
	// the caret into view after a user edit or caret move.
	reveal        *scrollReveal
	revealPending bool

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
	caret := s.ed.Caret()
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

// The caret blinks on a wall-clock timer, not a per-frame ticker: a frame is
// requested only when the caret's visibility actually flips (~2/s), so a
// focused-but-idle field no longer pins the app at the display rate. Each armed
// timer wakes the UI goroutine (via Post) at the next half-period boundary,
// advances the blink clock to it, invalidates once, and re-arms. When unfocused
// the timer stops; under reduce-motion it stops too and the caret is solid
// (caretVisible). Because the wakeups are one-shot invalidations — not a
// registered animation — the app still reads as idle between flips and settle
// loops never spin on a blinking caret.

// startBlink (re)arms the flip timer from the current blink phase, stopping any
// pending one first. It arms only while the field is focused, motion is
// allowed, and a runner Post hook exists (widget-only tests run without one;
// the caret simply stays solid there).
func (s *textFieldState) startBlink() {
	s.stopBlink()
	if !s.focused || s.ctx.ReduceMotion() {
		return
	}
	post := s.ctx.Post()
	if post == nil {
		return
	}
	gen := s.blinkGen
	const half = caretPeriod / 2
	delay := half - math.Mod(s.blink, half) // seconds to the next visibility flip
	s.blinkTimer = time.AfterFunc(time.Duration(delay*float64(time.Second)), func() {
		post(func() { s.blinkFlip(gen, delay) })
	})
}

// blinkFlip runs on the UI goroutine when the flip timer fires: it advances the
// blink clock exactly to the boundary the timer was armed for, requests the one
// repaint that shows the flip, and re-arms for the next half-period. A stale
// generation (the timer was stopped or re-armed after this fire was scheduled)
// is ignored.
func (s *textFieldState) blinkFlip(gen int, advance float64) {
	if gen != s.blinkGen || !s.focused {
		return
	}
	s.blink += advance
	s.ctx.Invalidate()
	s.startBlink()
}

// stopBlink cancels any pending flip timer and invalidates in-flight fires.
func (s *textFieldState) stopBlink() {
	s.blinkGen++
	if s.blinkTimer != nil {
		s.blinkTimer.Stop()
		s.blinkTimer = nil
	}
}

func (s *textFieldState) Init(ctx Ctx) { s.ctx = ctx }

func (s *textFieldState) Dispose() { s.stopBlink() }

// activity resets the blink so the caret is solid right after typing or moving,
// then resumes blinking after an idle half-period.
func (s *textFieldState) activity() {
	s.blink = 0
	s.startBlink()
}

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
	return ctx.Painter().ShapeIn("", s.ed.Text(), s.W().size())
}

// paraLines returns the wrapped lines of the current content at the last
// laid-out width (multiline mode).
func (s *textFieldState) paraLines(ctx Ctx) []text.Line {
	w := s.lastWidth
	if w <= 0 {
		w = 1e9
	}
	return ctx.Painter().ParagraphIn("", s.ed.Text(), s.W().size(), w)
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
	li := lineOf(lines, s.ed.Caret())
	x := lines[li].CaretX(s.ed.Caret() - lines[li].Start)
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
	m := ctx.Painter().MetricsIn("", f.size())
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
	// The nearest enclosing Scroll's caret-into-view service, if any.
	s.reveal, _ = Of[*scrollReveal](ctx)
	if f.Value != s.ed.Text() {
		s.ed.SetText(f.Value)
		s.ed.End(false)
	}

	onKey := func(k shell.Key) {
		if k.Kind != shell.KeyPress {
			return
		}
		s.activity()           // keep the caret solid while interacting
		s.revealPending = true // a key press moves or edits the caret: keep it visible
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
			s.ed.MoveTo(s.ed.Caret(), false) // collapse selection
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
			s.revealPending = true // typing moves the caret: keep it visible
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

	view := Interactive{
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
				s.ed.SelectWordAt(s.ed.Caret())
				s.SetState(nil)
			},
			OnText:        onText,
			OnKey:         onKey,
			OnComposition: onComposition,
			OnFocus: func(v bool) {
				s.focused = v
				s.activity() // caret solid on focus, then blinks
				s.softKeyboard(ctx, v, f, onText, onKey, onComposition)
				if f.OnFocus != nil {
					f.OnFocus(v)
				}
				s.SetState(nil)
			},
		},
		Child: fieldView{state: s},
	}
	// Anything that could have moved the text or the selection has happened by
	// now, so this is where the IME's copy is brought back in step.
	s.syncIME(ctx)
	return view
}

// softKeyboard raises or dismisses the platform on-screen keyboard as the field
// gains and loses focus, routing what it produces into the same handlers a
// hardware keyboard feeds.
//
// gophics draws its own editor, so there is no native text field for the
// platform to focus and nothing raises the keyboard on its own. Without this a
// phone shows a caret in a field that cannot be typed into: the field takes
// focus, blinks, and no keyboard ever appears. Desktop shells report no
// TextInput capability because a hardware keyboard already delivers key events,
// so this is a no-op there.
func (s *textFieldState) softKeyboard(
	ctx Ctx,
	focused bool,
	_ TextField,
	onText func(string),
	onKey func(shell.Key),
	onComposition func(shell.Composition),
) {
	ti := ctx.TextInput()
	if ti == nil {
		return // desktop, or a shell without the capability
	}
	if !focused {
		ti.Hide()
		return
	}

	// TextField exposes no keyboard-type or password hints yet, so ask for the
	// default layout with autocorrect on. Adding those knobs is a separate
	// change to its API, not something to infer here.
	ti.Show(shell.TextInputOptions{Autocorrect: true}, shell.TextInputHandler{
		OnText: onText,
		OnComposing: func(pre string) {
			onComposition(shell.Composition{Kind: shell.CompositionUpdate, Preedit: pre, Cursor: len([]rune(pre))})
		},
		// The IME reports editing keys separately from text; the editor already
		// knows how to apply them, so they are forwarded as the key presses it
		// would have received from hardware.
		OnEditKey: func(k shell.EditKey) {
			var code shell.KeyCode
			switch k {
			case shell.EditBackspace:
				code = shell.KeyBackspace
			case shell.EditEnter:
				code = shell.KeyEnter
			case shell.EditLeft:
				code = shell.KeyLeft
			case shell.EditRight:
				code = shell.KeyRight
			default:
				return
			}
			onKey(shell.Key{Kind: shell.KeyPress, Code: code})
		},
	})
	// Give the IME the surrounding text and the real selection so composition,
	// prediction and select-all have something to work against. Caret twice
	// would claim there is never a selection.
	s.imeText = ""
	s.syncIME(ctx)
}

// syncIME keeps the platform IME's view of the field in step with the editor.
//
// The IME needs the surrounding text and the selection range to do its job:
// autocorrect replaces the word behind the caret, predictive text reads what
// precedes it, and selection replacement needs to know what is selected. Told
// once when the field was focused, that view goes stale on the first
// keystroke, and the keyboard then operates on text the field no longer has --
// selection in particular behaves as though nothing is ever selected.
//
// It is called on every build, which is when anything that could change the
// text or the selection has already happened, and sends nothing when neither
// moved.
func (s *textFieldState) syncIME(ctx Ctx) {
	if !s.focused {
		return
	}
	ti := ctx.TextInput()
	if ti == nil {
		return
	}
	text := s.ed.Text()
	a, b := s.ed.Selection()
	if text == s.imeText && a == s.imeSelA && b == s.imeSelB {
		return
	}
	s.imeText, s.imeSelA, s.imeSelB = text, a, b
	ti.SetText(text, a, b)
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
	m := b.painter.MetricsIn("", f.size())
	want := geom.Size{
		W: b.painter.MeasureWidthIn("", b.state.ed.Text(), f.size()),
		H: m.Ascent + m.Descent,
	}
	// A text field fills its available width (clicks in the empty area
	// must land on the field); it shrink-wraps only when unbounded.
	if cs.BoundedW() {
		want.W = cs.Max.W
	}
	if f.Multiline && cs.BoundedW() {
		lines := b.painter.ParagraphIn("", b.state.ed.Text(), f.size(), cs.Max.W)
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
	caretX := b.painter.ShapeIn("", b.state.ed.Text(), f.size()).CaretX(b.state.ed.Caret())
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

// doReveal asks the enclosing Scroll to bring the caret into view. absX is the
// caret's absolute x; [absTop, absBot] its absolute vertical span. It maps to
// the scroll's content space via the anchor origin and scrolls the smaller of
// the two axes' need per the scroll's axis. Clears the pending flag.
func (b *fieldBox) doReveal(absX, absTop, absBot float32) {
	s := b.state
	s.revealPending = false
	if s.reveal == nil || !s.reveal.have {
		return
	}
	if s.reveal.horizontal() {
		cx := absX - s.reveal.origin.X
		s.reveal.reveal(cx, cx)
		return
	}
	s.reveal.reveal(absTop-s.reveal.origin.Y, absBot-s.reveal.origin.Y)
}

func (b *fieldBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.state.W().Multiline {
		b.paintMultiline(c, at)
		return
	}
	f := b.state.W()
	txC, caretC, selC, phC := f.resolvedColors()
	sz := f.size()
	m := b.painter.MetricsIn("", sz)
	display, preStart, preEnd := b.state.display()
	line := b.painter.ShapeIn("", display, sz)
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
		c.TextIn("", f.Placeholder, geom.Pt{X: origin.X, Y: baseline}, sz, phC)
	} else {
		c.TextIn("", display, geom.Pt{X: origin.X, Y: baseline}, sz, txC)
	}

	if composing {
		// Underline the preedit segment (IME convention).
		x0 := origin.X + line.CaretX(preStart)
		x1 := origin.X + line.CaretX(preEnd)
		y := baseline + m.Descent*0.6
		c.Line(geom.Pt{X: x0, Y: y}, geom.Pt{X: x1, Y: y}, 1.5, caretC)
	}

	if b.state.caretVisible() {
		caretIdx := b.state.ed.Caret()
		if composing {
			caretIdx = preStart + b.state.preeditCursor
		}
		x := origin.X + line.CaretX(caretIdx)
		drawCaret(c, x, at.Y, at.Y+b.size.H, caretC)
	}

	c.PopClip()

	if b.state.revealPending {
		b.doReveal(origin.X+line.CaretX(b.state.ed.Caret()), at.Y, at.Y+b.size.H)
	}
}

// caretWidth is the caret's thickness in logical pixels.
const caretWidth = 1.5

// drawCaret paints the insertion point as a rectangle anchored *at* x rather
// than a stroked line centred on it.
//
// A centred stroke puts half its width to the left of the insertion point, so it
// visibly bleeds into the character before the caret — and at index 0 half of it
// falls outside the field and is clipped away. Anchoring the rect at the boundary
// and growing rightwards is what every platform's caret does, and it makes the
// caret at the start of a field fully visible.
func drawCaret(c paint.Canvas, x, top, bottom float32, col paint.Color) {
	c.FillRect(geom.Rect{
		Min: geom.Pt{X: x, Y: top},
		Max: geom.Pt{X: x + caretWidth, Y: bottom},
	}, col)
}

// paintMultiline draws wrapped lines with per-line selection rects and the
// caret on its wrapped line. (Preedit rendering in multiline mode reuses
// the committed-text path; inline preedit arrives with focused IME work.)
func (b *fieldBox) paintMultiline(c paint.Canvas, at geom.Pt) {
	f := b.state.W()
	txC, caretC, selC, phC := f.resolvedColors()
	sz := f.size()
	m := b.painter.MetricsIn("", sz)
	txt := b.state.ed.Text()
	lines := b.painter.ParagraphIn("", txt, sz, b.size.W)
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
		c.TextIn("", lineText, geom.Pt{X: at.X, Y: baseline}, sz, txC)
	}

	if len(runes) == 0 && f.Placeholder != "" {
		c.TextIn("", f.Placeholder, geom.Pt{X: at.X, Y: at.Y + m.Ascent}, sz, phC)
	}

	if b.state.caretVisible() && len(lines) > 0 {
		li := lineOf(lines, b.state.ed.Caret())
		x := at.X + lines[li].CaretX(b.state.ed.Caret()-lines[li].Start)
		top := at.Y + float32(li)*lineH
		drawCaret(c, x, top, top+lineH, caretC)
	}

	c.PopClip()

	if b.state.revealPending {
		if len(lines) > 0 {
			li := lineOf(lines, b.state.ed.Caret())
			x := at.X + lines[li].CaretX(b.state.ed.Caret()-lines[li].Start)
			top := at.Y + float32(li)*lineH
			b.doReveal(x, top, top+lineH)
		} else {
			b.state.revealPending = false
		}
	}
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
