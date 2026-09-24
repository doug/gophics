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
// Known limits: LTR caret geometry; text can be dragged within a field but
// not between fields or out of the app.
//
// It behaves like the platform's own field. The editing keys follow the
// conventions under the user's hands — Cmd/Alt and the Emacs bindings on
// Apple keyboards, Ctrl and Home/End on a PC's (shell.GestureTuning.MacKeys);
// word and line movement and deletion (Windows' word-right lands at the next
// word's start, a Mac's at this word's end), a goal column that survives a
// short line, Home/End on the visual line of wrapped text, undo that groups a
// typed run the way native undo does, shift-click, double- and triple-click
// selection that extends by words or lines while dragging, a right-click
// edit menu, a selection that keeps scrolling while the pointer is held past
// the field's edge, selected text that a mouse can drag to a new place (or
// copy there with Alt/Option), and on touch the long-press menu with
// selection handles, a magnifier over a finger dragging the caret or a grip,
// a tap inside the selection keeping it, and a password field's glimpse of
// the character just typed. Obscure makes it a password field on every platform at once: bullets on screen,
// nothing on the clipboard, secure entry from the soft keyboard. ReadOnly,
// Disabled, MaxLength and Keyboard are the remaining knobs a native field has.
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
	// Autofocus takes keyboard focus when the field mounts, even if another
	// widget holds it. A field never focuses itself otherwise — mounting one
	// does not raise the keyboard — so this is how a screen names the one
	// field it opens into, or focuses a field that appears in response to
	// an action, such as edit-in-place. See Interactive.Autofocus.
	Autofocus bool
	// ConsumesTab reports whether something built around this field wants Tab
	// rather than focus traversal — an autocomplete with a suggestion
	// highlighted, say. Nil means no. A multiline field claims Tab for
	// indentation regardless.
	ConsumesTab func() bool
	// OnKeyPreview sees each key before the field acts on it and returns true
	// to consume it. It exists because keyboard events reach exactly one
	// widget — the focused one, with no bubbling — so a control built *around*
	// a field cannot otherwise see the keys typed into it. Autocomplete uses
	// it to take Up, Down, Tab and Escape for its suggestion list while the
	// caret stays in the field.
	OnKeyPreview func(shell.Key) bool

	TextColor        paint.Color
	PlaceholderColor paint.Color
	CaretColor       paint.Color
	SelectionColor   paint.Color

	// Obscure masks the content as a password field does: every character
	// draws as a bullet, the selection cannot be copied or cut, word
	// movement treats the whole value as one word (a mask should not reveal
	// where the words are), and the platform keyboard is asked for secure
	// entry with autocorrect and suggestions off.
	Obscure bool
	// ReadOnly allows the caret, selection and copying but no edits, and does
	// not raise a soft keyboard — a field for reading, as on every platform.
	ReadOnly bool
	// Disabled makes the field inert: not focusable, not editable, drawn
	// dimmed, and reported disabled to assistive technology.
	Disabled bool
	// MaxLength caps the content in runes (0 is unlimited). Typing, pasting
	// and IME commits that would exceed it are truncated to fit, which is
	// how native fields with a limit behave.
	MaxLength int
	// Keyboard hints the soft keyboard layout on platforms that have one.
	Keyboard shell.TextInputType
	// NoAutocorrect turns the platform's autocorrect and predictive text off.
	// The zero value keeps them on, which is the platform default for a
	// plain text field.
	NoAutocorrect bool
}

// dragUnit is what a drag selects by after a multi-click.
type dragUnit uint8

const (
	dragChars dragUnit = iota
	dragWords
	dragLines
)

// revealFor is how long a password field shows the character a finger just
// typed. iOS and Android both do this; a mouse-and-keyboard field does not.
const revealFor = 1200 * time.Millisecond

// shown is the text as rendered: the content, or a bullet per rune when
// obscured (except the one just typed on touch, for a moment). Every shaping
// site goes through this so the caret, hit-testing, selection and painting
// agree on the same string.
func (s *textFieldState) shown() string {
	if !s.W().Obscure {
		return s.ed.Text()
	}
	runes := []rune(s.ed.Text())
	out := []rune(strings.Repeat("\u2022", len(runes)))
	if s.revealOn && s.revealIdx < len(runes) {
		out[s.revealIdx] = runes[s.revealIdx]
	}
	return string(out)
}

