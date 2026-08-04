package theme

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// ShowDialog presents content centered over a dimming scrim, above the whole
// app. Tapping the scrim or pressing Escape dismisses it; the returned
// dismiss func closes it programmatically. Requires an OverlayHost in scope
// (app.NewCore installs one).
func ShowDialog(ctx widget.Ctx, content widget.Widget) (dismiss func()) {
	ov := widget.MustOf[widget.Overlay](ctx)
	th := Of(ctx)
	var tok widget.OverlayToken
	closed := false
	close := func() {
		if !closed {
			closed = true
			tok.Dismiss()
		}
	}
	// Overlay entries sit above the app's providers, so re-provide the
	// theme captured here for the themed widgets inside the dialog.
	tok = ov.Show(widget.Provide[Theme]{Value: th, Child: modalScrim{
		OnDismiss: close,
		Child: widget.Center(widget.Decorated{
			Color: th.Surface, Radius: th.Radius,
			Child: widget.Padding{All: 20, Child: content},
		}),
	}})
	return close
}

// ShowMenu presents items in a card anchored at topLeft (logical coords),
// above the app. Selecting an item or tapping outside dismisses it.
func ShowMenu(ctx widget.Ctx, topLeft geom.Pt, items []MenuItem) (dismiss func()) {
	ov := widget.MustOf[widget.Overlay](ctx)
	th := Of(ctx)
	var tok widget.OverlayToken
	closed := false
	close := func() {
		if !closed {
			closed = true
			tok.Dismiss()
		}
	}
	rows := make([]widget.Widget, len(items))
	for i, it := range items {
		it := it
		rows[i] = widget.Interactive{
			Handler: widget.Handler{OnTap: func() {
				close()
				if it.OnTap != nil {
					it.OnTap()
				}
			}},
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(14, 10),
				Child:  widget.Row(widget.Text{S: it.Label, Size: 14, Color: th.Text}, widget.Spacer()),
			},
		}
	}
	col := widget.Column(rows...)
	col.CrossAlign = 3 // CrossStretch
	menu := widget.Decorated{
		Color: th.Surface, Radius: th.Radius, BorderColor: th.Border, BorderWidth: 1,
		Child: widget.Sized{W: 200, Child: col},
	}
	tok = ov.Show(widget.Provide[Theme]{Value: th, Child: modalScrim{
		OnDismiss: close,
		// Anchor the menu at topLeft via padding from the top-left.
		Child: widget.Padding{
			Insets: geom.Insets{Left: topLeft.X, Top: topLeft.Y},
			Child:  widget.Align{X: 0, Y: 0, Child: menu},
		},
	}})
	return close
}

// MenuItem is one selectable menu row.
type MenuItem struct {
	Label string
	OnTap func()
}

// modalScrim is a full-bleed dismiss layer with Child painted on top.
// Tapping the scrim (outside Child) or pressing Escape dismisses.
type modalScrim struct {
	OnDismiss func()
	Child     widget.Widget
}

func (m modalScrim) Build(widget.Ctx) widget.Widget {
	scrim := widget.Interactive{
		Handler: widget.Handler{
			OnTap: m.OnDismiss,
			OnKey: func(k shell.Key) {
				if k.Kind == shell.KeyPress && k.Code == shell.KeyEscape {
					m.OnDismiss()
				}
			},
		},
		Child: widget.Fill{Color: paint.Color{A: 0.45}},
	}
	// Scrim below, content above; content taps don't reach the scrim.
	return widget.Stack{Children: []widget.Widget{scrim, m.Child}}
}
