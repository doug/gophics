package ui

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// shellPage is the app's home: three sections behind a bottom tab bar.
//
// A bottom bar rather than a drawer or a top tab strip, because all three
// destinations are reachable by the thumb of the hand already holding the
// phone, and because the reading queue is the only screen anyone opens the app
// for — the other two need to be present but never in the way.
type shellPage struct{}

func (shellPage) CreateState() widget.State { return &shellState{} }

type shellState struct {
	widget.StateBase[shellPage]
	tab int
}

// tabs are the three destinations. They are labelled rather than iconified: the
// app ships no image assets, and the Go fonts have no glyph for a gear or a
// bullseye — an icon tab bar drawn from them renders as a row of tofu boxes,
// which is exactly what the first build did. Three words fit comfortably, and
// an indicator bar marks the active one.
var tabs = []string{"Read", "Sources", "Settings"}

func (s *shellState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	in := ctx.SafeInsets()

	var body widget.Widget
	switch s.tab {
	case 1:
		body = sourcesTab{}
	case 2:
		body = settingsTab{}
	default:
		body = queueTab{}
	}

	col := widget.Column(
		widget.Expand(body),
		s.tabBar(th, in.Bottom),
	)
	col.CrossAlign = layout.CrossStretch
	return widget.Decorated{Color: th.Bg, Child: col}
}

func (s *shellState) tabBar(th theme.Theme, bottomInset float32) widget.Widget {
	kids := make([]widget.Widget, 0, len(tabs))
	for i, label := range tabs {
		active := i == s.tab
		fg, font := th.Muted, ""
		indicator := paint.Color{}
		if active {
			fg, font, indicator = th.Primary, "bold", th.Primary
		}
		item := widget.Column(
			widget.Decorated{Color: indicator, Radius: 2,
				Child: widget.Sized{W: 22, H: 3}},
			widget.Sized{H: 7},
			widget.Text{S: label, Font: font, Size: th.Type.Label, Color: fg},
		)
		item.CrossAlign = layout.CrossCenter
		kids = append(kids, widget.Expand(theme.Tappable{
			OnTap: func() { s.SetState(func() { s.tab = i }) },
			Pad:   geom.InsetsSymmetric(0, 9),
			Child: widget.Center(item),
		}))
	}
	row := widget.Row(kids...)
	return colStretch(
		divider(th),
		widget.Decorated{Color: th.Bg, Child: widget.Padding{
			Insets: geom.Insets{Bottom: bottomInset},
			Child:  row,
		}},
	)
}

// tabScaffold is the page frame used inside a tab: the same header treatment as
// a pushed page, minus the bottom safe-area inset, which the tab bar owns.
func tabScaffold(ctx widget.Ctx, headerW, body widget.Widget) widget.Widget {
	th := theme.Of(ctx)
	in := ctx.SafeInsets()

	col := widget.Column(
		widget.Decorated{Color: th.Primary, Child: widget.Padding{
			Insets: geom.Insets{Top: in.Top, Left: in.Left, Right: in.Right},
			Child:  headerW,
		}},
		widget.Expand(widget.Padding{
			Insets: geom.Insets{Left: in.Left, Right: in.Right},
			Child:  widget.SelectionArea{Child: body},
		}),
	)
	col.CrossAlign = layout.CrossStretch
	content := widget.Decorated{Color: th.Bg, Child: col}

	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		const maxW = 720
		if !cs.BoundedW() || cs.Max.W <= maxW+96 {
			return content
		}
		row := widget.Row(
			widget.Expand(widget.Sized{}),
			widget.Sized{W: maxW, Child: content},
			widget.Expand(widget.Sized{}),
		)
		row.CrossAlign = layout.CrossStretch
		return widget.Decorated{Color: th.Border, Child: row}
	}}
}
