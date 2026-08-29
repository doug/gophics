// Package ui is the news reader's interface: a ranked queue of unread
// articles, a reader that renders the full article rather than a headline, and
// the screens for choosing, previewing and editing sources.
//
// The whole app is one importable package so it can be built as a desktop
// binary (main.go), a web app, or a gomobile library for Android and iOS,
// without the widget tree knowing which.
//
// Everything below the widgets lives in internal/library: what to read, what to
// fetch, what to score. Nothing here talks to the network directly.
package ui

import (
	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Env is the app's shared state, provided down the tree so pages carry only
// data. A page that holds nothing but an article ID is a plain serialisable
// value, which is what lets a hot restart put you back where you were.
type Env struct {
	Lib *library.Library
}

// current is the library Root built, so a platform host can flush state when
// the app leaves the foreground without threading a handle back through the
// bind surface.
var current *Env

// Root returns the reader over the real library.
func Root() widget.Widget {
	current = &Env{Lib: library.Open()}
	return App{Env: current}
}

// Flush persists anything held back for batching. Hosts call it when the app
// goes to the background, which on a phone is the last moment anything is
// guaranteed to run: the ranking model's writes are deferred so that scrolling
// a queue does not write a file per row.
func Flush() {
	if current != nil && current.Lib != nil {
		current.Lib.FlushRank()
	}
}

// Background is the app background before a widget context exists.
func Background() paint.Color { return theme.Light().Bg }

// App is the root widget: theme, shared state, and the navigator over the
// three-tab shell.
type App struct {
	Env *Env
}

func (a App) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Provide[*Env]{
			Value: a.Env,
			Child: widget.Fill{Color: th.Bg, Child: widget.Navigator{Home: shellPage{}}},
		},
	}
}

func init() {
	// Pushed pages are registered so the navigation stack survives a
	// state-preserving hot restart during development.
	widget.RegisterSnapshotType[readerPage]()
	widget.RegisterSnapshotType[browsePage]()
	widget.RegisterSnapshotType[addFeedPage]()
	widget.RegisterSnapshotType[feedPage]()
	widget.RegisterSnapshotType[signInPage]()
	widget.RegisterSnapshotType[whyPage]()
}

// env pulls the shared state out of context.
func env(ctx widget.Ctx) *Env { return ctx.MustOf[*Env]() }

