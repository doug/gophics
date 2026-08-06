// Package theme is gophics's default design language: a Theme value
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
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
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
	Success      paint.Color
	Warning      paint.Color
	Danger       paint.Color
	Border       paint.Color
	Selection    paint.Color
	// Chart is a categorical palette for data series and per-item accents,
	// read positionally with ChartAt so a re-theme restyles charts too.
	Chart [6]paint.Color

	Radius float32
}

// ChartAt returns the i-th categorical color, wrapping if i is out of range.
func (t Theme) ChartAt(i int) paint.Color { return t.Chart[((i%6)+6)%6] }

// Light is the default light theme — the gophics identity: warm neutrals, a
// single clay accent, and a muted categorical palette. Opinionated on purpose
// (apps look considered out of the box) and just a struct (copy + tweak to
// re-theme, or swap wholesale).
func Light() Theme {
	return Theme{
		Bg:           paint.RGB(0.980, 0.976, 0.961), // warm off-white
		Surface:      paint.RGB(1, 1, 1),
		SurfaceHover: paint.RGB(0.957, 0.949, 0.929),
		Primary:      paint.RGB(0.851, 0.467, 0.341), // clay
		OnPrimary:    paint.RGB(1, 1, 1),
		Text:         paint.RGB(0.086, 0.086, 0.078), // warm near-black
		Muted:        paint.RGB(0.427, 0.416, 0.384),
		Success:      paint.RGB(0.361, 0.545, 0.376),
		Warning:      paint.RGB(0.831, 0.639, 0.290),
		Danger:       paint.RGB(0.776, 0.361, 0.318),
		Border:       paint.RGB(0.898, 0.886, 0.859),
		Selection:    paint.Color{R: 0.851, G: 0.467, B: 0.341, A: 0.24},
		Chart:        lightChart,
		Radius:       10,
	}
}

// Dark is the default dark theme — the same identity on a warm near-black.
func Dark() Theme {
	return Theme{
		Dark:         true,
		Bg:           paint.RGB(0.086, 0.086, 0.078), // warm near-black
		Surface:      paint.RGB(0.137, 0.133, 0.125),
		SurfaceHover: paint.RGB(0.180, 0.176, 0.165),
		Primary:      paint.RGB(0.878, 0.522, 0.396), // clay, lifted for dark
		OnPrimary:    paint.RGB(0.086, 0.086, 0.078),
		Text:         paint.RGB(0.949, 0.937, 0.914), // warm off-white
		Muted:        paint.RGB(0.667, 0.647, 0.612),
		Success:      paint.RGB(0.451, 0.647, 0.451),
		Warning:      paint.RGB(0.878, 0.694, 0.361),
		Danger:       paint.RGB(0.851, 0.451, 0.408),
		Border:       paint.RGB(0.271, 0.263, 0.247),
		Selection:    paint.Color{R: 0.878, G: 0.522, B: 0.396, A: 0.30},
		Chart:        darkChart,
		Radius:       10,
	}
}

// The muted categorical palette (clay, cactus, sky, heather, kraft, fig),
// tuned per scheme for contrast against the background.
var (
	lightChart = [6]paint.Color{
		paint.RGB(0.851, 0.467, 0.341), // clay
		paint.RGB(0.400, 0.596, 0.463), // cactus
		paint.RGB(0.416, 0.608, 0.780), // sky
		paint.RGB(0.545, 0.478, 0.706), // heather
		paint.RGB(0.780, 0.545, 0.361), // kraft
		paint.RGB(0.639, 0.400, 0.439), // fig
	}
	darkChart = [6]paint.Color{
		paint.RGB(0.878, 0.522, 0.396),
		paint.RGB(0.478, 0.678, 0.545),
		paint.RGB(0.494, 0.671, 0.843),
		paint.RGB(0.627, 0.561, 0.784),
		paint.RGB(0.843, 0.616, 0.435),
		paint.RGB(0.718, 0.478, 0.518),
	}
)

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
