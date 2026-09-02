package theme

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

// ListTile is one row of a list: an optional leading element, a title with an
// optional second line, and an optional trailing element. It is the shape most
// list rows want, so an app writes the content and not the geometry.
//
// A tile with OnTap is a button to assistive technology, labeled with its
// title and subtitle together — the two lines are read as one row, the way a
// sighted user takes them in.
type ListTile struct {
	Title    string
	Subtitle string
	// Leading and Trailing sit either side of the text, vertically centered —
	// an icon, an avatar, a checkbox, a chevron.
	Leading  widget.Widget
	Trailing widget.Widget
	OnTap    func()
	// OnLongPress, when set, gives the row a hold gesture (context actions).
	OnLongPress func()
	// Selected tints the row and reports itself to assistive technology.
	Selected bool
	// Dense reduces the vertical padding for information-heavy lists.
	Dense bool
}

func (t ListTile) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)

	pad := float32(12)
	if t.Dense {
		pad = 8
	}

	title := widget.Text{Value: t.Title, Size: th.Type.Body, Color: th.Text, MaxLines: 1, Ellipsis: true}
	var text widget.Widget = title
	if t.Subtitle != "" {
		text = widget.Column(
			title,
			widget.Sized{H: 2},
			widget.Text{Value: t.Subtitle, Size: th.Type.Caption, Color: th.Muted, MaxLines: 1, Ellipsis: true},
		)
	}

	// The text column is the only flexible part: leading and trailing keep
	// their natural size so an icon never gets squeezed by a long title.
	row := []widget.Widget{}
	if t.Leading != nil {
		row = append(row, widget.Center(t.Leading), widget.Sized{W: 12})
	}
	row = append(row, widget.Expand(widget.Align{X: 0, Y: 0.5, Child: text}))
	if t.Trailing != nil {
		row = append(row, widget.Sized{W: 12}, widget.Center(t.Trailing))
	}

	bg := th.Surface
	if t.Selected {
		bg = th.Selection
	}
	content := widget.Row(row...)

	if t.OnTap == nil && t.OnLongPress == nil {
		return widget.Decorated{Color: bg, Child: widget.Padding{
			Insets: geom.Insets{Left: 14, Right: 14, Top: pad, Bottom: pad},
			Child:  content,
		}}
	}

	tap := Tappable{
		Child:       content,
		OnTap:       t.OnTap,
		OnLongPress: t.OnLongPress,
		Background:  bg,
		Pad:         geom.Insets{Left: 14, Right: 14, Top: pad, Bottom: pad},
	}
	// One node for the whole row: a screen reader should hear "Jane Doe, two
	// unread, button", not walk into the row and find two separate texts.
	return widget.Semantics{
		Role:       layout.RoleButton,
		Label:      joinNonEmpty(t.Title, t.Subtitle),
		Selected:   t.Selected,
		OnActivate: t.OnTap,
		Child:      tap,
	}
}

func joinNonEmpty(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + ", " + b
}

// Divider is a hairline rule between rows, drawn in the theme's decorative
// border color and hidden from assistive technology (it means nothing spoken).
type Divider struct {
	// Indent insets the rule from the left, to line it up with a tile's text
	// rather than its leading icon.
	Indent float32
}

func (d Divider) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	return widget.Padding{
		Insets: geom.Insets{Left: d.Indent},
		Child:  widget.Decorated{Color: th.Border, Child: widget.Sized{H: 1}},
	}
}