// revealLast shows the rune just typed into a password field until
// revealFor passes, then masks it. Touch only: the glimpse exists because a
// thumb on glass cannot feel which key it hit.
func (s *textFieldState) revealLast(ctx Ctx) {
	if !s.W().Obscure || !ctx.Input().PointerIsTouch() || s.ed.Caret() == 0 {
		return
	}
	s.revealOn, s.revealIdx = true, s.ed.Caret()-1
	if s.revealTimer != nil {
		s.revealTimer.Stop()
	}
	// The timer fires on its own goroutine, and Post is the only way back to
	// the UI one. Without a runner (a bare Owner, a headless embedding) there
	// is no Post, so no timer: the glimpse then lasts until the next
	// keystroke or caret move masks it, the same policy as startBlink.
	post := ctx.Post()
	if post == nil {
		return
	}
	s.revealTimer = time.AfterFunc(revealFor, func() {
		post(func() {
			s.revealOn = false
			s.SetState(nil)
		})
	})
}

// forgetReveal masks a revealed character early — on any movement, so a
// password never shows a character the caret is no longer beside.
func (s *textFieldState) forgetReveal() { s.revealOn = false }

// dragByUnit extends a word- or line-anchored selection to cover the unit
// under idx as well as the anchor unit, whichever side idx is on.
func (s *textFieldState) dragByUnit(idx int) {
	a0, a1 := s.unitAnchor[0], s.unitAnchor[1]
	probe := s.ed // a copy: selecting on it leaves the field's own caret alone
	if s.dragUnit == dragWords {
		probe.SelectWordAt(idx)
	} else {
		probe.SelectLineAt(idx)
	}
	u0, u1 := probe.Selection()
	if u0 < a0 {
		s.ed.SetSelection(a1, u0) // dragging backwards: the caret leads at the start
	} else {
		s.ed.SetSelection(a0, u1)
	}
}

// visualLine is the wrapped line the caret is on and its rune span, for the
// Home/End a wrapped field means: the visual line, not the paragraph.
func (s *textFieldState) visualLine(ctx Ctx) (start, end int, ok bool) {
	lines := s.paraLines(ctx)
	if len(lines) == 0 {
		return 0, 0, false
	}
	l := lines[lineOf(lines, s.ed.Caret())]
	return l.Start, l.End, true
}

// editable reports whether edits are accepted at all.
func (f TextField) editable() bool { return !f.ReadOnly && !f.Disabled }

// fit trims an insertion so the content stays within MaxLength.
func (s *textFieldState) fit(t string) string {
	max := s.W().MaxLength
	if max <= 0 {
		return t
	}
	a, b := s.ed.Selection()
	room := max - (s.ed.Len() - (b - a))
	if room <= 0 {
		return ""
	}
	if r := []rune(t); len(r) > room {
		return string(r[:room])
	}
	return t
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

	// goalX is the x the caret is trying to keep while moving up and down —
	// native fields remember it across a short line, so Up, Up, Down returns
	// to the column you started in rather than the short line's end. Any
	// other movement or edit forgets it.
	goalX  float32
	goalOK bool
	// A double click then drag selects by whole words, a triple click then
	// drag by whole lines, on every desktop; unitAnchor is the unit first
	// selected, which the drag never shrinks below.
	dragUnit   dragUnit
	unitAnchor [2]int
	// Click counting at press time. The dispatcher reports a double tap on
	// the second release, but a native double-click-drag selects the word on
	// the second press and extends by words while the button stays down —
	// so the field counts presses itself, on the platform's double-tap
	// window, and selects the unit before any drag can start.
	clicks       int
	lastPress    time.Time
	lastPressPos geom.Pt
	// revealOn/revealIdx: the rune a touch keyboard just typed into a
	// password field, shown for revealFor before it turns into a bullet —
	// the glimpse iOS and Android give so a thumb can check itself.
	revealOn    bool
	revealIdx   int
	revealTimer *time.Timer
	// auto keeps a drag selection scrolling while the pointer is held past
	// the box; lastHeight, with lastWidth, is the box it is judged against.
	auto       *autoScroller
	lastHeight float32
	// dragText is a mouse carrying the selected text; dropIdx is where it
	// would land, -1 until the pointer has moved.
	dragText bool
	dropIdx  int
	// loupeOn is the finger's magnifier, shown in the overlay.
	loupeOn  bool
	loupeTok OverlayToken
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
		return s.shown(), 0, 0
	}
	runes := []rune(s.shown())
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

