package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// newScrollbarState builds a scroll state with plausible measured geometry,
// standing in for the frame that would have painted the bar.
func scrollbarStateFor(t *testing.T, thumbLen, trackLen, offset float32) *scrollState {
	t.Helper()
	s := &scrollState{}
	s.vp = &viewportRef{}
	s.barThumbLen, s.barTrackLen, s.offset = thumbLen, trackLen, offset
	return s
}

// The thumb travels the track while the content travels MaxOffset, so a pixel
// of thumb is worth maxOffset/track pixels of content. Using the viewport size
// as the denominator — the obvious wrong choice — makes a long document crawl.
func TestScrollbarDragRatio(t *testing.T) {
	// 400pt of content in a 100pt viewport: MaxOffset 300, thumb travel 75.
	// One pixel of thumb is therefore worth four of content.
	if got := barOffsetFor(0, 10, 300, 75); got != 40 {
		t.Errorf("10pt of thumb moved content %v, want 40 (ratio 300/75)", got)
	}
	// The whole track covers the whole document.
	if got := barOffsetFor(0, 75, 300, 75); got != 300 {
		t.Errorf("dragging the full track gave %v, want MaxOffset 300", got)
	}
}

// Dragging past either end must stop there rather than detaching the content
// from the bar.
func TestScrollbarDragClampsToRange(t *testing.T) {
	if got := barOffsetFor(0, 1000, 300, 75); got != 300 {
		t.Errorf("past the end gave %v, want 300", got)
	}
	if got := barOffsetFor(150, -1000, 300, 75); got != 0 {
		t.Errorf("past the start gave %v, want 0", got)
	}
}

// Unmeasured geometry must be inert rather than dividing by zero.
func TestScrollbarDragInertWhenUnmeasured(t *testing.T) {
	if got := barOffsetFor(42, 10, 300, 0); got != 42 {
		t.Errorf("unmeasured track moved the offset to %v, want it unchanged", got)
	}
	if got := barOffsetFor(42, 10, 0, 75); got != 42 {
		t.Errorf("nothing to scroll moved the offset to %v, want it unchanged", got)
	}
}

// The target must not exist when there is nothing to drag: an invisible
// Interactive over the content would eat presses meant for what is underneath.
func TestScrollbarThumbAbsentWhenNotScrollable(t *testing.T) {
	s := scrollbarStateFor(t, 0, 0, 0)
	s.barFade = 1
	if w := scrollbarThumb(s); w != nil {
		t.Error("thumb built with no measured geometry")
	}

	s2 := scrollbarStateFor(t, 40, 200, 0)
	s2.barFade = 0 // faded out
	if w := scrollbarThumb(s2); w != nil {
		t.Error("thumb built while the bar is invisible")
	}
}

// The bar fills the scroll area so it can paint at the edge, and must stay
// transparent to input — otherwise every tap on the content hits the bar.
func TestScrollbarBoxTakesNoHits(t *testing.T) {
	b := &scrollbarBox{size: geom.Size{W: 640, H: 480}}
	var hits []layout.Hit
	b.AddHits(geom.Pt{X: 10, Y: 10}, &hits)
	if len(hits) != 0 {
		t.Errorf("the decorative bar claimed %d hits", len(hits))
	}
}
