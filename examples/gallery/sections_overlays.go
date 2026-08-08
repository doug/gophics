package main

import (
	"math"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Dialogs & menus ---------------------------------------------------------

// dialogsSection triggers the overlay helpers — a modal dialog and an anchored
// menu — and echoes the action they returned.
type dialogsSection struct{}

func (dialogsSection) CreateState() widget.State { return &dialogsState{} }

type dialogsState struct {
	widget.StateBase[dialogsSection]
	result string
}

func (s *dialogsState) set(r string) { s.SetState(func() { s.result = r }) }

func (s *dialogsState) showDialog(ctx widget.Ctx) {
	th := theme.Of(ctx)
	var dismiss func()
	content := widget.Column(
		widget.Text{S: "Delete file?", Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text},
		widget.Sized{H: 8},
		widget.Text{S: "This can't be undone.", Size: th.Type.Body, Color: th.Muted, Wrap: true},
		widget.Sized{H: 18},
		widget.Row(
			widget.Spacer(),
			theme.Button{Label: "Cancel", OnTap: func() { dismiss(); s.set("Cancelled") }},
			widget.Sized{W: 10},
			theme.Button{Label: "Delete", Primary: true, OnTap: func() { dismiss(); s.set("Deleted") }},
		),
	)
	content.CrossAlign = layout.CrossStart
	dismiss = theme.ShowDialog(ctx, widget.Sized{W: 260, Child: content})
}

func (s *dialogsState) showMenu(ctx widget.Ctx) {
	theme.ShowMenu(ctx, geom.Pt{X: 40, Y: 320}, []theme.MenuItem{
		{Label: "Rename", OnTap: func() { s.set("Rename") }},
		{Label: "Duplicate", OnTap: func() { s.set("Duplicate") }},
		{Label: "Move to trash", OnTap: func() { s.set("Move to trash") }},
	})
}

func (s *dialogsState) showSheet(ctx widget.Ctx) {
	th := theme.Of(ctx)
	var dismiss func()
	content := widget.Column(
		widget.Text{S: "Share to…", Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text},
		widget.Sized{H: 8},
		widget.Text{S: "A rounded surface that slides up from the bottom edge — drag it down or tap the scrim to dismiss.",
			Size: th.Type.Body, Color: th.Muted, Wrap: true},
		widget.Sized{H: 18},
		theme.Button{Label: "Done", Primary: true, OnTap: func() { dismiss(); s.set("Sheet closed") }},
	)
	content.CrossAlign = layout.CrossStart
	dismiss = theme.ShowBottomSheet(ctx, content)
}

func (s *dialogsState) showSnackbar(ctx widget.Ctx) {
	theme.ShowSnackbar(ctx, "Saved")
}

func (s *dialogsState) showSnackbarAction(ctx widget.Ctx) {
	theme.ShowSnackbar(ctx, "Message archived",
		theme.WithAction("Undo", func() { s.set("Undo archive") }))
}

func (s *dialogsState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	return sectionColumn(
		groupLabel("Dialog & menu"),
		theme.Body("A centered modal over a dimming scrim; tap the scrim or press Escape to dismiss."),
		widget.Sized{H: 10},
		widget.Wrap{Spacing: 10, RunSpacing: 10, Children: []widget.Widget{
			theme.Button{Label: "Show dialog", Primary: true, OnTap: func() { s.showDialog(ctx) }},
			theme.Button{Label: "Show menu", OnTap: func() { s.showMenu(ctx) }},
		}},

		groupLabel("Bottom sheet"),
		theme.Body("A full-width surface that slides up from the bottom edge over a scrim."),
		widget.Sized{H: 10},
		widget.Wrap{Spacing: 10, RunSpacing: 10, Children: []widget.Widget{
			theme.Button{Label: "Show sheet", OnTap: func() { s.showSheet(ctx) }},
		}},

		groupLabel("Snackbar"),
		theme.Body("A transient, non-modal toast near the bottom — optionally with an action."),
		widget.Sized{H: 10},
		widget.Wrap{Spacing: 10, RunSpacing: 10, Children: []widget.Widget{
			theme.Button{Label: "Show snackbar", OnTap: func() { s.showSnackbar(ctx) }},
			theme.Button{Label: "Snackbar + Undo", OnTap: func() { s.showSnackbarAction(ctx) }},
		}},

		widget.Sized{H: 16},
		theme.Card{Child: widget.Text{
			S:     "Last action: " + orDash(s.result),
			Size:  th.Type.Body,
			Color: th.Text,
		}},
	)
}

// --- Layout ------------------------------------------------------------------

// layoutSection demonstrates the core layout primitives with small live views.
type layoutSection struct{}

func (layoutSection) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	// Grid: nine equal-width color swatches in three columns.
	gridCells := make([]widget.Widget, 9)
	for i := range gridCells {
		gridCells[i] = widget.Sized{H: 54, Child: widget.Decorated{
			Color: th.ChartAt(i), Radius: th.Radius,
			Child: widget.Center(widget.Text{S: string(rune('1' + i)), Font: theme.FontBold, Size: th.Type.Body, Color: th.OnPrimary}),
		}}
	}
	grid := widget.Grid{Columns: 3, Spacing: 8, Children: gridCells}

	// Wrap: chips of varying widths that flow onto new runs.
	chipWords := []string{"design", "tokens", "themeable", "warm", "behavior-native", "no CSS", "one system"}
	chips := make([]widget.Widget, len(chipWords))
	for i, w := range chipWords {
		chips[i] = chip(th, w)
	}
	wrap := widget.Wrap{Spacing: 8, RunSpacing: 8, Children: chips}

	// Stack: layered content — a surface, a centered badge, a corner tag.
	stack := widget.Stack{Children: []widget.Widget{
		widget.Sized{H: 110, Child: widget.Decorated{Color: th.SurfaceHover, Radius: th.Radius, Child: widget.Fill{}}},
		widget.Sized{H: 110, Child: widget.Center(widget.Decorated{Color: th.Primary, Radius: 24,
			Child: widget.Padding{Insets: geom.InsetsSymmetric(16, 10),
				Child: widget.Text{S: "centered", Font: theme.FontBold, Size: th.Type.Body, Color: th.OnPrimary}}})},
		widget.Sized{H: 110, Child: widget.Align{X: 1, Y: 0, Child: widget.Padding{All: 8,
			Child: chip(th, "top-right")}}},
	}}

	// AspectRatio: a 16:9 box that keeps its ratio at any width.
	aspect := widget.AspectRatio{Ratio: 16.0 / 9.0, Child: widget.Decorated{Radius: th.Radius,
		Child: widget.Canvas{Draw: func(c paint.Canvas, size geom.Size) {
			c.FillRRectGradient(geom.Rect{Max: size.Pt()}, th.Radius, th.ChartAt(2), th.ChartAt(3), true)
		}},
	}}

	return sectionColumn(
		groupLabel("Grid · 3 columns"),
		grid,
		groupLabel("Wrap"),
		wrap,
		groupLabel("Stack"),
		stack,
		groupLabel("AspectRatio · 16:9"),
		aspect,
	)
}

