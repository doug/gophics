package layout

import "github.com/doug/gophics/paint"

// RichSpan is one styled run of a rich paragraph.
type RichSpan struct {
	Text      string
	Font      string // named font family ("" = default)
	Color     paint.Color
	Underline bool
	// Link marks the span tappable; RichBox.LinkAt reports it.
	Link string
}