func (s *textFieldState) Dispose() {
	s.stopBlink()
	s.hideLoupe()
	// The menu lives in the overlay, beside the tree rather than under this
	// field, so unmounting the field does not take it down: without this a
	// route pop or a list scroll-out left the scrim and Cut/Copy/Paste bar
	// up, bound to a state that no longer exists.
	s.closeMenu()
	if s.revealTimer != nil {
		s.revealTimer.Stop()
		s.revealTimer = nil
	}
	if a := s.auto; a != nil && a.added {
		s.ctx.el.owner.RemoveTicker(a)
	}
}

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
	return ctx.Painter().ShapeIn("", s.shown(), s.W().size())
}

// paraLines returns the wrapped lines of the current content at the last
// laid-out width (multiline mode).
func (s *textFieldState) paraLines(ctx Ctx) []text.Line {
	w := s.lastWidth
	if w <= 0 {
		w = 1e9
	}
	return ctx.Painter().ParagraphIn("", s.shown(), s.W().size(), w)
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
	if !s.goalOK {
		s.goalX = lines[li].CaretX(s.ed.Caret() - lines[li].Start)
		s.goalOK = true
	}
	x := s.goalX
	li += dir
	// Past the first or last line, every native field goes to the end of the
	// text in that direction rather than staying put.
	if li < 0 {
		s.ed.Home(extend)
		s.SetState(nil)
		return
	}
	if li >= len(lines) {
		s.ed.End(extend)
		s.SetState(nil)
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
		return geom.Pt{X: pr.ShapeIn("", s.shown(), f.size()).CaretX(idx) - s.scrollX, Y: 0}
	}
	w := s.lastWidth
	if w <= 0 {
		w = 1e9
	}
	lines := pr.ParagraphIn("", s.shown(), f.size(), w)
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
	if !s.ed.HasSelection() || s.W().Obscure {
		return // a password field never hands its content to the clipboard
	}
	if cb := ctx.Clipboard(); cb != nil {
		_ = cb.ClipboardWrite(s.ed.SelectedText())
	}
}

func (s *textFieldState) cutSelection(ctx Ctx) {
	// The keyboard path checks editable before reaching here; the edit menu
	// did not, and Cut edited a ReadOnly field.
	if !s.ed.HasSelection() || s.W().Obscure || !s.W().editable() {
		return
	}
	s.copySelection(ctx)
	s.ed.Insert("")
	s.change(ctx)
}

