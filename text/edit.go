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
}

// SetText replaces the content and clamps the caret/selection.
func (e *Editor) SetText(s string) {
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
	start, end := e.Selection()
	ins := []rune(s)
	out := make([]rune, 0, len(e.runes)-(end-start)+len(ins))
	out = append(out, e.runes[:start]...)
	out = append(out, ins...)
	out = append(out, e.runes[end:]...)
	e.runes = out
	e.caret = start + len(ins)
	e.anchor = e.caret
}

// DeleteBackward deletes the selection, or the grapheme before the caret.
func (e *Editor) DeleteBackward() {
	if !e.HasSelection() {
		e.anchor = e.prevBoundary(e.caret)
	}
	e.Insert("")
}

// DeleteForward deletes the selection, or the grapheme after the caret.
func (e *Editor) DeleteForward() {
	if !e.HasSelection() {
		e.anchor = e.nextBoundary(e.caret)
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
