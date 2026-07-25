// Package paint provides gossamer's drawing layer.
//
// Canvas is the drawing interface the render layer paints into. The default
// implementation wraps gogpu/gg's CPU rasterizer (analytic AA), presented to
// the shell's frame target; scene.Recorder implements the same interface to
// capture display lists (PLAN.md §5: backends are pluggable behind Canvas).
// Coordinates are logical pixels; HiDPI is handled via gg's device scale.
package paint

import (
	"image"
	"image/color"
	"image/draw"
	"strings"

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

// Lerp interpolates between colors a and b; t=0 yields a, t=1 yields b.
func Lerp(a, b Color, t float32) Color {
	return Color{
		R: a.R + (b.R-a.R)*t,
		G: a.G + (b.G-a.G)*t,
		B: a.B + (b.B-a.B)*t,
		A: a.A + (b.A-a.A)*t,
	}
}

// WithAlpha returns c with its alpha multiplied by a.
func (c Color) WithAlpha(a float32) Color {
	c.A *= a
	return c
}

func (c Color) nrgba() color.NRGBA {
	return color.NRGBA{
		R: uint8(clamp01(c.R) * 255),
		G: uint8(clamp01(c.G) * 255),
		B: uint8(clamp01(c.B) * 255),
		A: uint8(clamp01(c.A) * 255),
	}
}

func (c Color) ggRGBA() gg.RGBA {
	return gg.RGBA{R: float64(c.R), G: float64(c.G), B: float64(c.B), A: float64(c.A)}
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

// Canvas is the drawing interface for one frame, in logical pixels.
// Implemented by the gg-backed painter canvas and by scene.Recorder.
type Canvas interface {
	// Clear fills the whole surface with c.
	Clear(c Color)
	FillRect(r geom.Rect, c Color)
	FillRRect(r geom.Rect, radius float32, c Color)
	// FillRRectGradient fills with a linear gradient from 'from' at the
	// top/left to 'to' at the bottom/right (by axis).
	FillRRectGradient(r geom.Rect, radius float32, from, to Color, horizontal bool)
	StrokeRRect(r geom.Rect, radius, width float32, c Color)
	Line(a, b geom.Pt, width float32, c Color)
	// Text draws s with its baseline-left at pos.
	Text(s string, pos geom.Pt, size float32, c Color)
	// PushClip clips subsequent drawing to r; balance with PopClip.
	// Nested clips intersect.
	PushClip(r geom.Rect)
	PopClip()
}

// DropShadow paints a soft shadow under the rounded rect r. It approximates
// a Gaussian blur with layered rrects (gg has no blur primitive — spike
// finding, PLAN.md §5.1); blur is the softness radius, offset shifts the
// shadow. Works on any Canvas, including recorders.
func DropShadow(c Canvas, r geom.Rect, radius float32, offset geom.Pt, blur float32, col Color) {
	const steps = 5
	if blur <= 0 {
		return
	}
	base := r.Translate(offset)
	for i := steps; i >= 1; i-- {
		grow := blur * float32(i) / steps
		layer := geom.Insets{Top: -grow, Right: -grow, Bottom: -grow, Left: -grow}.Inset(base)
		alpha := col.A / (steps * 1.6)
		c.FillRRect(layer, radius+grow, Color{col.R, col.G, col.B, alpha})
	}
}

// TextMetrics are font metrics at a given size, in logical pixels.
type TextMetrics struct {
	Ascent, Descent, LineGap float32
}

// LineHeight is the default line advance: ascent + descent + line gap.
func (m TextMetrics) LineHeight() float32 { return m.Ascent + m.Descent + m.LineGap }

// Painter owns the drawing context and font resources across frames.
// It is not safe for concurrent use; call it only from the UI goroutine.
type Painter struct {
	dc     *gg.Context
	w, h   int
	scale  float64
	source *text.FontSource
	faces  map[float32]text.Face

	// Shaping is the dominant text cost and layout re-measures every frame,
	// so advance widths are memoized (cleared on font change or when the
	// cache grows past a bound).
	widths  map[widthKey]float32
	metrics map[float32]TextMetrics
}

type widthKey struct {
	s    string
	size float32
}

const widthCacheLimit = 1 << 13

func NewPainter() *Painter {
	return &Painter{
		faces:   map[float32]text.Face{},
		widths:  map[widthKey]float32{},
		metrics: map[float32]TextMetrics{},
	}
}

// LoadFont sets the font used by Canvas.Text from raw TTF/OTF data.
func (p *Painter) LoadFont(data []byte) error {
	src, err := text.NewFontSource(data)
	if err != nil {
		return err
	}
	p.source = src
	clear(p.faces)
	clear(p.widths)
	clear(p.metrics)
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

// MeasureWidth returns the advance width of s at the given size, without
// needing an active frame. Results are memoized. Used by layout.
func (p *Painter) MeasureWidth(s string, size float32) float32 {
	k := widthKey{s, size}
	if w, ok := p.widths[k]; ok {
		return w
	}
	f := p.face(size)
	if f == nil {
		return 0
	}
	if len(p.widths) >= widthCacheLimit {
		clear(p.widths)
	}
	w := float32(f.Advance(s))
	p.widths[k] = w
	return w
}

// Metrics returns font metrics at the given size, without needing an active
// frame. Results are memoized. Used by layout.
func (p *Painter) Metrics(size float32) TextMetrics {
	if m, ok := p.metrics[size]; ok {
		return m
	}
	f := p.face(size)
	if f == nil {
		return TextMetrics{}
	}
	fm := f.Metrics()
	m := TextMetrics{
		Ascent:  float32(fm.Ascent),
		Descent: float32(fm.Descent),
		LineGap: float32(fm.LineGap),
	}
	p.metrics[size] = m
	return m
}

// WrapText splits s into lines that fit maxWidth at the given size.
// Explicit newlines are respected; lines break greedily at spaces, with
// over-long words placed on their own line (no mid-word breaking yet —
// grapheme-aware breaking arrives with the text package, PLAN.md §6.1).
func (p *Painter) WrapText(s string, size, maxWidth float32) []string {
	var lines []string
	for para := range strings.SplitSeq(s, "\n") {
		words := strings.Fields(para)
		if len(words) == 0 {
			lines = append(lines, "")
			continue
		}
		cur := words[0]
		for _, w := range words[1:] {
			candidate := cur + " " + w
			if p.MeasureWidth(candidate, size) <= maxWidth {
				cur = candidate
			} else {
				lines = append(lines, cur)
				cur = w
			}
		}
		lines = append(lines, cur)
	}
	return lines
}

// Begin starts drawing a frame, (re)allocating the context if the surface
// size or scale changed.
func (p *Painter) Begin(f shell.Frame) Canvas {
	return p.begin(f.Size(), f.Scale())
}

// BeginOffscreen starts drawing into an offscreen surface of the given
// logical size and scale, with no shell frame. Retrieve the result with
// Image. This is the headless path used by tests and golden images.
func (p *Painter) BeginOffscreen(size geom.Size, scale float32) Canvas {
	return p.begin(size, scale)
}

func (p *Painter) begin(size geom.Size, scale float32) Canvas {
	w, h := int(size.W), int(size.H)
	s := float64(scale)
	if p.dc == nil || p.w != w || p.h != h || p.scale != s {
		p.dc = gg.NewContextWithScale(w, h, s)
		p.w, p.h, p.scale = w, h, s
	}
	return &ggCanvas{p: p, dc: p.dc}
}

// Image returns the current surface contents (physical-pixel resolution).
func (p *Painter) Image() image.Image {
	if p.dc == nil {
		return nil
	}
	return p.dc.Image()
}

// End presents the frame to the shell's target.
func (p *Painter) End(f shell.Frame) error {
	switch t := f.Target().(type) {
	case shell.GPUTarget:
		// Empty damage rect = full compositor pass; damage-aware paths come
		// with scene-level dirty tracking (PLAN.md §5).
		return p.dc.FlushGPUWithViewDamage(t.View, uint32(t.W), uint32(t.H), image.Rectangle{})
	case shell.PixelTarget:
		t.Put(asRGBA(p.dc.Image()))
		return nil
	}
	return nil
}

func asRGBA(img image.Image) *image.RGBA {
	if rgba, ok := img.(*image.RGBA); ok {
		return rgba
	}
	b := img.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, img, b.Min, draw.Src)
	return out
}

// ggCanvas is the gg-backed Canvas.
type ggCanvas struct {
	p  *Painter
	dc *gg.Context
}

func (c *ggCanvas) Clear(col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.Clear()
}

func (c *ggCanvas) FillRect(r geom.Rect, col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.DrawRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
	c.dc.Fill()
}

func (c *ggCanvas) FillRRect(r geom.Rect, radius float32, col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.DrawRoundedRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
	c.dc.Fill()
}

func (c *ggCanvas) FillRRectGradient(r geom.Rect, radius float32, from, to Color, horizontal bool) {
	var brush gg.Brush
	if horizontal {
		brush = gg.LinearGradient(from.ggRGBA(), to.ggRGBA(),
			float64(r.Min.X), float64(r.Min.Y), float64(r.Max.X), float64(r.Min.Y))
	} else {
		brush = gg.LinearGradient(from.ggRGBA(), to.ggRGBA(),
			float64(r.Min.X), float64(r.Min.Y), float64(r.Min.X), float64(r.Max.Y))
	}
	c.dc.SetFillBrush(brush)
	c.dc.DrawRoundedRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
	c.dc.Fill()
	c.dc.SetColor(color.NRGBA{A: 255}) // reset brush to solid
}

func (c *ggCanvas) StrokeRRect(r geom.Rect, radius, width float32, col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.SetLineWidth(float64(width))
	c.dc.DrawRoundedRectangle(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
	c.dc.Stroke()
}

func (c *ggCanvas) Line(a, b geom.Pt, width float32, col Color) {
	c.dc.SetColor(col.nrgba())
	c.dc.SetLineWidth(float64(width))
	c.dc.DrawLine(float64(a.X), float64(a.Y), float64(b.X), float64(b.Y))
	c.dc.Stroke()
}

func (c *ggCanvas) Text(s string, pos geom.Pt, size float32, col Color) {
	face := c.p.face(size)
	if face == nil {
		return
	}
	c.dc.SetFont(face)
	c.dc.SetColor(col.nrgba())
	c.dc.DrawString(s, float64(pos.X), float64(pos.Y))
}

func (c *ggCanvas) PushClip(r geom.Rect) {
	c.dc.Push()
	c.dc.ClipRect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
}

func (c *ggCanvas) PopClip() { c.dc.Pop() }