func chip(th theme.Theme, s string) widget.Widget {
	return widget.Decorated{Color: th.Surface, Radius: 20, BorderColor: th.Border, BorderWidth: 1,
		Child: widget.Padding{Insets: geom.InsetsSymmetric(12, 7),
			Child: widget.Text{S: s, Size: th.Type.Label, Color: th.Text}}}
}

// --- Animations --------------------------------------------------------------

// animationsSection triggers the implicit-animation family on tap: a tweened
// color, a growing bar, a scale pop, and a rotation.
type animationsSection struct{}

func (animationsSection) CreateState() widget.State { return &animationsState{} }

type animationsState struct {
	widget.StateBase[animationsSection]
	colorOn bool
	wide    bool
	big     bool
	angle   float32
}

func (s *animationsState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	// AnimateColor: cross-fade a surface between two palette colors.
	col := th.ChartAt(2)
	if s.colorOn {
		col = th.ChartAt(0)
	}
	colorTile := widget.AnimateColor(col, 250*time.Millisecond, func(c paint.Color) widget.Widget {
		return animTile(th, c, "AnimateColor")
	})

	// AnimateFloat: grow a bar's width between two fractions within a track.
	const track = 280
	frac := float32(0.3)
	if s.wide {
		frac = 1.0
	}
	bar := widget.AnimateFloat(frac, 300*time.Millisecond, func(f float32) widget.Widget {
		return widget.Row(widget.Sized{W: f * track, H: 18, Child: widget.Decorated{Color: th.Primary, Radius: 9, Child: widget.Fill{}}})
	})
	barTrack := widget.Decorated{Color: th.Surface, Radius: 9, BorderColor: th.Border, BorderWidth: 1,
		Child: widget.Sized{W: track, H: 18, Child: widget.Align{X: 0, Y: 0.5, Child: bar}}}

	// AnimatedScale: pop a badge on tap.
	scale := float32(1)
	if s.big {
		scale = 1.4
	}
	pop := widget.AnimatedScale(scale, 200*time.Millisecond, animTile(th, th.ChartAt(4), "Scale"))

	// AnimatedRotation: spin a square by a quarter turn each tap.
	spin := widget.AnimatedRotation(s.angle, 300*time.Millisecond, animTile(th, th.ChartAt(5), "Rotation"))

	tap := func(child widget.Widget, onTap func()) widget.Widget {
		return widget.Interactive{Handler: widget.Handler{OnTap: onTap}, Child: child}
	}

	return sectionColumn(
		theme.Body("Set a new value and it tweens from wherever it is — no controller to manage. Tap each tile."),
		groupLabel("AnimateColor"),
		tap(colorTile, func() { s.SetState(func() { s.colorOn = !s.colorOn }) }),
		groupLabel("AnimateFloat"),
		tap(barTrack, func() { s.SetState(func() { s.wide = !s.wide }) }),
		groupLabel("AnimatedScale"),
		widget.Row(tap(pop, func() { s.SetState(func() { s.big = !s.big }) })),
		groupLabel("AnimatedRotation"),
		widget.Row(tap(spin, func() { s.SetState(func() { s.angle += math.Pi / 2 }) })),
	)
}

// animTile is a small labeled square used by the animation demos.
func animTile(th theme.Theme, col paint.Color, label string) widget.Widget {
	return widget.Decorated{Color: col, Radius: th.Radius, Child: widget.Sized{W: 96, H: 96,
		Child: widget.Center(widget.Text{S: label, Font: theme.FontBold, Size: th.Type.Label, Color: th.OnPrimary})}}
}
