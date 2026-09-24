package text

import (
	"unicode"

	"github.com/go-text/typesetting/segmenter"
)

// Editor is a single-line text editing model: content, caret, and
// selection. It is pure state — widgets own one and render from it.
//
// The caret and anchor (see Caret/Anchor) are rune indices in [0, Len]. When
// they differ, [min,max) is the selection and the caret is the moving end.
// Horizontal movement and deletion step by grapheme cluster (UAX #29), so emoji
// and combining sequences behave as single characters.
type Editor struct {
	runes  []rune
	caret  int // rune index of the caret, always in [0, len(runes)]
	anchor int // rune index of the selection anchor, always in [0, len(runes)]

	seg segmenter.Segmenter

	// Undo history; see snapshot.
	undo, redo []editSnapshot
	lastOp     editOp
	lastCaret  int // caret after the last typed insert, for coalescing
}

// SetText replaces the content and clamps the caret/selection.
func (e *Editor) SetText(s string) {
	if string(e.runes) != s {
		e.snapshot(opReplace)
	}
	e.runes = []rune(s)
	e.caret = clampIdx(e.caret, len(e.runes))
	e.anchor = clampIdx(e.anchor, len(e.runes))
}

// Caret returns the caret position, a rune index in [0, Len].
func (e *Editor) Caret() int { return e.caret }

// Anchor returns the selection anchor, a rune index in [0, Len]; when it differs
// from Caret, [min,max) of the two is the selection.
func (e *Editor) Anchor() int { return e.anchor }

// SetSelection places the anchor and caret, clamping both into range. This is
// the only way to set them from outside the package: a raw write could leave an
// index past the end of the text and panic the next edit.
func (e *Editor) SetSelection(anchor, caret int) {
	e.anchor = clampIdx(anchor, len(e.runes))
	e.caret = clampIdx(caret, len(e.runes))
}

// Text returns the content.
func (e *Editor) Text() string { return string(e.runes) }

// Len returns the content length in runes.
func (e *Editor) Len() int { return len(e.runes) }

// Selection returns the selected range [start, end); start == end means no
// selection.
func (e *Editor) Selection() (start, end int) {
	if e.anchor <= e.caret {
		return e.anchor, e.caret
	}
	return e.caret, e.anchor
}

// HasSelection reports whether a nonempty range is selected.
func (e *Editor) HasSelection() bool { return e.anchor != e.caret }

// SelectedText returns the selected content.
func (e *Editor) SelectedText() string {
	s, en := e.Selection()
	return string(e.runes[s:en])
}

// Insert replaces the selection (or inserts at the caret) with s.
func (e *Editor) Insert(s string) {
	e.snapshot(opType)
	e.insertRaw(s)
}

// DeleteBackward deletes the selection, or the grapheme before the caret.
func (e *Editor) DeleteBackward() { e.deleteTo(e.prevBoundary(e.caret)) }

// DeleteForward deletes the selection, or the grapheme after the caret.
func (e *Editor) DeleteForward() { e.deleteTo(e.nextBoundary(e.caret)) }

// deleteTo deletes the selection, or the range between the caret and idx, as
// one undo step. When there is nothing to delete — Backspace at the start of
// a field, Delete at its end — it records nothing: a snapshot per no-op would
// fill the history with identical entries, and the next Undo would then appear
// to do nothing.
func (e *Editor) deleteTo(idx int) {
	if !e.HasSelection() && idx == e.caret {
		return
	}
	e.snapshot(opDelete)
	if !e.HasSelection() {
		e.anchor = idx
	}
	e.insertRaw("")
}

// Move moves the caret one grapheme left (dir < 0) or right (dir > 0).
// With extend, the anchor stays put (shift-selection); otherwise a
// selection collapses to its edge in the movement direction.
func (e *Editor) Move(dir int, extend bool) {
	if !extend && e.HasSelection() {
		start, end := e.Selection()
		if dir < 0 {
			e.caret = start
		} else {
			e.caret = end
		}
		e.anchor = e.caret
		return
	}
	if dir < 0 {
		e.caret = e.prevBoundary(e.caret)
	} else {
		e.caret = e.nextBoundary(e.caret)
	}
	if !extend {
		e.anchor = e.caret
	}
}

// MoveTo places the caret at the given rune index; with extend the anchor
// stays put.
func (e *Editor) MoveTo(idx int, extend bool) {
	e.caret = clampIdx(idx, len(e.runes))
	if !extend {
		e.anchor = e.caret
	}
}