// page is the standard screen scaffold: a colored header that extends behind
// the status bar, and a body inset by the safe areas. On a wide window it
// centres a reading column rather than stretching lines to the full width,
// because a 1600-pixel line of prose is unreadable.
func page(ctx widget.Ctx, headerW, body widget.Widget) widget.Widget {
	th := theme.Of(ctx)
	in := ctx.SafeInsets()

	col := widget.Column(
		widget.Decorated{Color: th.Primary, Child: widget.Padding{
			Insets: geom.Insets{Top: in.Top, Left: in.Left, Right: in.Right},
			Child:  headerW,
		}},
		widget.Expand(widget.Padding{
			Insets: geom.Insets{Left: in.Left, Right: in.Right, Bottom: in.Bottom},
			Child:  widget.SelectionArea{Child: body},
		}),
	)
	col.CrossAlign = layout.CrossStretch
	// An opaque background so a slide transition covers the page beneath.
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

// header is the title bar. Actions sit on the right, where a thumb reaches.
func header(th theme.Theme, title, subtitle string, lead widget.Widget, actions ...widget.Widget) widget.Widget {
	titleCol := widget.Column(
		widget.Text{S: title, Font: "bold", Size: th.Type.Heading, Color: th.OnPrimary,
			MaxLines: 1, Ellipsis: true},
	)
	titleCol.CrossAlign = layout.CrossStart
	if subtitle != "" {
		titleCol = widget.Column(
			widget.Text{S: title, Font: "bold", Size: th.Type.Heading, Color: th.OnPrimary,
				MaxLines: 1, Ellipsis: true},
			widget.Sized{H: 2},
			widget.Text{S: subtitle, Size: th.Type.Caption, Color: withAlpha(th.OnPrimary, 0.75),
				MaxLines: 1, Ellipsis: true},
		)
		titleCol.CrossAlign = layout.CrossStart
	}

	kids := []widget.Widget{}
	if lead != nil {
		kids = append(kids, lead, widget.Sized{W: 6})
	}
	kids = append(kids, widget.Expand(titleCol))
	for _, a := range actions {
		kids = append(kids, widget.Sized{W: 4}, a)
	}
	row := widget.Row(kids...)
	row.CrossAlign = layout.CrossCenter
	return widget.Padding{Insets: geom.InsetsSymmetric(12, 10), Child: row}
}

// backButton returns to the previous screen.
func backButton(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	nav := ctx.MustOf[widget.Nav]()
	return theme.Tappable{
		OnTap:  nav.Pop,
		Radius: th.Radius,
		Pad:    geom.InsetsSymmetric(8, 6),
		Child:  widget.Text{S: "‹", Font: "bold", Size: th.Type.Title, Color: th.OnPrimary},
	}
}

// headerAction is a text button in the title bar.
func headerAction(th theme.Theme, label string, onTap func()) widget.Widget {
	return theme.Tappable{
		OnTap:  onTap,
		Radius: th.Radius,
		Pad:    geom.InsetsSymmetric(10, 6),
		Child:  widget.Text{S: label, Size: th.Type.Label, Color: th.OnPrimary},
	}
}

// withAlpha fades a color, for secondary text on the header bar.
func withAlpha(c paint.Color, a float32) paint.Color {
	c.A *= a
	return c
}

// centered is the empty/loading/error state used by every screen, so they all
// fail the same way.
func centered(th theme.Theme, title, detail string) widget.Widget {
	col := widget.Column(
		widget.Text{S: title, Font: "bold", Size: th.Type.Heading, Color: th.Text, Wrap: true},
	)
	if detail != "" {
		col = widget.Column(
			widget.Text{S: title, Font: "bold", Size: th.Type.Heading, Color: th.Text, Wrap: true},
			widget.Sized{H: 8},
			widget.Text{S: detail, Size: th.Type.Body, Color: th.Muted, Wrap: true},
		)
	}
	col.CrossAlign = layout.CrossCenter
	return widget.Center(widget.Padding{All: 32, Child: widget.Sized{W: 360, Child: col}})
}

// divider is a hairline between rows.
func divider(th theme.Theme) widget.Widget {
	return widget.Decorated{Color: th.Border, Child: widget.Sized{H: 1}}
}

// chip is a small rounded label — a category filter, a source tag.
func chip(th theme.Theme, label string, selected bool, onTap func()) widget.Widget {
	fg, bg := th.Text, th.Surface
	if selected {
		fg, bg = th.OnPrimary, th.Primary
	}
	return widget.Padding{Insets: geom.Insets{Right: 8},
		Child: theme.Tappable{
			OnTap:      onTap,
			Background: bg,
			Radius:     14,
			Pad:        geom.InsetsSymmetric(12, 7),
			Child:      widget.Text{S: label, Size: th.Type.Label, Color: fg},
		},
	}
}

// chipBarHeight is the height of a horizontal strip of chips.
//
// It has to be a fixed number, and that is worth explaining. widget.Scroll
// overlays a scrollbar that lays out at cs.Constrain(cs.Max) to fill the scroll
// area (widget/basic.go, scrollbarBox.Layout). Placed in a Column, a horizontal
// Scroll gets an unbounded cross axis, so that overlay measures as infinitely
// tall — and the Scroll with it. Everything below then lays out at y=+Inf and
// simply is not drawn, while the chips above it look perfectly fine, which is a
// memorably confusing way to lose a screen. Bounding the height keeps a
// horizontal scroll usable inside a vertical stack.
const chipBarHeight = 32

// chipBar is a horizontally scrollable strip of chips.
func chipBar(kids ...widget.Widget) widget.Widget {
	return widget.Sized{H: chipBarHeight,
		Child: widget.Scroll{Axis: layout.Horizontal, Child: widget.Row(kids...)}}
}

// button is a primary action.
func button(th theme.Theme, label string, onTap func()) widget.Widget {
	return theme.Tappable{
		OnTap:      onTap,
		Background: th.Primary,
		Radius:     th.Radius,
		Pad:        geom.InsetsSymmetric(16, 10),
		Haptic:     true,
		Child:      widget.Text{S: label, Font: "bold", Size: th.Type.Label, Color: th.OnPrimary},
	}
}

// secondaryButton is a lower-emphasis action next to a primary one.
func secondaryButton(th theme.Theme, label string, onTap func()) widget.Widget {
	return theme.Tappable{
		OnTap:      onTap,
		Background: th.Surface,
		Radius:     th.Radius,
		Pad:        geom.InsetsSymmetric(16, 10),
		Child:      widget.Text{S: label, Size: th.Type.Label, Color: th.Text},
	}
}
