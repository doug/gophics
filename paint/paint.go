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
	"math"

	"github.com/gogpu/gg"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/text"
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
	// Text draws s with its baseline-left at pos, in the default family.
	Text(s string, pos geom.Pt, size float32, c Color)
	// TextIn draws s in a named font family ("" = default).
	TextIn(font, s string, pos geom.Pt, size float32, c Color)
	// Image draws img scaled into dst (bilinear). Pass the same image
	// value across frames — scene diffing compares by identity.
	Image(img image.Image, dst geom.Rect)
	// PushClip clips subsequent drawing to r; balance with PopClip.
	// Nested clips intersect.
	PushClip(r geom.Rect)
	// PushClipRRect clips to a rounded rectangle (e.g. a rounded photo or
	// avatar); also balanced with PopClip.
	PushClipRRect(r geom.Rect, radius float32)
	PopClip()
	// PushOpacity begins a group composited at alpha [0,1] — everything
	// until the matching PopOpacity fades as one (not per-shape). Balance
	// every PushOpacity with PopOpacity.
	PushOpacity(alpha float32)
	PopOpacity()
	// PushTransform applies an affine transform (translate + scale/rotate
	// about a pivot) to everything drawn until the matching PopTransform —
	// the basis for scale/rotate animations and shared-element flights.
	// Balance every PushTransform with PopTransform.
	PushTransform(t Transform)
	PopTransform()
}

// Transform is an affine transform: content is translated by (TX, TY) and
// scaled/rotated about Pivot (in pre-transform coordinates). Zero SX/SY mean
// no scaling (treated as 1); zero Rotation means none. The mapping is
// translate, then about the pivot: rotate, then scale.
type Transform struct {
	TX, TY         float32
	SX, SY         float32 // 0 → 1 (no scale)
	Rotation       float32 // radians
	PivotX, PivotY float32
}

// MapRect returns the transform that makes content authored in rectangle src
// appear at rectangle dst — the shared-element flight primitive. Scale is
// dst/src about src's top-left, then translate src to dst.
func MapRect(src, dst geom.Rect) Transform {
	sx, sy := float32(1), float32(1)
	if w := src.Dx(); w != 0 {
		sx = dst.Dx() / w
	}
	if h := src.Dy(); h != 0 {
		sy = dst.Dy() / h
	}
	return Transform{
		TX: dst.Min.X - src.Min.X, TY: dst.Min.Y - src.Min.Y,
		SX: sx, SY: sy, PivotX: src.Min.X, PivotY: src.Min.Y,
	}
}

func (t Transform) sx() float32 {
	if t.SX == 0 {
		return 1
	}
	return t.SX
}