// Home moves to the start; End to the end.
func (e *Editor) Home(extend bool) { e.MoveTo(0, extend) }
func (e *Editor) End(extend bool)  { e.MoveTo(len(e.runes), extend) }

// SelectAll selects the whole content.
func (e *Editor) SelectAll() {
	e.anchor = 0
	e.caret = len(e.runes)
}

// SelectWordAt selects the word (run of letters/digits/underscore) containing
// idx — the double-click-to-select behavior. If idx isn't on a word character,
// it selects the single character there, matching common editor behavior.
func (e *Editor) SelectWordAt(idx int) {
	n := len(e.runes)
	if n == 0 {
		return
	}
	idx = clampIdx(idx, n)
	lo, hi := idx, idx
	for lo > 0 && isWordRune(e.runes[lo-1]) {
		lo--
	}
	for hi < n && isWordRune(e.runes[hi]) {
		hi++
	}
	if lo == hi { // not on a word char — select the single character
		if idx < n {
			hi = idx + 1
		} else if idx > 0 {
			lo = idx - 1
		}
	}
	e.anchor, e.caret = lo, hi
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// prevBoundary returns the grapheme boundary before idx.
func (e *Editor) prevBoundary(idx int) int {
	if idx <= 0 {
		return 0
	}
	prev := 0
	for _, b := range e.boundaries() {
		if b >= idx {
			break
		}
		prev = b
	}
	return prev
}

// nextBoundary returns the grapheme boundary after idx.
func (e *Editor) nextBoundary(idx int) int {
	for _, b := range e.boundaries() {
		if b > idx {
			return b
		}
	}
	return len(e.runes)
}

// boundaries returns all grapheme start indices plus the end index.
func (e *Editor) boundaries() []int {
	e.seg.Init(e.runes)
	iter := e.seg.GraphemeIterator()
	var out []int
	for iter.Next() {
		out = append(out, iter.Grapheme().Offset)
	}
	out = append(out, len(e.runes))
	return out
}

func clampIdx(v, n int) int {
	if v < 0 {
		return 0
	}
	if v > n {
		return n
	}
	return v
}

// CaretX returns the x position of the caret placed before rune index idx.
//
// Positions come from the glyphs' own X, not from summing advances. Summing
// only works while glyph order matches logical order, which bidi breaks: after
// reordering (UAX #9 rule L2) the glyphs are in *visual* order, so "every glyph
// before this cluster" is no longer a contiguous run of pixels, and the total is
// the width of a set of glyphs scattered across the line.
//
// In a right-to-left run the caret before a cluster sits at that glyph's right
// edge, because "before" means earlier in reading order and reading runs the
// other way. Getting this wrong puts the caret on the far side of the character
// being typed, which is the usual symptom of an LTR-only implementation.
func (l Line) CaretX(idx int) float32 {
	target := l.Start + idx
	for i, g := range l.Glyphs {
		if g.Cluster != target {
			continue
		}
		if l.rtlAt(i) {
			return g.X + g.Advance
		}
		return g.X
	}
	// Past the last cluster: the caret belongs at the line's trailing edge,
	// which is the left for an RTL line and the right for an LTR one.
	if l.RTL {
		return 0
	}
	return l.Width
}

// rtlAt reports whether the glyph at visual index i belongs to a right-to-left
// run.
//
// Determined from the clusters of its visual neighbours rather than from stored
// per-glyph state: glyphs adjacent in the slice are adjacent on screen, so
// within a run the cluster index rises in LTR and falls in RTL. That makes the
// direction a local property of the reordered line and needs nothing threaded
// through from shaping.
func (l Line) rtlAt(i int) bool {
	if i+1 < len(l.Glyphs) && l.Glyphs[i+1].Cluster < l.Glyphs[i].Cluster {
		return true
	}
	if i > 0 && l.Glyphs[i-1].Cluster > l.Glyphs[i].Cluster {
		return true
	}
	// A run of one carries no local evidence, so fall back to the line's base
	// direction — which is the right answer for a lone RTL word on an RTL line.
	if len(l.Glyphs) == 1 {
		return l.RTL
	}
	return false
}

// IndexAt returns the rune index whose caret position is nearest to x
// (for click-to-position): the inverse of CaretX.
//
// Each glyph offers two caret positions — before its cluster and after it, in
// reading order — and the half of the glyph x falls in picks the nearer one.
// Which edge is "before" depends on the glyph's direction: in a left-to-right
// run it is the left edge, in a right-to-left run the right one (see CaretX).
// An LTR-only rule puts a click on the left half of an RTL glyph *before* the
// cluster, whose caret CaretX then draws at the glyph's right edge — a full
// glyph away from where the user clicked.
func (l Line) IndexAt(x float32) int {
	if len(l.Glyphs) == 0 {
		return 0
	}
	var pen float32
	for i, g := range l.Glyphs {
		rtl := l.rtlAt(i)
		if x < pen+g.Advance/2 {
			// Left half: the caret at the glyph's left edge.
			if rtl {
				return l.nextCluster(g.Cluster) - l.Start
			}
			return g.Cluster - l.Start
		}
		pen += g.Advance
		if x < pen {
			// Right half: the caret at the glyph's right edge.
			if rtl {
				return g.Cluster - l.Start
			}
			return l.nextCluster(g.Cluster) - l.Start
		}
	}
	// Past the last glyph: the caret at the line's right edge, which is the
	// start of the text on an RTL line and the end on an LTR one (CaretX
	// places both there).
	if l.RTL {
		return 0
	}
	return l.End - l.Start
}

func (l Line) nextCluster(cluster int) int {
	next := l.End
	for _, g := range l.Glyphs {
		if g.Cluster > cluster && g.Cluster < next {
			next = g.Cluster
		}
	}
	return next
}

// --- Word, line and document movement ---------------------------------------
//
// These are what a native field does for Alt/Ctrl+arrow, Home/End and their
// deleting cousins. Word boundaries follow the convention macOS and GTK share:
// moving left lands at the start of the previous word, moving right at the end
// of the next one, skipping the whitespace and punctuation between. Windows
// moves right to the *start* of the next word; that difference is real, small,
// and not worth a second code path until someone measures it.

// wordStartBefore is the start of the word ending at or before idx.
func (e *Editor) wordStartBefore(idx int) int {
	i := idx
	for i > 0 && !isWordRune(e.runes[i-1]) {
		i--
	}
	for i > 0 && isWordRune(e.runes[i-1]) {
		i--
	}
	return i
}

// wordEndAfter is the end of the word starting at or after idx.
func (e *Editor) wordEndAfter(idx int) int {
	i, n := idx, len(e.runes)
	for i < n && !isWordRune(e.runes[i]) {
		i++
	}
	for i < n && isWordRune(e.runes[i]) {
		i++
	}
	return i
}

// MoveWord moves the caret one word left (dir < 0) or right, extending the
// selection when extend is set. Without extend, a selection collapses to its
// edge first, the way Move does.
func (e *Editor) MoveWord(dir int, extend bool) {
	if !extend && e.HasSelection() {
		start, end := e.Selection()
		if dir < 0 {
			e.caret = start
		} else {
			e.caret = end
		}
		e.anchor = e.caret
		return
	}
	if dir < 0 {
		e.caret = e.wordStartBefore(e.caret)
	} else {
		e.caret = e.wordEndAfter(e.caret)
	}
	if !extend {
		e.anchor = e.caret
	}
}

// MoveWordStart is Windows' word-right: to the start of the next word rather
// than the end of this one. Ctrl+Right on a PC lands before the next word,
// where Alt+Right on a Mac lands after the current one; both are "a word",
// and a user of each expects theirs. Left is the same on both platforms.
func (e *Editor) MoveWordStart(extend bool) {
	if !extend && e.HasSelection() {
		_, end := e.Selection()
		e.caret, e.anchor = end, end
		return
	}
	i, n := e.caret, len(e.runes)
	for i < n && isWordRune(e.runes[i]) {
		i++
	}
	for i < n && !isWordRune(e.runes[i]) {
		i++
	}
	e.caret = i
	if !extend {
		e.anchor = e.caret
	}
}

// DeleteWordBackward deletes from the caret to the start of the previous word,
// or the selection if there is one.
func (e *Editor) DeleteWordBackward() { e.deleteTo(e.wordStartBefore(e.caret)) }

// DeleteWordForward deletes from the caret to the end of the next word, or the
// selection if there is one.
func (e *Editor) DeleteWordForward() { e.deleteTo(e.wordEndAfter(e.caret)) }

// lineStart is the index just after the newline before idx, or 0.
func (e *Editor) lineStart(idx int) int {
	i := idx
	for i > 0 && e.runes[i-1] != '\n' {
		i--
	}
	return i
}

// lineEnd is the index of the newline at or after idx, or Len.
func (e *Editor) lineEnd(idx int) int {
	i, n := idx, len(e.runes)
	for i < n && e.runes[i] != '\n' {
		i++
	}
	return i
}

// LineHome moves to the start of the caret's line — Home on a multiline field,
// Cmd+Left on a Mac, Ctrl+A in Emacs bindings. In single-line text this is
// the start of the text.
func (e *Editor) LineHome(extend bool) { e.MoveTo(e.lineStart(e.caret), extend) }

// LineEnd is LineHome's counterpart.
func (e *Editor) LineEnd(extend bool) { e.MoveTo(e.lineEnd(e.caret), extend) }

// DeleteToLineStart deletes from the caret to the start of its line — Cmd+
// Backspace on a Mac. A selection is deleted instead.
func (e *Editor) DeleteToLineStart() { e.deleteTo(e.lineStart(e.caret)) }

// DeleteToLineEnd deletes from the caret to the end of its line — Ctrl+K. On
// an empty remainder it deletes the newline itself, as Emacs does, so
// repeated Ctrl+K eats lines.
func (e *Editor) DeleteToLineEnd() {
	end := e.lineEnd(e.caret)
	if end == e.caret && end < len(e.runes) {
		end++ // the newline
	}
	e.deleteTo(end)
}

// SelectLineAt selects the whole line containing idx — a triple click.
func (e *Editor) SelectLineAt(idx int) {
	idx = clampIdx(idx, len(e.runes))
	e.anchor = e.lineStart(idx)
	e.caret = e.lineEnd(idx)
}

// --- Undo ---------------------------------------------------------------------
//
// A native field groups consecutive typing into one undo step and separates it
// from a deletion or a paste, so Cmd+Z takes back "the word you just typed",
// not one character or the whole session. The model here: every mutation is
// classed as typing, deletion, or a replacement (paste, autocorrect, SetText
// from outside); a snapshot is pushed before a mutation whenever its class
// differs from the previous one, or the caret moved in between. Typing after
// typing coalesces; anything else is a boundary.

type editOp uint8

const (
	opNone editOp = iota
	opType
	opDelete
	opReplace
)

type editSnapshot struct {
	runes         []rune
	caret, anchor int
}

// snapshot records the state before a mutation of class op, unless it
// coalesces with the previous one.
func (e *Editor) snapshot(op editOp) {
	if op == e.lastOp && op == opType && e.caret == e.lastCaret {
		e.lastCaret = e.caret // still contiguous; updated after the insert below
		return
	}
	e.pushUndo()
	e.redo = e.redo[:0]
	e.lastOp = op
}

const maxUndo = 200

// pushUndo records the current state as an undo step, dropping the oldest
// once the history is full.
func (e *Editor) pushUndo() {
	e.undo = append(e.undo, editSnapshot{append([]rune(nil), e.runes...), e.caret, e.anchor})
	if len(e.undo) > maxUndo {
		e.undo = e.undo[1:]
	}
}

// insertRaw is Insert without the undo bookkeeping.
func (e *Editor) insertRaw(s string) {
	start, end := e.Selection()
	ins := []rune(s)
	out := make([]rune, 0, len(e.runes)-(end-start)+len(ins))
	out = append(out, e.runes[:start]...)
	out = append(out, ins...)
	out = append(out, e.runes[end:]...)
	e.runes = out
	e.caret = start + len(ins)
	e.anchor = e.caret
	e.lastCaret = e.caret
}

// Replace is Insert as one undo step that never coalesces with typing: a
// paste, an autocorrect, a programmatic change.
func (e *Editor) Replace(s string) {
	e.snapshot(opReplace)
	e.insertRaw(s)
}

// Undo reverts the last edit group; false if there was none.
func (e *Editor) Undo() bool {
	if len(e.undo) == 0 {
		return false
	}
	e.redo = append(e.redo, editSnapshot{append([]rune(nil), e.runes...), e.caret, e.anchor})
	snap := e.undo[len(e.undo)-1]
	e.undo = e.undo[:len(e.undo)-1]
	e.runes, e.caret, e.anchor = snap.runes, snap.caret, snap.anchor
	e.lastOp = opNone
	return true
}

// Redo reapplies the last undone group; false if there was none.
func (e *Editor) Redo() bool {
	if len(e.redo) == 0 {
		return false
	}
	e.pushUndo()
	snap := e.redo[len(e.redo)-1]
	e.redo = e.redo[:len(e.redo)-1]
	e.runes, e.caret, e.anchor = snap.runes, snap.caret, snap.anchor
	e.lastOp = opNone
	return true
}

// CanUndo and CanRedo report whether the corresponding menu item is enabled.
func (e *Editor) CanUndo() bool { return len(e.undo) > 0 }
func (e *Editor) CanRedo() bool { return len(e.redo) > 0 }
