package widget

import (
	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Text displays text: single-line by default, word-wrapped when Wrap is
// set, with optional decorations.
type Text struct {
	S         string
	Font      string  // named font family ("" = default; e.g. "bold")
	Size      float32 // 0 → 14
	Color     paint.Color
	Wrap      bool
	Strike    bool
	Underline bool
	// MaxLines caps wrapped lines (0 = unlimited); Ellipsis truncates
	// overflow with "…" (single line to width, or wrapped at MaxLines).
	MaxLines int
	Ellipsis bool
}

func (t Text) size() float32 {
	if t.Size == 0 {
		return 14
	}
	return t.Size
}

func (t Text) createBox(ctx Ctx) layout.Box { return &layoutbox.TextBox{Painter: ctx.Painter()} }
func (t Text) updateBox(ctx Ctx, b layout.Box) {
	tb := b.(*layoutbox.TextBox)
	tb.Text, tb.Font, tb.TextSize, tb.Color = t.S, t.Font, t.size(), t.Color
	tb.Wrap, tb.Strike, tb.Underline = t.Wrap, t.Strike, t.Underline
	tb.MaxLines, tb.Ellipsis = t.MaxLines, t.Ellipsis
	// Inside a SelectionArea, register as a selectable fragment.
	if reg, ok := ctx.Of[*selectionRegistry](); ok {
		tb.Selection = reg
	} else {
		tb.Selection = nil
	}
}
func (t Text) childWidgets() []Widget          { return nil }
func (t Text) attach(layout.Box, []layout.Box) {}