func (t Transform) sy() float32 {
	if t.SY == 0 {
		return 1
	}
	return t.SY
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
//
// Fonts are organized as named families: "" is the default; register others
// (e.g. "bold", "mono") with LoadFontFamily and select them per text run
// (widget.Text.Font, layout.RichSpan.Font). Fallback fonts and the system
// chain apply to every family.
type Painter struct {
	dc    *gg.Context
	w, h  int
	scale float64

	families  map[string]*text.Font
	fallbacks []*text.Font
	shapers   map[string]*text.Shaper

	// Shaping is the dominant text cost and layout re-measures every frame,
	// so shaped lines are memoized (cleared on font change or when the
	// cache grows past a bound).
	shapes  map[shapeKey]text.Line
	metrics map[metricsKey]TextMetrics
	imgBufs map[image.Image]*gg.ImageBuf

	// Rendering glyph outlines is ~80% of raster cost, so each distinct
	// text run (font, string, size, color, scale) is rasterized once into a
	// device-resolution image and blitted thereafter. Scrolling repeats the
	// same runs frame to frame, giving near-total hit rate on the hot path.
	runs map[runKey]*cachedRun
}

type runKey struct {
	font  string
	text  string
	size  float32
	col   Color
	scale float32
}

type cachedRun struct {
	buf        *gg.ImageBuf
	dstW, dstH float32 // logical draw size (maps device image back 1:1)
	offX, offY float32 // logical offset from the baseline-left draw point
}

const runCacheLimit = 4096

type shapeKey struct {
	font string
	s    string
	size float32
}

type metricsKey struct {
	font string
	size float32
}

const shapeCacheLimit = 1 << 13

func NewPainter() *Painter {
	return &Painter{
		families: map[string]*text.Font{},
		shapers:  map[string]*text.Shaper{"": text.NewShaper()},
		shapes:   map[shapeKey]text.Line{},
		metrics:  map[metricsKey]TextMetrics{},
		imgBufs:  map[image.Image]*gg.ImageBuf{},
		runs:     map[runKey]*cachedRun{},
	}
}

func (p *Painter) rebuildShapers() {
	for name, sh := range p.shapers {
		primary, ok := p.families[name]
		if !ok {
			primary = p.families[""]
		}
		chain := make([]*text.Font, 0, 1+len(p.fallbacks))
		if primary != nil {
			chain = append(chain, primary)
		}
		chain = append(chain, p.fallbacks...)
		sh.SetFonts(chain...)
	}
	clear(p.shapes)
	clear(p.runs)
	clear(p.metrics)
}

func (p *Painter) shaperFor(font string) *text.Shaper {
	if sh, ok := p.shapers[font]; ok {
		return sh
	}
	return p.shapers[""]
}

// LoadFont sets the default-family font from raw TTF/OTF data.
func (p *Painter) LoadFont(data []byte) error {
	return p.LoadFontFamily("", data)
}

// LoadFontFamily registers (or replaces) a named font family — e.g. "bold",
// "mono" — selectable per text run.
func (p *Painter) LoadFontFamily(name string, data []byte) error {
	f, err := text.Parse(data)
	if err != nil {
		return err
	}
	p.families[name] = f
	if _, ok := p.shapers[name]; !ok {
		p.shapers[name] = text.NewShaper()
	}
	p.rebuildShapers()
	return nil
}

// LoadFallbackFont appends a fallback font used by every family (per-rune
// font selection during shaping — e.g. a CJK, Arabic, or symbol font).
func (p *Painter) LoadFallbackFont(data []byte) error {
	f, err := text.Parse(data)
	if err != nil {
		return err
	}
	p.fallbacks = append(p.fallbacks, f)
	p.rebuildShapers()
	return nil
}

// Shape returns the shaped single line for s at size in the default
// family (memoized): full shaping via the text package — bidi, fallback,
// positional forms.
func (p *Painter) Shape(s string, size float32) text.Line {
	return p.ShapeIn("", s, size)
}

// ShapeIn is Shape in a named font family ("" = default).
func (p *Painter) ShapeIn(font, s string, size float32) text.Line {
	k := shapeKey{font, s, size}
	if l, ok := p.shapes[k]; ok {
		return l
	}
	if len(p.shapes) >= shapeCacheLimit {
		clear(p.shapes)
	}
	l := p.shaperFor(font).Line(s, size)
	p.shapes[k] = l
	return l
}

// MeasureWidth returns the shaped advance width of s at the given size,
// without needing an active frame. Used by layout.
func (p *Painter) MeasureWidth(s string, size float32) float32 {
	return p.Shape(s, size).Width
}

// MeasureWidthIn is MeasureWidth in a named font family.
func (p *Painter) MeasureWidthIn(font, s string, size float32) float32 {
	return p.ShapeIn(font, s, size).Width
}

// Metrics returns default-family font metrics at the given size, without
// needing an active frame. Used by layout.
func (p *Painter) Metrics(size float32) TextMetrics {
	return p.MetricsIn("", size)
}

// MetricsIn is Metrics in a named font family.
func (p *Painter) MetricsIn(font string, size float32) TextMetrics {
	k := metricsKey{font, size}
	if m, ok := p.metrics[k]; ok {
		return m
	}
	f := p.shaperFor(font).Primary()
	if f == nil {
		return TextMetrics{}
	}
	a, d, g := f.Extents(size)
	m := TextMetrics{Ascent: a, Descent: d, LineGap: g}
	p.metrics[k] = m
	return m
}

// Paragraph shapes and wraps s to maxWidth, returning positioned lines
// with rune ranges (see text.Shaper.Paragraph). Used by rich text layout.
func (p *Painter) Paragraph(s string, size, maxWidth float32) []text.Line {
	return p.ParagraphIn("", s, size, maxWidth)
}

// ParagraphIn is Paragraph in a named font family.
func (p *Painter) ParagraphIn(font, s string, size, maxWidth float32) []text.Line {
	return p.shaperFor(font).Paragraph(s, size, maxWidth)
}

// WrapText splits s into lines that fit maxWidth at the given size, using
// Unicode line-breaking (UAX #14) over shaped widths. Explicit newlines are
// respected.
func (p *Painter) WrapText(s string, size, maxWidth float32) []string {
	return p.WrapTextIn("", s, size, maxWidth)
}

// WrapTextIn is WrapText in a named font family.
func (p *Painter) WrapTextIn(font, s string, size, maxWidth float32) []string {
	lines := p.ParagraphIn(font, s, size, maxWidth)
	runes := []rune(s)
	out := make([]string, len(lines))
	for i, l := range lines {
		start, end := l.Start, l.End
		// Trim the trailing break rune (space/newline) kept by the wrapper.
		for end > start && (runes[end-1] == ' ' || runes[end-1] == '\n') {
			end--
		}
		out[i] = string(runes[start:end])
	}
	return out
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
		// gossamer chooses CPU vs GPU explicitly (this Painter surface is the
		// CPU path; the GPU path uses a separate ggcanvas context). Opt this
		// context out of the process-global accelerator — present under the
		// gossamer_gpu build — so its fills and image blits never defer to a
		// GPU and read back blank (Headless / CPU present).
		p.dc.SetGPUDisabled(true)
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

// End presents the frame to the shell's target. It is a no-op before the
// first Begin (nothing has been rasterized yet).
func (p *Painter) End(f shell.Frame) error {
	if p.dc == nil {
		return nil
	}
	switch t := f.Target().(type) {
	case shell.GPUTarget:
		// gg's GPU compositor path — usable only when the gg/gpu accelerator
		// is registered (build tag gossamer_gpu). The default shells present
		// via PixelTarget instead (M1 CPU-raster + blit; PLAN.md §5.1).
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

// GPUCanvas wraps an externally-owned gg.Context (e.g. a GPU-accelerated one
// from ggcanvas) as a gossamer Canvas, so a recorded scene can be replayed
// onto it. It shares the Painter's caches (glyph runs, image buffers). Used
// by the GPU present path (M5); the Painter's own surface is untouched.
func (p *Painter) GPUCanvas(dc *gg.Context) Canvas { return &ggCanvas{p: p, dc: dc} }

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

// Text shapes s (bidi, fallback, positional forms — see the text package)
// and blits its cached device-resolution image (the run cache), which is
// rasterized once by filling the glyph outlines — correct scripts,
// measurement/rendering parity, gg's analytic AA quality.
func (c *ggCanvas) Text(s string, pos geom.Pt, size float32, col Color) {
	c.TextIn("", s, pos, size, col)
}

func (c *ggCanvas) TextIn(font, s string, pos geom.Pt, size float32, col Color) {
	run := c.p.runFor(font, s, size, col)
	if run == nil {
		return
	}
	c.dc.DrawImageEx(run.buf, gg.DrawImageOptions{
		X: float64(pos.X + run.offX), Y: float64(pos.Y + run.offY),
		DstWidth: float64(run.dstW), DstHeight: float64(run.dstH),
	})
}

// runFor returns the cached image for a text run at the current device
// scale, rasterizing it on first use. Glyph outlines are filled once at
// device resolution into a tight image; subsequent frames blit it.
func (p *Painter) runFor(font, s string, size float32, col Color) *cachedRun {
	if s == "" {
		return nil
	}
	scale := float32(p.scale)
	if scale <= 0 {
		scale = 1
	}
	k := runKey{font, s, size, col, scale}
	if r, ok := p.runs[k]; ok {
		return r
	}
	line := p.ShapeIn(font, s, size)
	if len(line.Glyphs) == 0 {
		return nil
	}
	m := p.MetricsIn(font, size)
	const pad = 2 // device px, guards left/top glyph overhang
	wDev := int(math.Ceil(float64(line.Width*scale))) + 2*pad
	hDev := int(math.Ceil(float64((m.Ascent+m.Descent)*scale))) + 2*pad
	if wDev <= 0 || hDev <= 0 {
		return nil
	}
	scratch := gg.NewContext(wDev, hDev)
	// Glyphs are always CPU-rasterized into a bitmap (then blitted, on the GPU
	// as a textured quad in the GPU build). Opt out of the process-global
	// accelerator so this fill isn't deferred to a GPU, which would leave
	// scratch.Image() blank and make cached text runs render as nothing.
	scratch.SetGPUDisabled(true)
	scratch.SetColor(col.nrgba())
	scratch.ClearPath()
	baseline := m.Ascent*scale + pad
	sink := ggSink{scratch}
	for _, g := range line.Glyphs {
		g.Font.AppendGlyphPath(sink, g.GID, size*scale, g.X*scale+pad, baseline+g.Y*scale)
	}
	scratch.Fill()
	r := &cachedRun{
		buf:  gg.ImageBufFromImage(scratch.Image()),
		dstW: float32(wDev) / scale,
		dstH: float32(hDev) / scale,
		offX: -pad / scale,
		offY: -m.Ascent - pad/scale,
	}
	if len(p.runs) >= runCacheLimit {
		clear(p.runs)
	}
	p.runs[k] = r
	return r
}

// ggSink adapts a gg path builder to text.PathSink.
type ggSink struct{ dc *gg.Context }

func (s ggSink) MoveTo(x, y float32) { s.dc.MoveTo(float64(x), float64(y)) }
func (s ggSink) LineTo(x, y float32) { s.dc.LineTo(float64(x), float64(y)) }
func (s ggSink) QuadTo(cx, cy, x, y float32) {
	s.dc.QuadraticTo(float64(cx), float64(cy), float64(x), float64(y))
}
func (s ggSink) CubeTo(c1x, c1y, c2x, c2y, x, y float32) {
	s.dc.CubicTo(float64(c1x), float64(c1y), float64(c2x), float64(c2y), float64(x), float64(y))
}
func (s ggSink) Close() { s.dc.ClosePath() }

func (c *ggCanvas) Image(img image.Image, dst geom.Rect) {
	if img == nil || dst.IsEmpty() {
		return
	}
	buf, ok := c.p.imgBufs[img]
	if !ok {
		if len(c.p.imgBufs) > 256 {
			clear(c.p.imgBufs)
		}
		buf = gg.ImageBufFromImage(img)
		c.p.imgBufs[img] = buf
	}
	c.dc.DrawImageEx(buf, gg.DrawImageOptions{
		X: float64(dst.Min.X), Y: float64(dst.Min.Y),
		DstWidth: float64(dst.Dx()), DstHeight: float64(dst.Dy()),
	})
}

func (c *ggCanvas) PushClip(r geom.Rect) {
	c.dc.Push()
	c.dc.ClipRect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()))
}

func (c *ggCanvas) PushClipRRect(r geom.Rect, radius float32) {
	c.dc.Push()
	c.dc.ClipRoundRect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
}

func (c *ggCanvas) PopClip() { c.dc.Pop() }

func (c *ggCanvas) PushOpacity(alpha float32) { c.dc.PushLayer(gg.BlendNormal, float64(alpha)) }
func (c *ggCanvas) PopOpacity()               { c.dc.PopLayer() }

func (c *ggCanvas) PushTransform(t Transform) {
	c.dc.Push() // saves the current matrix (restored by Pop)
	c.dc.Translate(float64(t.TX), float64(t.TY))
	px, py := float64(t.PivotX), float64(t.PivotY)
	c.dc.Translate(px, py)
	if t.Rotation != 0 {
		c.dc.Rotate(float64(t.Rotation))
	}
	c.dc.Scale(float64(t.sx()), float64(t.sy()))
	c.dc.Translate(-px, -py)
}

func (c *ggCanvas) PopTransform() { c.dc.Pop() }

// LoadSystemFonts extends every family with the platform's installed fonts
// (see text.Shaper.UseSystemFonts). Call after loading fonts.
func (p *Painter) LoadSystemFonts() error {
	for _, sh := range p.shapers {
		if err := sh.UseSystemFonts(""); err != nil {
			return err
		}
	}
	clear(p.shapes)
	clear(p.runs)
	return nil
}
