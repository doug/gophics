package layout

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// SelectionSink collects selectable text fragments during a paint pass and
// reports, for each, the rune range to highlight. widget.SelectionArea provides
// the concrete implementation; a TextBox registers with the one injected into
// it, so plain Text inside a SelectionArea becomes selectable.
//
// This is framework-level selection — the sink paints its own highlight and the
// area copies to the platform clipboard — so it works identically on web,
// terminal, desktop, and mobile, with no dependency on native text (the model
// Flutter uses).
type SelectionSink interface {
	// RegisterText records a fragment (its absolute origin and wrapped-line
	// model) for this frame and returns the [lo, hi) rune range to highlight in
	// it, plus the highlight color. The range is linear over the lines with a
	// virtual newline (+1) between each; hi <= lo means no highlight.
	RegisterText(f TextFragment) (lo, hi int, col paint.Color)
}

// TextFragment describes one painted, wrapped text block for selection. Origin
// is the block's absolute (surface) top-left — the box's paint origin.
type TextFragment struct {
	Origin  geom.Pt
	Lines   []string
	Font    string
	Size    float32
	LineH   float32 // baseline-to-baseline advance
	Ascent  float32 // first-line baseline offset from the top
	Descent float32
	Painter *paint.Painter
}
