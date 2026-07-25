// Package theme is gossamer's default design language: a Theme value
// provided to the tree plus components pre-wired to consult it. The core
// widget package stays unstyled primitives; this package is the styled
// layer — the widgets/material split as two ordinary Go packages.
//
//	widget.Provide[theme.Theme]{Value: theme.Auto(ctx), Child: app}
//	...
//	th := theme.Of(ctx)
//	theme.Button{Label: "Save", OnTap: save}
//
// Themes are plain structs: customize by struct literal, derive by copy.
package theme

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// FontBold is the font family name themed components use for emphasis;
// register it via app.Config.FontFamilies.
const FontBold = "bold"

// Theme is the design-token set consumed by themed components.
type Theme struct {
	Dark bool

	Bg           paint.Color // app background
	Surface      paint.Color // cards, fields
	SurfaceHover paint.Color
	Primary      paint.Color // accent / actions
	OnPrimary    paint.Color // content on Primary
	Text         paint.Color
	Muted        paint.Color
	Danger       paint.Color
	Border       paint.Color
	Selection    paint.Color

	Radius float32
}

// Light is the default light theme.
func Light() Theme {
	return Theme{
		Bg:           paint.RGB(0.97, 0.97, 0.96),
		Surface:      paint.RGB(1, 1, 1),
		SurfaceHover: paint.RGB(0.94, 0.94, 0.95),
		Primary:      paint.RGB(0.13, 0.45, 0.90),
		OnPrimary:    paint.RGB(1, 1, 1),
		Text:         paint.RGB(0.10, 0.10, 0.12),
		Muted:        paint.RGB(0.45, 0.46, 0.50),
		Danger:       paint.RGB(0.80, 0.25, 0.25),
		Border:       paint.RGB(0.85, 0.85, 0.87),
		Selection:    paint.Color{R: 0.13, G: 0.45, B: 0.90, A: 0.30},
		Radius:       8,
	}
}

// Dark is the default dark theme.
func Dark() Theme {
	return Theme{
		Dark:         true,
		Bg:           paint.RGB(0.09, 0.10, 0.12),
		Surface:      paint.RGB(0.14, 0.15, 0.18),
		SurfaceHover: paint.RGB(0.18, 0.20, 0.24),
		Primary:      paint.RGB(0.40, 0.64, 0.98),
		OnPrimary:    paint.RGB(0.05, 0.08, 0.12),
		Text:         paint.RGB(0.92, 0.93, 0.95),
		Muted:        paint.RGB(0.55, 0.57, 0.62),
		Danger:       paint.RGB(0.92, 0.45, 0.45),
		Border:       paint.RGB(0.26, 0.28, 0.33),
		Selection:    paint.Color{R: 0.40, G: 0.64, B: 0.98, A: 0.35},
		Radius:       8,
	}
}

// Auto picks Light or Dark from the platform color scheme.
func Auto(ctx widget.Ctx) Theme {
	if ctx.DarkMode() {
		return Dark()
	}
	return Light()
}

// Of returns the provided Theme, falling back to Auto when none is
// provided — themed components work without any setup.
func Of(ctx widget.Ctx) Theme {
	if th, ok := widget.Of[Theme](ctx); ok {
		return th
	}
	return Auto(ctx)
}

// Title returns bold title text.
func Title(s string) widget.Widget {
	return themedText{S: s, Size: 17, Bold: true, Role: roleText}
}

// Body returns body text.
func Body(s string) widget.Widget {
	return themedText{S: s, Size: 14, Wrap: true, Role: roleText}
}

// Caption returns small muted text.
func Caption(s string) widget.Widget {
	return themedText{S: s, Size: 12, Role: roleMuted}
}

type textRole uint8

const (
	roleText textRole = iota
	roleMuted
)

type themedText struct {
	S    string
	Size float32
	Bold bool
	Wrap bool
	Role textRole
}

func (t themedText) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	col := th.Text
	if t.Role == roleMuted {
		col = th.Muted
	}
	font := ""
	if t.Bold {
		font = FontBold
	}
	return widget.Text{S: t.S, Font: font, Size: t.Size, Color: col, Wrap: t.Wrap}
}

// Card wraps content in a themed surface.
type Card struct {
	Child widget.Widget
	Pad   float32 // 0 → 12
}

func (c Card) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	pad := c.Pad
	if pad == 0 {
		pad = 12
	}
	return widget.Decorated{
		Color: th.Surface, Radius: th.Radius,
		Child: widget.Padding{All: pad, Child: c.Child},
	}
}

// Button is a themed tap target with hover feedback.
type Button struct {
	Label   string
	OnTap   func()
	Primary bool // filled accent style; default is a bordered surface
}

func (b Button) CreateState() widget.State { return &buttonState{} }

type buttonState struct {
	widget.StateBase[Button]
	hovered bool
}

func (s *buttonState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	b := s.W()
	bg, fg, border := th.Surface, th.Text, th.Border
	if b.Primary {
		bg, fg, border = th.Primary, th.OnPrimary, th.Primary
	}
	if s.hovered {
		bg = paint.Lerp(bg, th.Text, 0.08)
	}
	return widget.Interactive{
		Handler: widget.Handler{
			OnTap:   b.OnTap,
			OnEnter: func() { s.SetState(func() { s.hovered = true }) },
			OnExit:  func() { s.SetState(func() { s.hovered = false }) },
		},
		Child: widget.Decorated{
			Color: bg, Radius: th.Radius, BorderColor: border, BorderWidth: 1,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(14, 8),
				Child:  widget.Text{S: b.Label, Font: FontBold, Size: 14, Color: fg},
			},
		},
	}
}

// Field is a themed text input: surface chrome, focus-accented border.
type Field struct {
	Value       string
	Placeholder string
	Multiline   bool
	OnChange    func(string)
	OnSubmit    func(string)
}

func (f Field) CreateState() widget.State { return &fieldState{} }

type fieldState struct {
	widget.StateBase[Field]
	focused bool
}

func (s *fieldState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	f := s.W()
	border, bw := th.Border, float32(1)
	if s.focused {
		border, bw = th.Primary, 1.5
	}
	return widget.Decorated{
		Color: th.Surface, Radius: th.Radius, BorderColor: border, BorderWidth: bw,
		Child: widget.Padding{
			Insets: geom.InsetsSymmetric(12, 10),
			Child: widget.TextField{
				Value:            f.Value,
				Placeholder:      f.Placeholder,
				Multiline:        f.Multiline,
				OnChange:         f.OnChange,
				OnSubmit:         f.OnSubmit,
				OnFocus:          func(v bool) { s.SetState(func() { s.focused = v }) },
				TextColor:        th.Text,
				PlaceholderColor: th.Muted,
				CaretColor:       th.Primary,
				SelectionColor:   th.Selection,
			},
		},
	}
}
