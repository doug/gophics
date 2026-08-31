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
// Known limits: LTR caret geometry, single line.
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

	// Where the current press landed, kept because OnLongPress reports no
	// position: pressLocal indexes into the text, pressGlobal anchors the edit
	// menu. dismissMenu closes an open one (nil when none is up).
	pressLocal  geom.Pt
	pressGlobal geom.Pt
	dismissMenu func()

	// handles is set when a finger made the selection, which is when the grips
	// are worth drawing; dragHandle is which one a drag is currently moving
	// (-1 none, 0 start, 1 end).
	handles    bool
	dragHandle int

	// imeShown is what the platform was last told about this field's keyboard,
	// so Build can drive Show/Hide idempotently.
	imeShown bool

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
	idx := min(target.Start+target.IndexAt(x), target.End)
	s.ed.MoveTo(idx, extend)
	s.SetState(nil)
}

// indexAtPt maps a box-local point to a rune index.
// The clipboard operations, named once so the keyboard shortcuts and the
// long-press edit menu cannot drift apart. They were inline in the key handler
// and reachable only by Cmd+C/X/V, which on a phone — no Command key — meant
// text could be selected and never copied.

// Selection handles — the draggable grips at each end of a touch selection.
//
// A drawn editor gets none of the platform's, and without them a selection made
// on a phone can only be replaced, never adjusted: there is no keyboard to
// shift-arrow with. They are the other half of the long-press menu.
//
// They are shown only for a selection a finger made (handles set by
// OnLongPress). A mouse user adjusts with shift-click and would find two dots
// under their text puzzling.
const (
	selHandleRadius = 7
	// A fingertip is a great deal larger than the dot it is aiming at, so the
	// grab area is generous. Platforms do the same; the dot is a target, not a
	// hitbox.
	selHandleGrab = 18
)

// caretPt returns the local position of the caret for rune index idx: the top
// of the caret, in the same space OnPress reports. It is the inverse of
// indexAtPt, and follows the same single/multiline split.
func (s *textFieldState) caretPt(pr *paint.Painter, idx int) geom.Pt {
	f := s.W()
	if !f.Multiline {
		return geom.Pt{X: pr.ShapeIn("", s.ed.Text(), f.size()).CaretX(idx) - s.scrollX, Y: 0}
	}
	w := s.lastWidth
	if w <= 0 {
		w = 1e9
	}
	lines := pr.ParagraphIn("", s.ed.Text(), f.size(), w)
	if len(lines) == 0 {
		return geom.Pt{}
	}
	li := lineOf(lines, idx)
	l := lines[li]
	m := pr.MetricsIn("", f.size())
	rel := max(idx-l.Start, 0)
	return geom.Pt{X: l.CaretX(rel), Y: float32(li) * m.LineHeight()}
}

// handleCentres returns the two handle positions in local coordinates, and
// whether there is a selection to show them for.
func (s *textFieldState) handleCentres(pr *paint.Painter) (lo, hi geom.Pt, ok bool) {
	a, b := s.ed.Selection()
	if a == b || !s.handles {
		return lo, hi, false
	}
	m := pr.MetricsIn("", s.W().size())
	drop := m.Ascent + m.Descent + selHandleRadius
	lo = s.caretPt(pr, a)
	hi = s.caretPt(pr, b)
	lo.Y += drop
	hi.Y += drop
	return lo, hi, true
}

// moveHandle drags one end of the selection to p, keeping the other anchored.
//
// The ends swap when they cross, which is what a user doing it expects: drag
// the left grip past the right one and the selection does not collapse, it
// inverts. Tracking which handle is which through the swap is why this holds
// the anchor explicitly rather than reusing MoveTo's extend flag.
func (s *textFieldState) moveHandle(ctx Ctx, p geom.Pt) {
	if s.moveHandleTo(s.indexAtPt(ctx, p)) {
		s.SetState(nil)
	}
}

// moveHandleTo is the drag itself, in text-index space: the hit testing that
// produced idx is the caller's business, which is what makes this testable
// without pixels.
func (s *textFieldState) moveHandleTo(idx int) (changed bool) {
	a, b := s.ed.Selection()
	anchor := b
	if s.dragHandle == 1 {
		anchor = a
	}
	if idx == anchor {
		return false // a zero-width selection would drop the handles mid-drag
	}
	if idx < anchor {
		s.dragHandle = 0
	} else {
		s.dragHandle = 1
	}
	s.ed.MoveTo(anchor, false)
	s.ed.MoveTo(idx, true)
	return true
}

