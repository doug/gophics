package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
)

// The text edit menu — Cut, Copy, Paste, Select All — raised by a long press.
//
// This exists because gophics draws its own editor, and a drawn editor gets
// none of the platform's text affordances for free. Copy was bound to Cmd+C
// and nothing else, which is fine on a desktop and means that on a phone, where
// there is no Command key, text could be selected and never copied. The
// clipboard reached the system pasteboard and no gesture could reach the
// clipboard.
//
// It is drawn here rather than asked of the platform because the platform will
// not offer one: iOS raises its callout bar for a view conforming to
// UITextInput, and Android's floating toolbar comes from a TextView's
// ActionMode. A view that owns its pixels has to draw the menu as well — which
// is the same conclusion Flutter reached, and why its selection toolbar is
// Dart rather than native.
//
// What that costs is the system's extra items: Look Up, Translate, Share, and
// the text classification that turns a phone number into "Call". Those arrive
// only with the native conformance; see docs/design-capabilities.md.

// EditAction is one entry in the edit menu.
type EditAction struct {
	Label string
	OnTap func()
}

// editMenuMetrics are the bar's fixed dimensions. The menu is one line of short
// words, so hard numbers read more clearly here than a scale would.
const (
	editMenuHeight    = 40
	editMenuPadX      = 14
	editMenuGapAbove  = 12 // clearance between the bar and the touch point
	editMenuTextSize  = 14
	editMenuRadius    = 9
	editMenuHalfGuess = 70 // half a typical bar, for centring; see ShowEditMenu
)

// ShowEditMenu presents actions in a bar above at (global logical coordinates),
// and returns a function that dismisses it. Selecting an action or tapping
// outside dismisses it too.
//
// The bar is centred on at by an estimate rather than by measurement: nothing
// in the widget layer can measure a subtree before placing it, and the overlay
// positions by padding from the top-left. The estimate is clamped so the bar
// never leaves the left edge or rides under the status bar. A long menu near
// the right edge can still overhang, which is the known limit of anchoring
// this way.
func ShowEditMenu(ctx Ctx, at geom.Pt, actions []EditAction) (dismiss func()) {
	if len(actions) == 0 {
		return func() {}
	}
	ov, ok := Of[Overlay](ctx)
	if !ok {
		return func() {} // no OverlayHost above us; nothing to show it in
	}

	var tok OverlayToken
	closed := false
	closeFn := func() {
		if !closed {
			closed = true
			tok.Dismiss()
		}
	}

	dark := ctx.DarkMode()
	bg := paint.RGB(0.98, 0.98, 0.99)
	fg := paint.RGB(0.09, 0.10, 0.12)
	sep := paint.RGB(0.85, 0.86, 0.88)
	if dark {
		bg = paint.RGB(0.16, 0.17, 0.20)
		fg = paint.RGB(0.96, 0.97, 0.98)
		sep = paint.RGB(0.30, 0.31, 0.34)
	}

	kids := make([]Widget, 0, len(actions)*2-1)
	for i, a := range actions {
		if i > 0 {
			kids = append(kids, Decorated{Color: sep, Child: Sized{W: 1, H: editMenuHeight - 16}})
		}
		kids = append(kids, Interactive{
			Handler: Handler{OnTap: func() {
				closeFn()
				if a.OnTap != nil {
					a.OnTap()
				}
			}},
			Child: Padding{
				Insets: geom.InsetsSymmetric(editMenuPadX, 0),
				Child: Center(Text{
					S: a.Label, Size: editMenuTextSize, Color: fg,
				}),
			},
		})
	}

	row := Row(kids...)
	row.CrossAlign = 1 // centre the separators and labels on the bar's axis
	bar := Decorated{
		Color: bg, Radius: editMenuRadius,
		BorderColor: sep, BorderWidth: 1,
		Child: Sized{H: editMenuHeight, Child: row},
	}

	// Above the touch point, so a finger does not cover the menu it just
	// raised — the same reason both platforms open theirs upward.
	x := at.X - editMenuHalfGuess
	if x < 8 {
		x = 8
	}
	y := at.Y - editMenuHeight - editMenuGapAbove
	if min := ctx.SafeInsets().Top + 8; y < min {
		// No room above (a selection near the top): drop below the touch point
		// rather than under the status bar.
		y = at.Y + editMenuGapAbove
	}

	tok = ov.Show(editMenuScrim{
		OnDismiss: closeFn,
		Child: Padding{
			Insets: geom.Insets{Left: x, Top: y},
			Child:  Align{X: 0, Y: 0, Child: bar},
		},
	})
	return closeFn
}

// editMenuScrim catches the tap that dismisses the menu without dimming the
// app. A selection is still visible behind it and dimming the text you are
// about to copy would be perverse.
type editMenuScrim struct {
	OnDismiss func()
	Child     Widget
}

func (m editMenuScrim) Build(Ctx) Widget {
	return Stack{Children: []Widget{
		Interactive{
			Handler: Handler{
				OnTap: m.OnDismiss,
				OnKey: func(k shell.Key) {
					if k.Kind == shell.KeyPress && k.Code == shell.KeyEscape {
						m.OnDismiss()
					}
				},
			},
			Child: Fill{}, // transparent, but full-bleed so the tap lands anywhere
		},
		m.Child,
	}}
}

// editActionsFor builds the standard menu for a selection.
//
// The set is context-dependent the way both platforms do it: no Cut or Copy
// without a selection, no Paste without a clipboard, and no Select All when
// everything is already selected. An action that cannot do anything is left
// out rather than shown disabled — a four-item bar where two do nothing is
// worse than a two-item bar.
func editActionsFor(ctx Ctx, sel selectionOps) []EditAction {
	var out []EditAction
	cb := ctx.Clipboard()
	hasSel := sel.HasSelection()

	if hasSel && sel.Cut != nil {
		out = append(out, EditAction{Label: "Cut", OnTap: sel.Cut})
	}
	if hasSel && sel.Copy != nil {
		out = append(out, EditAction{Label: "Copy", OnTap: sel.Copy})
	}
	if cb != nil && sel.Paste != nil {
		if t, err := cb.ClipboardRead(); err == nil && t != "" {
			out = append(out, EditAction{Label: "Paste", OnTap: sel.Paste})
		}
	}
	if sel.SelectAll != nil && !sel.AllSelected() {
		out = append(out, EditAction{Label: "Select All", OnTap: sel.SelectAll})
	}
	return out
}

// selectionOps is what a widget offers the edit menu. A nil action is one the
// widget does not support — a read-only selection has no Cut or Paste.
type selectionOps struct {
	HasSelection func() bool
	AllSelected  func() bool
	Cut          func()
	Copy         func()
	Paste        func()
	SelectAll    func()
}
