package text

import (
	"unicode"

	"github.com/go-text/typesetting/segmenter"
)

// Editor is a single-line text editing model: content, caret, and
// selection. It is pure state — widgets own one and render from it.
//
// Caret and Anchor are rune indices in [0, len(runes)]. When they differ,
// [min,max) is the selection and Caret is the moving end. Horizontal
// movement and deletion step by grapheme cluster (UAX #29), so emoji and
// combining sequences behave as single characters.
type Editor struct {
	runes  []rune
	Caret  int
	Anchor int

	seg segmenter.Segmenter
}

// SetText replaces the content and clamps the caret/selection.
func (e *Editor) SetText(s string) {
	e.runes = []rune(s)
	e.Caret = clampIdx(e.Caret, len(e.runes))
	e.Anchor = clampIdx(e.Anchor, len(e.runes))
}

// Text returns the content.
func (e *Editor) Text() string { return string(e.runes) }

// Len returns the content length in runes.
func (e *Editor) Len() int { return len(e.runes) }

// Selection returns the selected range [start, end); start == end means no
// selection.
func (e *Editor) Selection() (start, end int) {
	if e.Anchor <= e.Caret {
		return e.Anchor, e.Caret
	}
	return e.Caret, e.Anchor
}

// HasSelection reports whether a nonempty range is selected.
func (e *Editor) HasSelection() bool { return e.Anchor != e.Caret }

// SelectedText returns the selected content.
func (e *Editor) SelectedText() string {
	s, en := e.Selection()
	return string(e.runes[s:en])
}

// Insert replaces the selection (or inserts at the caret) with s.
func (e *Editor) Insert(s string) {
	start, end := e.Selection()
	ins := []rune(s)
	out := make([]rune, 0, len(e.runes)-(end-start)+len(ins))
	out = append(out, e.runes[:start]...)
	out = append(out, ins...)
	out = append(out, e.runes[end:]...)
	e.runes = out
	e.Caret = start + len(ins)
	e.Anchor = e.Caret
}

// DeleteBackward deletes the selection, or the grapheme before the caret.
func (e *Editor) DeleteBackward() {
	if !e.HasSelection() {
		e.Anchor = e.prevBoundary(e.Caret)
	}
	e.Insert("")
}

// DeleteForward deletes the selection, or the grapheme after the caret.
func (e *Editor) DeleteForward() {
	if !e.HasSelection() {
		e.Anchor = e.nextBoundary(e.Caret)
	}
	e.Insert("")
}

// Move moves the caret one grapheme left (dir < 0) or right (dir > 0).
// With extend, the anchor stays put (shift-selection); otherwise a
// selection collapses to its edge in the movement direction.
func (e *Editor) Move(dir int, extend bool) {
	if !extend && e.HasSelection() {
		start, end := e.Selection()
		if dir < 0 {
			e.Caret = start
		} else {
			e.Caret = end
		}
		e.Anchor = e.Caret
		return
	}
	if dir < 0 {
		e.Caret = e.prevBoundary(e.Caret)
	} else {
		e.Caret = e.nextBoundary(e.Caret)
	}
	if !extend {
		e.Anchor = e.Caret
	}
}

// MoveTo places the caret at the given rune index; with extend the anchor
// stays put.
func (e *Editor) MoveTo(idx int, extend bool) {
	e.Caret = clampIdx(idx, len(e.runes))
	if !extend {
		e.Anchor = e.Caret
	}
}

// Home moves to the start; End to the end.
func (e *Editor) Home(extend bool) { e.MoveTo(0, extend) }
func (e *Editor) End(extend bool)  { e.MoveTo(len(e.runes), extend) }

// SelectAll selects the whole content.
func (e *Editor) SelectAll() {
	e.Anchor = 0
	e.Caret = len(e.runes)
}

// SelectWordAt selects the word (run of letters/digits/underscore) containing
// idx — the double-click-to-select behavior. If idx isn't on a word character,
// it selects the single character there, matching common editor behavior.
func (e *Editor) SelectWordAt(idx int) {
	n := len(e.runes)
	if n == 0 {
		return
	}
	if idx > n {
		idx = n
	}
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
	e.Anchor, e.Caret = lo, hi
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

// CaretX returns the x position of the caret placed before rune index idx,
// as the summed advance of glyphs in earlier clusters. Correct for LTR
// content; RTL caret geometry arrives with paragraph-direction support
// (PLAN.md §6.1).
func (l Line) CaretX(idx int) float32 {
	var x float32
	for _, g := range l.Glyphs {
		if g.Cluster < l.Start+idx {
			x += g.Advance
		}
	}
	return x
}

// IndexAt returns the rune index whose caret position is nearest to x
// (for click-to-position). Inverse of CaretX under the same LTR caveat.
func (l Line) IndexAt(x float32) int {
	if x <= 0 || len(l.Glyphs) == 0 {
		return 0
	}
	var pen float32
	for _, g := range l.Glyphs {
		if x < pen+g.Advance/2 {
			return g.Cluster - l.Start
		}
		pen += g.Advance
		if x < pen {
			// Past the midpoint: caret after this cluster.
			return l.nextCluster(g.Cluster) - l.Start
		}
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