// handleAt reports which handle the point p is grabbing: -1 none, 0 the
// selection start, 1 the end.
func (s *textFieldState) handleAt(ctx Ctx, p geom.Pt) int {
	return s.handleAtPt(ctx.Painter(), p)
}

func (s *textFieldState) handleAtPt(pr *paint.Painter, p geom.Pt) int {
	lo, hi, ok := s.handleCentres(pr)
	if !ok {
		return -1
	}
	dl, dh := dist2(p, lo), dist2(p, hi)
	grab := float32(selHandleGrab * selHandleGrab)
	switch {
	case dl <= grab && dl <= dh:
		return 0
	case dh <= grab:
		return 1
	}
	return -1
}

func (s *textFieldState) copySelection(ctx Ctx) {
	if !s.ed.HasSelection() {
		return
	}
	if cb := ctx.Clipboard(); cb != nil {
		_ = cb.ClipboardWrite(s.ed.SelectedText())
	}
}

func (s *textFieldState) cutSelection(ctx Ctx) {
	if !s.ed.HasSelection() {
		return
	}
	s.copySelection(ctx)
	s.ed.Insert("")
	s.change(ctx)
}

func (s *textFieldState) pasteClipboard(ctx Ctx) {
	cb := ctx.Clipboard()
	if cb == nil {
		return
	}
	t, err := cb.ClipboardRead()
	if err != nil || t == "" {
		return
	}
	if s.W().Multiline {
		t = sanitizeMultiline(t)
	} else {
		t = sanitize(t)
	}
	if t == "" {
		return
	}
	s.ed.Insert(t)
	s.revealPending = true
	s.change(ctx)
}

func (s *textFieldState) selectAll() {
	s.ed.SelectAll()
	s.SetState(nil)
}

// closeMenu dismisses an open edit menu, if any. Safe to call when none is up.
func (s *textFieldState) closeMenu() {
	if s.dismissMenu != nil {
		s.dismissMenu()
		s.dismissMenu = nil
	}
}

// editOps exposes the field to the edit menu.
func (s *textFieldState) editOps(ctx Ctx) selectionOps {
	return selectionOps{
		HasSelection: s.ed.HasSelection,
		AllSelected: func() bool {
			a, b := s.ed.Selection()
			return a == 0 && b == len([]rune(s.ed.Text())) && b > 0
		},
		Cut:       func() { s.cutSelection(ctx) },
		Copy:      func() { s.copySelection(ctx) },
		Paste:     func() { s.pasteClipboard(ctx) },
		SelectAll: s.selectAll,
	}
}

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
	li := max(int(p.Y/m.LineHeight()), 0)
	if li >= len(lines) {
		li = len(lines) - 1
	}
	l := lines[li]
	idx := min(l.Start+l.IndexAt(p.X), l.End)
	return idx
}

