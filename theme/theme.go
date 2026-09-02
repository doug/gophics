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
//
// # Callback naming
//
// Controls follow one naming convention:
//
//   - OnChange(value) — a controlled value changed. The control renders the
//     value the caller passed in and requests a new one through OnChange
//     (Switch, Checkbox, Slider, Segmented, Tabs, Dropdown).
//   - OnTap / OnSelect — an event on a specific item, not a value change on
//     the control itself (Tappable.OnTap; Radio.OnSelect, which fires per
//     option — the group's value lives with the caller, not the Radio).
//   - OnPick — a picker's terminal choice (DatePicker, TimePicker): the user
//     committed a selection, typically dismissing the picker.
package theme

import (
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// haptic plays a feedback event when the platform supports it (nil elsewhere),
// so themed controls can add tactile feedback without every call site guarding.
func haptic(ctx widget.Ctx, k shell.HapticKind) {
	if h := ctx.Haptic(); h != nil {
		h.Play(k)
	}
}

// FontBold is the font family name themed components use for emphasis;
// register it via app.Config.FontFamilies.
const FontBold = "bold"

// Theme is the design-token set consumed by themed components.
type Theme struct {
	Dark bool

	Bg           paint.Color // app background
	Surface      paint.Color // cards, fields — inline, on the app's own background
	SurfaceHover paint.Color
	// Elevated is the surface of something floating *over* the app: a dialog, a
	// menu, a dropdown list, a sheet. It has to stay readable above arbitrary
	// content, which the inline Surface does not — on the glass themes Surface is
	// around half transparent, which is right for a panel resting on a known
	// background and wrong for a dialog with a page showing through it.
	Elevated  paint.Color
	Primary   paint.Color // accent / actions
	OnPrimary paint.Color // content on Primary
	Text      paint.Color
	Muted     paint.Color
	Success   paint.Color
	Warning   paint.Color
	Danger    paint.Color
	// Border is decorative chrome: table rules, dividers, the hairline edge of a
	// panel. It is allowed to be very faint — on the glass themes it is a barely
	// there rim highlight.
	Border paint.Color
	// Outline is the edge of an *interactive* control: an unchecked checkbox or
	// radio, a switch or slider track, a field or button border. It must stay
	// legible on every background, because it is the only thing telling the user
	// the control exists. Keeping it apart from Border is what stops a theme
	// tuned for pretty panel edges from erasing its own form controls — which is
	// exactly what the glass themes did.
	Outline   paint.Color
	Selection paint.Color
	// Chart is a categorical palette for data series and per-item accents,
	// read positionally with ChartAt so a re-theme restyles charts too.
	Chart [6]paint.Color

	Radius float32
	Type   TypeScale // named text sizes
	// Blur, when > 0, is the backdrop-blur radius themed surfaces use for a
	// frosted-glass material (the Glass presets set it and make Surface
	// translucent). Zero = opaque surfaces.
	Blur float32
}

// TypeScale is the theme's named text sizes, in logical px — the ramp app text
// should lean on (and that the themed text helpers use). Special one-off sizes
// (a giant hero number) can still be set explicitly; the scale is the default.
type TypeScale struct {
	Display float32 // hero numbers / splash
	Title   float32 // page titles
	Heading float32 // section / card headings
	Body    float32 // default reading size
	Label   float32 // controls, chips, dense captions
	Caption float32 // fine print, timestamps
}

func defaultType() TypeScale {
	return TypeScale{Display: 32, Title: 22, Heading: 17, Body: 15, Label: 13, Caption: 11}
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
		Elevated:     paint.RGB(1, 1, 1),
		SurfaceHover: paint.RGB(0.957, 0.949, 0.929),
		Primary:      paint.RGB(0.851, 0.467, 0.341), // clay
		OnPrimary:    paint.RGB(1, 1, 1),
		Text:         paint.RGB(0.086, 0.086, 0.078), // warm near-black
		Muted:        paint.RGB(0.427, 0.416, 0.384),
		Success:      paint.RGB(0.361, 0.545, 0.376),
		Warning:      paint.RGB(0.831, 0.639, 0.290),
		Danger:       paint.RGB(0.776, 0.361, 0.318),
		Border:       paint.RGB(0.898, 0.886, 0.859),
		Outline:      paint.RGB(0.706, 0.690, 0.655),
		Selection:    paint.Color{R: 0.851, G: 0.467, B: 0.341, A: 0.24},
		Chart:        lightChart,
		Radius:       10,
		Type:         defaultType(),
	}
}