func (s *textFieldState) pasteClipboard(ctx Ctx) {
	// Editable first, then read: reading the clipboard is what makes iOS put
	// up its "pasted from" notice, and a field that cannot take the paste
	// has no business triggering it.
	cb := ctx.Clipboard()
	if cb == nil || !s.W().editable() {
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
	t = s.fit(t)
	if t == "" {
		return
	}
	s.ed.Replace(t) // a paste is its own undo step, never merged with typing
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

// editOps exposes the field to the edit menu. An action the field cannot
// perform is left nil so the menu omits it rather than listing an item that
// would silently do nothing: an Obscure field offers no Cut or Copy, and a
// field that cannot be edited (ReadOnly, Disabled) offers no Cut or Paste.
func (s *textFieldState) editOps(ctx Ctx) selectionOps {
	ops := selectionOps{
		HasSelection: s.ed.HasSelection,
		AllSelected: func() bool {
			a, b := s.ed.Selection()
			return a == 0 && b == len([]rune(s.ed.Text())) && b > 0
		},
		SelectAll: s.selectAll,
	}
	if !s.W().Obscure {
		ops.Copy = func() { s.copySelection(ctx) }
	}
	if s.W().editable() {
		ops.Paste = func() { s.pasteClipboard(ctx) }
		if !s.W().Obscure {
			ops.Cut = func() { s.cutSelection(ctx) }
		}
	}
	return ops
}

// selectWordAt selects the word around idx — or, in an Obscure field, the
// whole value: a word highlight over the bullets would give away where the
// secret's words begin and end, which native password fields avoid the same
// way. A drag from such a selection extends by lines, never by words, for
// the same reason.
func (s *textFieldState) selectWordAt(idx int) {
	if s.W().Obscure {
		s.selectAll()
		s.dragUnit = dragLines
		return
	}
	s.ed.SelectWordAt(idx)
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
		// The preview runs before the Kind filter: a consumer that tracks key
		// releases must see them, and one that does not can filter its own.
		if f.OnKeyPreview != nil && f.OnKeyPreview(k) {
			return
		}
		if k.Kind != shell.KeyPress {
			return
		}
		s.activity()           // keep the caret solid while interacting
		s.revealPending = true // a key press moves or edits the caret: keep it visible
		s.keyPress(ctx, f, k)
	}

	onText := func(t string) {
		if f.Multiline {
			t = sanitizeMultiline(t)
		} else {
			t = sanitize(t)
		}
		t = s.fit(t)
		if t != "" && f.editable() {
			s.goalOK = false
			s.ed.Insert(t)
			s.revealPending = true // typing moves the caret: keep it visible
			s.revealLast(ctx)
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
		Autofocus: f.Autofocus,
		Gestures: Gestures{
			// A multiline field indents with Tab; a single-line one hands it to
			// focus traversal unless whatever wraps it says otherwise.
			ConsumesTab: func() bool {
				return f.Multiline || (f.ConsumesTab != nil && f.ConsumesTab())
			},
			OnPress: func(p geom.Pt) {
				s.activity()
				s.closeMenu() // a new press replaces whatever the last one raised
				s.stopAuto()
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
				s.dragUnit = dragChars
				s.goalOK = false
				s.forgetReveal()
				// Second and third presses in quick succession, in place.
				now := time.Now()
				window := time.Duration(ctx.el.owner.Gestures.Resolved().DoubleTap * float64(time.Second))
				if now.Sub(s.lastPress) <= window && near(p, s.lastPressPos, 8) {
					s.clicks++
				} else {
					s.clicks = 1
				}
				s.lastPress, s.lastPressPos = now, p
				if s.clicks >= 2 && !ctx.Input().PointerIsTouch() {
					idx := s.indexAtPt(ctx, p)
					if s.clicks == 2 {
						s.dragUnit = dragWords
						s.selectWordAt(idx)
					} else {
						s.ed.SelectLineAt(idx)
						s.dragUnit = dragLines
					}
					s.unitAnchor[0], s.unitAnchor[1] = s.ed.Selection()
					s.handles = false
					s.SetState(nil)
					return
				}
				// A finger tapping inside its own selection is asking for the
				// menu, not moving the caret — iOS and Android both keep the
				// selection and show it. A mouse click collapses; that is
				// what a mouse means.
				if ctx.Input().PointerIsTouch() && s.ed.HasSelection() {
					idx := s.indexAtPt(ctx, p)
					if a, b := s.ed.Selection(); idx >= a && idx <= b {
						s.handles = true
						s.SetState(nil)
						if acts := editActionsFor(ctx, s.editOps(ctx)); len(acts) > 0 {
							s.dismissMenu = ShowEditMenu(ctx, s.pressGlobal, acts)
						}
						return
					}
				}
				if s.beginTextDrag(ctx, p) {
					return
				}
				s.handles = false // a plain press ends the touch selection
				// Shift-click extends the selection from the anchor, as every
				// desktop field does; a plain click places the caret.
				extend := ctx.Input().Mods()&shell.ModShift != 0
				s.ed.MoveTo(s.indexAtPt(ctx, p), extend)
				s.SetState(nil)
			},
			OnTripleTap: func() {
				// Third click: the line (multiline) or the whole value, and a
				// drag from here selects by lines.
				s.activity()
				s.ed.SelectLineAt(s.ed.Caret())
				s.dragUnit = dragLines
				s.unitAnchor[0], s.unitAnchor[1] = s.ed.Selection()
				s.SetState(nil)
			},
			OnSecondaryTap: func(p geom.Pt) {
				// Right-click: select the word under the pointer when nothing
				// is selected, then the edit menu — Cocoa's and Windows'
				// behavior both.
				s.activity()
				s.closeMenu()
				if !s.ed.HasSelection() {
					s.selectWordAt(s.indexAtPt(ctx, p))
				}
				s.SetState(nil)
				if acts := editActionsFor(ctx, s.editOps(ctx)); len(acts) > 0 {
					s.dismissMenu = ShowEditMenu(ctx, ctx.Input().Pointer(), acts)
				}
			},
			OnLongPress: func() {
				// The touch idiom for reaching an editor's actions. A press on
				// bare text selects the word under it first, so the menu that
				// follows has something to act on — which is what both
				// platforms do, and what makes Copy meaningful without a
				// keyboard.
				s.activity()
				if !s.ed.HasSelection() {
					s.selectWordAt(s.indexAtPt(ctx, s.pressLocal))
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
				s.dragTo(ctx, p)
			},
			OnDoubleTap: func() {
				// OnPress already placed the caret at the click; select the word
				// around it, and a drag from here selects by words.
				s.activity()
				s.dragUnit = dragWords
				s.selectWordAt(s.ed.Caret())
				s.unitAnchor[0], s.unitAnchor[1] = s.ed.Selection()
				s.SetState(nil)
			},
			OnText:        onText,
			OnKey:         onKey,
			OnComposition: onComposition,
			OnPressEnd: func() {
				s.endDrag(ctx)
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
				// Raise the keyboard here, not from the build below, because
				// this runs inside the pointer event that moved focus — and a
				// mobile browser only opens the soft keyboard when the element
				// is focused during a user gesture. The build happens in the
				// next animation frame, by which time the activation has
				// expired: the hidden input is focused, silently, and no
				// keyboard ever appears. On a phone that is a caret blinking
				// in a field you cannot type into.
				// Only latch when there is a capability to latch *for*. The
				// first build can run before the window is wired, so
				// TextInput() is nil then; marking the keyboard shown on that
				// pass would skip the build-time path forever and the field
				// would never register its handlers at all.
				if s.imeShown != v && ctx.TextInput() != nil {
					s.imeShown = v
					s.softKeyboard(ctx, v, f, onText, onKey, onComposition)
				}
				if f.OnFocus != nil {
					f.OnFocus(v)
				}
				s.SetState(nil)
			},
		},
		Child: fieldView{state: s},
	}
	if f.Disabled {
		// Inert: no gestures means not focusable (focus follows OnText/OnKey),
		// not tappable, not autofocused — a disabled native field is skipped
		// by Tab and ignores the pointer. The view still draws, dimmed.
		view.Gestures = Gestures{}
		view.Autofocus = false
	}
	// The build-time path remains, for focus gained without a focus *event*: a
	// focusable widget mounted while nothing has focus takes it silently, and
	// on that path nothing else would ever call Show — so the field would miss
	// the keyboard type, the autocorrect hint, and the OnReplace autocorrect
	// needs.
	//
	// It cannot raise a soft keyboard, and is not trying to. There is no user
	// gesture in progress on that path, and a mobile browser refuses outside
	// one; what it does is configure the IME so a later gesture finds it ready.
	// The gesture case is handled in OnFocus above, which is the one that has
	// to open the keyboard.
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
	f TextField,
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
	if f.ReadOnly || f.Disabled {
		return // nothing to type into; a native read-only field raises no keyboard
	}
	ti.Show(shell.TextInputOptions{
		Type:        f.Keyboard,
		Autocorrect: !f.NoAutocorrect && !f.Obscure,
		Secure:      f.Obscure,
	}, shell.TextInputHandler{
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
		W: b.painter.MeasureWidthIn("", b.state.shown(), f.size()),
		H: m.Ascent + m.Descent,
	}
	// A text field fills its available width (clicks in the empty area
	// must land on the field); it shrink-wraps only when unbounded.
	if cs.BoundedW() {
		want.W = cs.Max.W
	}
	if f.Multiline && cs.BoundedW() {
		lines := b.painter.ParagraphIn("", b.state.shown(), f.size(), cs.Max.W)
		if n := len(lines); n > 1 {
			want.H += float32(n-1) * m.LineHeight()
		}
	}
	b.size = cs.Constrain(want)
	b.state.lastWidth = b.size.W
	b.state.lastHeight = b.size.H

	if f.Multiline {
		b.state.scrollX = 0
		return b.size
	}
	// Keep the caret visible: adjust scrollX so it lies within the box.
	caretX := b.painter.ShapeIn("", b.state.shown(), f.size()).CaretX(b.state.ed.Caret())
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
	if b.state.dragText && b.state.dropIdx >= 0 {
		// Where the carried text would land.
		drawCaret(c, origin.X+line.CaretX(b.state.dropIdx), at.Y, at.Y+b.size.H, caretC)
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
	txt := b.state.shown() // bullets for a password: the plaintext never paints
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
	if d := b.state.dropIdx; b.state.dragText && d >= 0 && len(lines) > 0 {
		li := lineOf(lines, d)
		x := at.X + lines[li].CaretX(d-lines[li].Start)
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
	f := b.state.W()
	return layout.SemInfo{
		Role:     layout.RoleTextField,
		Label:    f.Placeholder,
		Value:    b.state.shown(), // a screen reader gets bullets for a password too
		Focused:  b.state.focused,
		Disabled: f.Disabled,
		Secure:   f.Obscure,
	}
}

func (b *fieldBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H {
		*hits = append(*hits, layout.Hit{Box: b, Pos: p})
	}
}

// keyPress applies one key press under the platform's editing conventions.
//
// Two conventions exist and a field has to speak the one under the user's
// hands. Apple's: Cmd is the command key, Alt moves by word, Cmd+arrow goes
// to the ends of the line and document, and the Emacs bindings every Cocoa
// text view honors (Ctrl+A/E to the line ends, Ctrl+K to kill to the end,
// Ctrl+F/B/D/H for the arrows and deletes). Windows and Linux: Ctrl is both
// the command key and the word modifier, Home/End go to the line ends,
// Ctrl+Home/End to the document's, and Ctrl+Y is redo beside Ctrl+Shift+Z.
// Which one applies is shell.GestureTuning.MacKeys, set by the platform —
// a Mac browser and an iPad's hardware keyboard are Apple's, an Android
// keyboard is a PC's.
func (s *textFieldState) keyPress(ctx Ctx, f TextField, k shell.Key) {
	mac := ctx.el.owner.Gestures.Resolved().MacKeys
	shift := k.Mods&shell.ModShift != 0
	var cmd, word, emacs bool
	if mac {
		cmd = k.Mods&shell.ModSuper != 0
		word = k.Mods&shell.ModAlt != 0
		emacs = k.Mods&shell.ModCtrl != 0 && !cmd
	} else {
		// Ctrl, or Super for a headless host that only knows "the command
		// key": Mods.Command() is what every caller used before there were
		// two conventions, and a PC keyboard's Win key is not a modifier a
		// field would otherwise see.
		cmd = k.Mods.Command()
		word = k.Mods&shell.ModCtrl != 0
	}
	// A password field has no words to move by: its mask must not reveal
	// where they are, so word movement becomes line movement.
	if f.Obscure {
		word = false
		if mac && k.Mods&shell.ModAlt != 0 {
			cmd = true
		}
	}
	edit := f.editable()
	moved := func() { s.SetState(nil) }
	changed := func() { s.change(ctx) }
	// Only Up/Down keep the goal column; every other key forgets it, and so
	// does any edit. A revealed password character is hidden by any key too.
	if k.Code != shell.KeyUp && k.Code != shell.KeyDown {
		s.goalOK = false
	}
	s.forgetReveal()
	// Home and End in a wrapped field mean the visual line. LineHome/LineEnd
	// on the editor mean the paragraph, which is right for single-line text
	// and for the Emacs bindings' notion of a line.
	lineHome := func(extend bool) {
		if start, _, ok := s.visualLine(ctx); ok && f.Multiline {
			s.ed.MoveTo(start, extend)
			return
		}
		s.ed.LineHome(extend)
	}
	lineEnd := func(extend bool) {
		if _, end, ok := s.visualLine(ctx); ok && f.Multiline {
			s.ed.MoveTo(end, extend)
			return
		}
		s.ed.LineEnd(extend)
	}

	// Emacs bindings first: on a Mac, Ctrl+A is "line start", not "select
	// all", which is why cmd above is Super alone there.
	if emacs {
		switch k.Code {
		case shell.KeyA:
			s.ed.LineHome(shift)
			moved()
			return
		case shell.KeyE:
			s.ed.LineEnd(shift)
			moved()
			return
		case shell.KeyF:
			s.ed.Move(1, shift)
			moved()
			return
		case shell.KeyB:
			s.ed.Move(-1, shift)
			moved()
			return
		case shell.KeyN:
			if f.Multiline {
				s.moveVertical(ctx, 1, shift)
			}
			return
		case shell.KeyP:
			if f.Multiline {
				s.moveVertical(ctx, -1, shift)
			}
			return
		case shell.KeyD:
			if edit {
				s.ed.DeleteForward()
				changed()
			}
			return
		case shell.KeyH:
			if edit {
				s.ed.DeleteBackward()
				changed()
			}
			return
		case shell.KeyK:
			if edit {
				s.ed.DeleteToLineEnd()
				changed()
			}
			return
		}
	}

	switch k.Code {
	case shell.KeyLeft, shell.KeyRight:
		dir := 1
		if k.Code == shell.KeyLeft {
			dir = -1
		}
		switch {
		case mac && cmd && dir < 0:
			lineHome(shift)
		case mac && cmd:
			lineEnd(shift)
		case word && !mac && dir > 0:
			s.ed.MoveWordStart(shift) // Windows: to the start of the next word
		case word:
			s.ed.MoveWord(dir, shift)
		default:
			s.ed.Move(dir, shift)
		}
		moved()
	case shell.KeyUp, shell.KeyDown:
		dir := 1
		if k.Code == shell.KeyUp {
			dir = -1
		}
		switch {
		case mac && cmd && dir < 0:
			s.ed.Home(shift) // Cmd+Up: start of the document
			moved()
		case mac && cmd:
			s.ed.End(shift)
			moved()
		case f.Multiline:
			s.moveVertical(ctx, dir, shift)
		case mac && dir < 0:
			s.ed.Home(shift) // a single-line Cocoa field: Up is the start
			moved()
		case mac:
			s.ed.End(shift)
			moved()
		}
	case shell.KeyHome:
		if !mac && cmd {
			s.ed.Home(shift) // Ctrl+Home: the document
		} else {
			lineHome(shift)
		}
		moved()
	case shell.KeyEnd:
		if !mac && cmd {
			s.ed.End(shift)
		} else {
			lineEnd(shift)
		}
		moved()
	case shell.KeyBackspace:
		if !edit {
			return
		}
		switch {
		case mac && cmd:
			s.ed.DeleteToLineStart()
		case word:
			s.ed.DeleteWordBackward()
		default:
			s.ed.DeleteBackward()
		}
		changed()
	case shell.KeyDelete:
		if !edit {
			return
		}
		if word {
			s.ed.DeleteWordForward()
		} else {
			s.ed.DeleteForward()
		}
		changed()
	case shell.KeyTab:
		// Multiline fields indent; a single-line Tab is focus traversal.
		if f.Multiline && edit {
			if t := s.fit("\t"); t != "" {
				s.ed.Insert(t)
				changed()
			}
		}
	case shell.KeyEnter:
		if f.Multiline && !cmd {
			if edit {
				if t := s.fit("\n"); t != "" {
					s.ed.Insert(t)
					changed()
				}
			}
			return
		}
		if f.OnSubmit != nil {
			f.OnSubmit(s.ed.Text())
		}
	case shell.KeyEscape:
		if s.dragText {
			s.dragText, s.dropIdx = false, -1 // put the text back
			s.SetState(nil)
			return
		}
		s.ed.MoveTo(s.ed.Caret(), false) // collapse selection
		moved()
	case shell.KeyA:
		if cmd {
			s.selectAll()
		}
	case shell.KeyC:
		if cmd {
			s.copySelection(ctx)
		}
	case shell.KeyX:
		if cmd && edit {
			s.cutSelection(ctx)
		}
	case shell.KeyV:
		if cmd {
			s.pasteClipboard(ctx)
		}
	case shell.KeyZ:
		if !cmd || !edit {
			return
		}
		if shift {
			if s.ed.Redo() {
				changed()
			}
		} else if s.ed.Undo() {
			changed()
		}
	case shell.KeyY:
		if cmd && !mac && edit && s.ed.Redo() {
			changed()
		}
	}
}

// near reports whether two points are within slop of each other.
func near(a, b geom.Pt, slop float32) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx+dy*dy <= slop*slop
}
