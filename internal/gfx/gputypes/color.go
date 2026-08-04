package gputypes

// Color represents an RGBA color with float64 components.
//
// Each component is typically in the range [0.0, 1.0] for standard colors,
// though HDR colors may use values outside this range.
type Color struct {
	// R is the red component.
	R float64
	// G is the green component.
	G float64
	// B is the blue component.
	B float64
	// A is the alpha (opacity) component.
	A float64
}

// NewColor creates a new Color with the given RGBA values.
func NewColor(r, g, b, a float64) Color {
	return Color{R: r, G: g, B: b, A: a}
}

// NewColorRGB creates a new opaque Color with the given RGB values and alpha=1.0.
func NewColorRGB(r, g, b float64) Color {
	return Color{R: r, G: g, B: b, A: 1.0}
}

// Predefined colors for common use cases.
var (
	// ColorTransparent is fully transparent black.
	ColorTransparent = Color{R: 0, G: 0, B: 0, A: 0}
	// ColorBlack is opaque black.
	ColorBlack = Color{R: 0, G: 0, B: 0, A: 1}
	// ColorWhite is opaque white.
	ColorWhite = Color{R: 1, G: 1, B: 1, A: 1}
	// ColorRed is opaque red.
	ColorRed = Color{R: 1, G: 0, B: 0, A: 1}
	// ColorGreen is opaque green.
	ColorGreen = Color{R: 0, G: 1, B: 0, A: 1}
	// ColorBlue is opaque blue.
	ColorBlue = Color{R: 0, G: 0, B: 1, A: 1}
	// ColorYellow is opaque yellow.
	ColorYellow = Color{R: 1, G: 1, B: 0, A: 1}
	// ColorCyan is opaque cyan.
	ColorCyan = Color{R: 0, G: 1, B: 1, A: 1}
	// ColorMagenta is opaque magenta.
	ColorMagenta = Color{R: 1, G: 0, B: 1, A: 1}
	// ColorGray is opaque 50% gray.
	ColorGray = Color{R: 0.5, G: 0.5, B: 0.5, A: 1}
)

// WithAlpha returns a copy of the color with a new alpha value.
func (c Color) WithAlpha(a float64) Color {
	return Color{R: c.R, G: c.G, B: c.B, A: a}
}

// RGBA returns the color components as individual values.
func (c Color) RGBA() (r, g, b, a float64) {
	return c.R, c.G, c.B, c.A
}

// Premultiplied returns the color with RGB components multiplied by alpha.
func (c Color) Premultiplied() Color {
	return Color{
		R: c.R * c.A,
		G: c.G * c.A,
		B: c.B * c.A,
		A: c.A,
	}
}