// Dark is the default dark theme — the same identity on a warm near-black.
func Dark() Theme {
	return Theme{
		Dark:         true,
		Bg:           paint.RGB(0.086, 0.086, 0.078), // warm near-black
		Surface:      paint.RGB(0.137, 0.133, 0.125),
		Elevated:     paint.RGB(0.169, 0.165, 0.157),
		SurfaceHover: paint.RGB(0.180, 0.176, 0.165),
		Primary:      paint.RGB(0.878, 0.522, 0.396), // clay, lifted for dark
		OnPrimary:    paint.RGB(0.086, 0.086, 0.078),
		Text:         paint.RGB(0.949, 0.937, 0.914), // warm off-white
		Muted:        paint.RGB(0.667, 0.647, 0.612),
		Success:      paint.RGB(0.451, 0.647, 0.451),
		Warning:      paint.RGB(0.878, 0.694, 0.361),
		Danger:       paint.RGB(0.851, 0.451, 0.408),
		Border:       paint.RGB(0.271, 0.263, 0.247),
		Outline:      paint.RGB(0.451, 0.439, 0.416),
		Selection:    paint.Color{R: 0.878, G: 0.522, B: 0.396, A: 0.30},
		Chart:        darkChart,
		Radius:       10,
		Type:         defaultType(),
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

// Glass is the identity as a frosted-glass material: translucent light surfaces
// over a backdrop blur, for UIs layered on a photo or gradient. It derives from
// Light (same accent, type scale, chart palette) — only the material changes.
// The blur is real on both the CPU and GPU paths (see paint.Canvas.BackdropBlur).
func Glass() Theme {
	t := Light()
	t.Surface = paint.Color{R: 1, G: 1, B: 1, A: 0.55}
	t.SurfaceHover = paint.Color{R: 1, G: 1, B: 1, A: 0.72}
	t.Elevated = paint.Color{R: 1, G: 1, B: 1, A: 0.92}
	t.Border = paint.Color{R: 1, G: 1, B: 1, A: 0.55}
	// A control's edge cannot borrow the panel rim: over a blurred, arbitrary
	// backdrop it has to carry its own contrast.
	t.Outline = paint.Color{R: 0.20, G: 0.19, B: 0.18, A: 0.55}
	t.Blur = 24
	return t
}

// GlassDark is Glass over a dark backdrop.
func GlassDark() Theme {
	t := Dark()
	t.Surface = paint.Color{R: 0.13, G: 0.13, B: 0.12, A: 0.48}
	t.SurfaceHover = paint.Color{R: 0.22, G: 0.22, B: 0.21, A: 0.6}
	t.Elevated = paint.Color{R: 0.16, G: 0.16, B: 0.15, A: 0.94}
	t.Border = paint.Color{R: 1, G: 1, B: 1, A: 0.16}
	t.Outline = paint.Color{R: 1, G: 1, B: 1, A: 0.55}
	t.Blur = 24
	return t
}

// Of returns the provided Theme, falling back to Auto when none is
// provided — themed components work without any setup.
func Of(ctx widget.Ctx) Theme {
	if th, ok := ctx.Of[Theme](); ok {
		return th
	}
	return Auto(ctx)
}

// Display, Title, Heading, Body, Label, and Caption return themed text at the
// matching TypeScale size and semantic color, tracking the active theme so an
// app never hardcodes a size or a text color for standard copy.
func Display(s string) widget.Widget { return themedText{S: s, Role: roleDisplay} }
func Title(s string) widget.Widget   { return themedText{S: s, Role: roleTitle} }
func Heading(s string) widget.Widget { return themedText{S: s, Role: roleHeading} }
func Body(s string) widget.Widget    { return themedText{S: s, Role: roleBody, Wrap: true} }
func Label(s string) widget.Widget   { return themedText{S: s, Role: roleLabel} }
func Caption(s string) widget.Widget { return themedText{S: s, Role: roleCaption} }

type textRole uint8

const (
	roleDisplay textRole = iota
	roleTitle
	roleHeading
	roleBody
	roleLabel
	roleCaption
)

type themedText struct {
	S    string
	Wrap bool
	Role textRole
}

func (t themedText) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	size, col, bold := th.Type.Body, th.Text, false
	switch t.Role {
	case roleDisplay:
		size, bold = th.Type.Display, true
	case roleTitle:
		size, bold = th.Type.Title, true
	case roleHeading:
		size, bold = th.Type.Heading, true
	case roleLabel:
		size, col = th.Type.Label, th.Muted
	case roleCaption:
		size, col = th.Type.Caption, th.Muted
	}
	font := ""
	if bold {
		font = FontBold
	}
	return widget.Text{Value: t.S, Font: font, Size: size, Color: col, Wrap: t.Wrap}
}

// Card wraps content in a themed surface.
type Card struct {
	Child widget.Widget
	Pad   float32 // 0 → 12
	// Solid draws the card on the near-opaque Elevated surface with no
	// backdrop blur.
	//
	// A backdrop blur costs whatever is behind it, drawn again, so a frosted
	// card over expensive content pays for that content twice. Over a page of
	// charts that is the difference between a 4ms frame and a 16ms one, with
	// the worst frames near 43ms — visible as stutter while scrolling. Set
	// this for cards whose contents need a steady background anyway: a chart's
	// grid lines and axis labels are easier to read over one, and at 0.92
	// alpha the surface still belongs to the same family.
	Solid bool
}

func (c Card) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	pad := c.Pad
	if pad == 0 {
		pad = 12
	}
	color, blur := th.Surface, th.Blur
	if c.Solid {
		color, blur = th.Elevated, 0
	}
	return widget.Decorated{
		Color: color, Radius: th.Radius, Blur: blur,
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
	ctx     widget.Ctx
	hovered bool
	// press drives the pressed-down highlight: it jumps to full on pointer-down
	// and eases back to 0 on release, so a tap flashes even when press and
	// release land in the same frame. This is the touch-native feedback hover
	// can't give (touch has no hover).
	press *anim.Controller
}

func (s *buttonState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.press = &anim.Controller{
		Duration: 160 * time.Millisecond, Curve: anim.EaseOut,
		OnChange: func() { s.SetState(func() {}) },
	}
	ctx.AddTicker(s.press)
}

func (s *buttonState) Dispose() { s.ctx.RemoveTicker(s.press) }

// pressIn lights the highlight fully on pointer-down.
func (s *buttonState) pressIn() { s.press.Jump(1) }

// pressOut releases it: an eased fade, or instant when reduce-motion is set.
func (s *buttonState) pressOut() {
	if s.ctx.ReduceMotion() {
		s.press.Jump(0)
		return
	}
	s.press.Reverse()
	s.ctx.Invalidate() // kick the frame loop so the fade advances
}

func (s *buttonState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	b := s.W()
	bg, fg, border := th.Surface, th.Text, th.Outline
	if b.Primary {
		bg, fg, border = th.Primary, th.OnPrimary, th.Primary
	}
	if s.hovered {
		bg = paint.Lerp(bg, th.Text, 0.08)
	}
	// Pressed highlight sits on top of hover: a firmer darken toward the text
	// color, scaled by the press animation (full on down, fading on release).
	if p := s.press.Value(); p > 0 {
		bg = paint.Lerp(bg, th.Text, 0.16*p)
	}
	return widget.Interactive{
		Gestures: widget.Gestures{
			OnTap:      b.OnTap,
			OnEnter:    func() { s.SetState(func() { s.hovered = true }) },
			OnExit:     func() { s.SetState(func() { s.hovered = false }) },
			OnPress:    func(geom.Pt) { s.pressIn() },
			OnPressEnd: s.pressOut,
		},
		Child: widget.Decorated{
			Color: bg, Radius: th.Radius, BorderColor: border, BorderWidth: 1,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(14, 8),
				Child:  widget.Text{Value: b.Label, Font: FontBold, Size: 14, Color: fg},
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
	// Autofocus takes keyboard focus when the field mounts. See
	// widget.Interactive.Autofocus.
	Autofocus bool
	// OnFocus reports focus gained and lost. Losing it is how a field that
	// appeared for one edit knows to put itself away.
	OnFocus func(bool)
}

func (f Field) CreateState() widget.State { return &fieldState{} }

type fieldState struct {
	widget.StateBase[Field]
	focused bool
}

func (s *fieldState) Build(ctx widget.Ctx) widget.Widget {
	th := Of(ctx)
	f := s.W()
	border, bw := th.Outline, float32(1)
	if s.focused {
		border, bw = th.Primary, 1.5
	}
	return widget.Decorated{
		Color: th.Surface, Radius: th.Radius, BorderColor: border, BorderWidth: bw,
		Child: widget.Padding{
			Insets: geom.InsetsSymmetric(12, 10),
			Child: widget.TextField{
				Autofocus:   f.Autofocus,
				Value:       f.Value,
				Placeholder: f.Placeholder,
				Multiline:   f.Multiline,
				OnChange:    f.OnChange,
				OnSubmit:    f.OnSubmit,
				OnFocus: func(v bool) {
					s.SetState(func() { s.focused = v })
					if f.OnFocus != nil {
						f.OnFocus(v)
					}
				},
				TextColor:        th.Text,
				PlaceholderColor: th.Muted,
				CaretColor:       th.Primary,
				SelectionColor:   th.Selection,
			},
		},
	}
}