func (s *textFieldState) Build(ctx Ctx) Widget {
	f := s.W()
	// The nearest enclosing Scroll's caret-into-view service, if any.
	s.reveal, _ = ctx.Of[*scrollReveal]()
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
				s.selectAll()
			}
		case shell.KeyC:
			if k.Mods.Command() {
				s.copySelection(ctx)
			}
		case shell.KeyX:
			if k.Mods.Command() {
				s.cutSelection(ctx)
			}
		case shell.KeyV:
			if k.Mods.Command() {
				s.pasteClipboard(ctx)
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
		Gestures: Gestures{
			OnPress: func(p geom.Pt) {
				s.activity()
				s.closeMenu() // a new press replaces whatever the last one raised
				// Both spaces: the index needs the local point, the edit menu
				// needs the global one, and a long press reports neither.
				s.pressLocal = p
				s.pressGlobal = ctx.Input().Pointer()
				// A press on a handle grabs it rather than moving the caret —
				// otherwise reaching for the grip collapses the selection it is
				// there to adjust.
				if h := s.handleAt(ctx, p); h >= 0 {
					s.dragHandle = h
					s.SetState(nil)
					return
				}
				s.dragHandle = -1
				s.handles = false // a plain press ends the touch selection
				s.ed.MoveTo(s.indexAtPt(ctx, p), false)
				s.SetState(nil)
			},
			OnLongPress: func() {
				// The touch idiom for reaching an editor's actions. A press on
				// bare text selects the word under it first, so the menu that
				// follows has something to act on — which is what both
				// platforms do, and what makes Copy meaningful without a
				// keyboard.
				s.activity()
				if !s.ed.HasSelection() {
					s.ed.SelectWordAt(s.indexAtPt(ctx, s.pressLocal))
				}
				s.handles = true // a finger made this selection; give it grips
				s.SetState(nil)
				if acts := editActionsFor(ctx, s.editOps(ctx)); len(acts) > 0 {
					s.dismissMenu = ShowEditMenu(ctx, s.pressGlobal, acts)
				}
			},
			OnDrag: func(p, _ geom.Pt) {
				s.activity()
				s.closeMenu() // the selection is moving under it
				if s.dragHandle >= 0 {
					s.moveHandle(ctx, p)
					return
				}
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
			OnPressEnd: func() {
				if s.dragHandle >= 0 {
					s.dragHandle = -1
					// The range just changed, so re-offer the actions for it —
					// the same thing both platforms do when a grip is let go.
					if acts := editActionsFor(ctx, s.editOps(ctx)); len(acts) > 0 {
						s.dismissMenu = ShowEditMenu(ctx, s.pressGlobal, acts)
					}
				}
			},
			OnFocus: func(v bool) {
				s.focused = v
				// Deliberately not closing the edit menu here. Pressing the menu
				// takes focus away from the field, so dismissing on blur tears
				// the menu down between the press and the release — and OnTap,
				// which needs both on the same element, never fires. The menu
				// looks like it works and does nothing.
				//
				// Outside taps are already handled by the menu's own scrim, and
				// a fresh press in the field closes it there.
				s.activity() // caret solid on focus, then blinks
				if f.OnFocus != nil {
					f.OnFocus(v)
				}
				s.SetState(nil)
			},
		},
		Child: fieldView{state: s},
	}
	// Raise or lower the keyboard from the built state rather than from the
	// focus *event*, because a field can be focused without one ever firing: a
	// focusable widget mounted while nothing has focus takes it silently. Doing
	// this on the transition alone meant the first field on a screen — the
	// common case on a phone — never got Show, so it never received the
	// keyboard type, the autocorrect hint, or the OnReplace that autocorrect
	// needs. TextInputActive still answered yes through its focus fallback,
	// which is what hid it.
	if s.focused != s.imeShown {
		s.imeShown = s.focused
		s.softKeyboard(ctx, s.focused, f, onText, onKey, onComposition)
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
		// Autocorrect and predictive replacement. This is the one handler
		// callback mobile uses: replacing a span cannot be expressed as an
		// insertion, and it is what the IME does every time it fixes a word.
		OnReplace: func(start, end int, t string) {
			n := len([]rune(s.ed.Text()))
			if start < 0 || end > n || start > end {
				return // the IME's view lagged behind an edit; drop it
			}
			s.ed.MoveTo(start, false)
			s.ed.MoveTo(end, true)
			if s.W().Multiline {
				t = sanitizeMultiline(t)
			} else {
				t = sanitize(t)
			}
			s.ed.Insert(t)
			s.revealPending = true
			s.change(ctx)
		},
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

// paintSelHandles draws the two grips under a touch selection.
//
// A filled dot with a short stem up to the text, which is the shape both
// platforms use and reads as "grab me" at a glance. Drawn in the caret colour
// so the selection, the caret and the grips are visibly one thing.
func (b *fieldBox) paintSelHandles(c paint.Canvas, at geom.Pt) {
	lo, hi, ok := b.state.handleCentres(b.painter)
	if !ok {
		return
	}
	_, caretC, _, _ := b.state.W().resolvedColors()
	for _, p := range []geom.Pt{lo, hi} {
		g := at.Add(p)
		// Stem: from the dot up to roughly the text baseline, so the grip is
		// attached to the selection rather than floating beneath it.
		c.Line(geom.Pt{X: g.X, Y: g.Y - selHandleRadius*2}, geom.Pt{X: g.X, Y: g.Y}, 1.5, caretC)
		c.FillRRect(geom.Rect{
			Min: geom.Pt{X: g.X - selHandleRadius, Y: g.Y - selHandleRadius},
			Max: geom.Pt{X: g.X + selHandleRadius, Y: g.Y + selHandleRadius},
		}, selHandleRadius, caretC)
	}
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

	b.paintSelHandles(c, at)
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

	b.paintSelHandles(c, at)
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
