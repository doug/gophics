package text

import "sort"

// Caret placement and hit testing on a shaped line.
//
// Positions come from the glyphs' own X, not from summing advances. Summing
// only works while glyph order matches logical order, which bidi breaks: after
// reordering (UAX #9 rule L2) the glyphs are in *visual* order, so "every glyph
// before this cluster" is no longer a contiguous run of pixels, and the total is
// the width of a set of glyphs scattered across the line.

// clusterSpan is one cluster's rune range and visual extent on a line.
type clusterSpan struct {
	start, end  int     // rune range [start, end) in the shaped string
	left, right float32 // visual extent
	rtl         bool    // the run reads right to left
}

// clusters returns the line's clusters in logical order. A cluster's extent
// is the union of its glyphs (a base and its marks, or one ligature glyph),
// and its rune range ends where the next cluster begins — so a ligature's
// span covers every rune it fused.
func (l Line) clusters() []clusterSpan {
	spans := make([]clusterSpan, 0, len(l.Glyphs))
	at := map[int]int{} // cluster start → index in spans
	for _, g := range l.Glyphs {
		i, ok := at[g.Cluster]
		if !ok {
			i = len(spans)
			at[g.Cluster] = i
			spans = append(spans, clusterSpan{start: g.Cluster, left: g.X, right: g.X + g.Advance, rtl: g.RTL})
			continue
		}
		spans[i].left = min(spans[i].left, g.X)
		spans[i].right = max(spans[i].right, g.X+g.Advance)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	for i := range spans {
		if i+1 < len(spans) {
			spans[i].end = spans[i+1].start
		} else {
			spans[i].end = l.End
		}
	}
	return spans
}

// CaretX returns the x position of the caret placed before rune index idx.
//
// In a right-to-left run the caret before a cluster sits at that glyph's right
// edge, because "before" means earlier in reading order and reading runs the
// other way. Getting this wrong puts the caret on the far side of the character
// being typed, which is the usual symptom of an LTR-only implementation.
//
// An index inside a cluster — between the f and the i of an "fi" ligature,
// where Editor.Move lands because they are separate graphemes — has no glyph
// edge of its own. The caret is interpolated across the cluster by rune
// fraction, as browsers do, rather than jumping to the end of the line.
func (l Line) CaretX(idx int) float32 {
	return l.caretX(l.clusters(), l.Start+idx)
}

// caretX is CaretX over precomputed clusters, for a target rune index into the
// shaped string.
func (l Line) caretX(spans []clusterSpan, target int) float32 {
	for i := len(spans) - 1; i >= 0; i-- {
		c := spans[i]
		if target < c.start {
			continue
		}
		if target >= c.end {
			break // past the last cluster
		}
		frac := float32(target-c.start) / float32(c.end-c.start)
		if c.rtl {
			return c.right - frac*(c.right-c.left)
		}
		return c.left + frac*(c.right-c.left)
	}
	// Past the last cluster: the caret belongs at the line's trailing edge,
	// which is the left for an RTL line and the right for an LTR one.
	if l.RTL {
		return 0
	}
	return l.Width
}

// rtlAt reports whether the glyph at visual index i belongs to a right-to-left
// run. Shaping records it per glyph (see assemble): inferring it from the
// clusters of the visual neighbours failed for a run of one glyph — a lone
// Arabic letter in a Latin line has monotone neighbours and read as LTR, so
// its "before" and "after" caret edges were swapped.
func (l Line) rtlAt(i int) bool { return l.Glyphs[i].RTL }

// IndexAt returns the rune index whose caret position is nearest to x
// (for click-to-position): the inverse of CaretX.
//
// It is literally that — the index i in [0, Len] minimising |CaretX(i) − x| —
// because CaretX is what the caret is drawn with, and a click has to land
// where the caret will appear. Deciding per glyph which half was clicked
// gave the right answer inside a run and the wrong one at a bidi run
// boundary, where the index "after" a glyph in reading order can be drawn
// at the far end of the line: clicking the last glyph of "abc 123" on an
// RTL line returned 7, drawn at x=0. Two indices can share a position at a
// run boundary (there is no caret affinity); the later one wins, so a click
// at the end of the line lands at the end of the text.
func (l Line) IndexAt(x float32) int {
	n := l.End - l.Start
	if len(l.Glyphs) == 0 {
		return 0
	}
	spans := l.clusters()
	best, bestD := 0, float32(0)
	for i := 0; i <= n; i++ {
		d := l.caretX(spans, l.Start+i) - x
		if d < 0 {
			d = -d
		}
		if i == 0 || d <= bestD {
			best, bestD = i, d
		}
	}
	return best
}
