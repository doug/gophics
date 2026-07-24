// Package paint provides gossamer's drawing layer.
//
// M0/M1-provisional: Canvas is a concrete type wrapping gogpu/gg's CPU
// rasterizer, flushed to the frame's GPU surface. Per PLAN.md §5, this
// becomes an interface with pluggable backends (gg stays one of them) once
// the display-list/scene layers exist. Coordinates are logical pixels; HiDPI
// is handled internally via gg's device scale.
package paint

import (
	"image"
	"image/color"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
)

// Color is a straight-alpha RGBA color with float32 components in [0, 1].
type Color struct {
	R, G, B, A float32
}

// RGB returns an opaque Color.
func RGB(r, g, b float32) Color { return Color{r, g, b, 1} }

func (c Color) nrgba() color.NRGBA {
	return color.NRGBA{
		R: uint8(clamp01(c.R) * 255),
		G: uint8(clamp01(c.G) * 255),
		B: uint8(clamp01(c.B) * 255),
		A: uint8(clamp01(c.A) * 255),
	}
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// Painter owns the drawing context and font resources across frames.
// It is not safe for concurrent use; call it only from the UI goroutine.
type Painter struct {
	dc     *gg.Context
	w, h   int
	scale  float64
	source *text.FontSource
	faces  map[float32]text.Face
}

func NewPainter() *Painter {
	return &Painter{faces: map[float32]text.Face{}}
}

// LoadFont sets the font used by Canvas.Text from raw TTF/OTF data.
func (p *Painter) LoadFont(data []byte) error {
	src, err := text.NewFontSource(data)
	if err != nil {
		return err
	}
	p.source = src
	clear(p.faces)
	return nil
}

func (p *Painter) face(size float32) text.Face {
	if p.source == nil {
		return nil
	}
	f, ok := p.faces[size]
	if !ok {
		f = p.source.Face(float64(size))
		p.faces[size] = f
	}
	return f
}

// Begin starts drawing a frame, (re)allocating the context if the surface
// size or scale changed.
func (p *Painter) Begin(f shell.Frame) *Canvas {
	size := f.Size()
	w, h := int(size.W), int(size.H)
	scale := float64(f.Scale())
	if p.dc == nil || p.w != w || p.h != h || p.scale != scale {
		p.dc = gg.NewContextWithScale(w, h, scale)
		p.w, p.h, p.scale = w, h, scale
	}
	return &Canvas{p: p, dc: p.dc}
}

// BeginOffscreen starts drawing into an offscreen surface of the given
// logical size and scale, with no shell frame. Retrieve the result with
// Image. This is the headless path used by tests and golden images.
func (p *Painter) BeginOffscreen(size geom.Size, scale float32) *Canvas {
	w, h := int(size.W), int(size.H)
	s := float64(scale)
	if p.dc == nil || p.w != w || p.h != h || p.scale != s {
		p.dc = gg.NewContextWithScale(w, h, s)
		p.w, p.h, p.scale = w, h, s
	}
	return &Canvas{p: p, dc: p.dc}
}

// Image returns the current surface contents. Valid after drawing with
// BeginOffscreen (physical-pixel resolution).
func (p *Painter) Image() image.Image {
	if p.dc == nil {
		return nil
	}
	return p.dc.Image()
}

// End flushes the frame to the shell's GPU surface.
func (p *Painter) End(f shell.Frame) error {
	view, pw, ph := f.View()
	// Empty damage rect = full compositor pass; damage-aware paths come with
	// the scene layer (PLAN.md §5).
	return p.dc.FlushGPUWithViewDamage(view, uint32(pw), uint32(ph), image.Rectangle{})
}

// Canvas draws into the current frame in logical pixels.
type Canvas struct {
	p  *Painter
	dc *gg.Context
}

// Clear fills the whole surface with c.
func (c *Canvas) Clear(col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.Clear()
}

func (c *Canvas) FillRect(r geom.Rect, col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.DrawRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
	c.dc.Fill()
}

func (c *Canvas) FillRRect(r geom.Rect, radius float32, col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.DrawRoundedRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
	c.dc.Fill()
}

func (c *Canvas) StrokeRRect(r geom.Rect, radius, width float32, col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.SetLineWidth(float64(width))
	c.dc.DrawRoundedRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
	c.dc.Stroke()
}

func (c *Canvas) Line(a, b geom.Pt, width float32, col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.SetLineWidth(float64(width))
	c.dc.DrawLine(float64(a.X), float64(a.Y), float64(b.X), float64(b.Y))
	c.dc.Stroke()
}

// Text draws s with its baseline-left at pos.
func (c *Canvas) Text(s string, pos geom.Pt, size float32, col Color) {
	face := c.p.face(size)
	if face == nil {
		return
	}
	c.dc.SetFont(face)
	c.dc.SetColor(col.nrgba())
	c.dc.DrawString(s, float64(pos.X), float64(pos.Y))
}

// MeasureText returns the logical width and height of s at the given size.
func (c *Canvas) MeasureText(s string, size float32) (w, h float32) {
	face := c.p.face(size)
	if face == nil {
		return 0, 0
	}
	c.dc.SetFont(face)
	mw, mh := c.dc.MeasureString(s)
	return float32(mw), float32(mh)
}
