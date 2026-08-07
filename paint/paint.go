// Package paint provides gophics's drawing layer.
//
// Canvas is the drawing interface the render layer paints into. The default
// implementation wraps gogpu/gg's CPU rasterizer (analytic AA), presented to
// the shell's frame target; scene.Recorder implements the same interface to
// capture display lists (backends are pluggable behind Canvas).
// Coordinates are logical pixels; HiDPI is handled via gg's device scale.
package paint

import (
	"image"
	"image/color"
	"image/draw"
	"math"

	"github.com/doug/gophics/internal/gfx/gg"
	ggtext "github.com/doug/gophics/internal/gfx/gg/text"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/text"
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
	// FillPath fills a retained path (non-zero winding). Pass the same *Path
	// across frames so scene diffing can compare it by identity.
	FillPath(p *Path, c Color)
	// StrokePath strokes a retained path with round caps and joins.
	StrokePath(p *Path, width float32, c Color)
	Line(a, b geom.Pt, width float32, c Color)
	// TextIn draws s with its baseline-left at pos, in the named font family
	// ("" = the default family).
	TextIn(font, s string, pos geom.Pt, size float32, c Color)
	// Image draws img scaled into dst (bilinear). Pass the same image
	// value across frames — scene diffing compares by identity.
	Image(img image.Image, dst geom.Rect)
	// DrawSprite blits a source region of atlas into Dst (see Sprite). Pass the
	// same atlas value across calls to share one cached texture.
	DrawSprite(atlas image.Image, s Sprite)
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
	// BackdropBlur frosts the already-drawn content within r by the given
	// radius — the core of a glass/vibrancy material. It reads what is behind,
	// so call it before painting a panel's translucent tint and content on top;
	// clip to a rounded rect first for rounded glass. On the CPU rasterizer it
	// is a box blur of the pixmap; on the GPU it renders the backdrop to a
	// reduced-resolution offscreen and composites it back upscaled (a downsample
	// blur), scissored to r — so it works on both paths.
	BackdropBlur(r geom.Rect, radius float32)
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
// a Gaussian blur with layered rrects (gg has no blur primitive); blur is the
// softness radius, offset shifts the
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
	shapes   map[shapeKey]text.Line
	metrics  map[metricsKey]TextMetrics
	imgBufs  map[image.Image]*gg.ImageBuf
	tintBufs map[tintKey]*gg.ImageBuf // per (atlas, src rect, quantized tint)

	// Rendering glyph outlines is ~80% of raster cost, so each distinct
	// text run (font, string, size, color, scale) is rasterized once into a
	// device-resolution image and blitted thereafter. Scrolling repeats the
	// same runs frame to frame, giving near-total hit rate on the hot path.
	runs map[runKey]*cachedRun

	// GPU text uses gg's glyph-mask tier, which needs a gg face built from the
	// same font bytes (matching GIDs). Sources are created at load; faces are
	// memoized per (family, size).
	ggSources map[string]*ggtext.FontSource
	ggFaces   map[ggFaceKey]ggtext.Face
}

type ggFaceKey struct {
	family string
	size   float32
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

// NewPainter creates a Painter — the app's text shaper and font/metrics/shape
// caches, and the factory for offscreen Canvases (BeginOffscreen). Make one per
// app or test and reuse it; it is not safe for concurrent use (the UI goroutine
// owns it). app.Run and app.Headless each hold one.
func NewPainter() *Painter {
	return &Painter{
		families:  map[string]*text.Font{},
		shapers:   map[string]*text.Shaper{"": text.NewShaper()},
		shapes:    map[shapeKey]text.Line{},
		metrics:   map[metricsKey]TextMetrics{},
		imgBufs:   map[image.Image]*gg.ImageBuf{},
		tintBufs:  map[tintKey]*gg.ImageBuf{},
		runs:      map[runKey]*cachedRun{},
		ggSources: map[string]*ggtext.FontSource{},
		ggFaces:   map[ggFaceKey]ggtext.Face{},
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
	// Parallel gg source over the same bytes for the GPU glyph-mask tier
	// (GIDs are font-file-intrinsic, so gg's parse matches gophics's shaper).
	if src, srcErr := ggtext.NewFontSource(data); srcErr == nil {
		p.ggSources[name] = src
		clear(p.ggFaces)
	}
	if _, ok := p.shapers[name]; !ok {
		p.shapers[name] = text.NewShaper()
	}
	p.rebuildShapers()
	return nil
}

// ggFaceFor returns the memoized gg face for a family at a logical size, or
// nil if no gg source is registered for it.
func (p *Painter) ggFaceFor(family string, size float32) ggtext.Face {
	src, ok := p.ggSources[family]
	if !ok {
		src, ok = p.ggSources[""]
		family = ""
	}
	if !ok {
		return nil
	}
	k := ggFaceKey{family, size}
	if f, ok := p.ggFaces[k]; ok {
		return f
	}
	f := src.Face(float64(size))
	p.ggFaces[k] = f
	return f
}

// glyphsFromFamily reports whether every glyph came from the family's primary
// font. Fallback-font glyphs carry GIDs from a different font, so the gg face
// (built from the family bytes) would rasterize the wrong outlines for them.
func (p *Painter) glyphsFromFamily(family string, glyphs []text.Glyph) bool {
	primary, ok := p.families[family]
	if !ok {
		primary = p.families[""]
	}
	if primary == nil {
		return false
	}
	for i := range glyphs {
		if glyphs[i].Font != primary {
			return false
		}
	}
	return true
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
		// gophics chooses CPU vs GPU explicitly (this Painter surface is the
		// CPU path; the GPU path uses a separate ggcanvas context). Opt this
		// context out of the process-global accelerator — registered by default
		// (see paint/accel.go) — so its fills and image blits never defer to a
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
		// gg's GPU compositor path — usable when the gg/gpu accelerator is
		// registered (the default; see paint/accel.go). Shells fall back to a
		// PixelTarget (CPU-raster + blit) when the GPU is unavailable.
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
// from ggcanvas) as a gophics Canvas, so a recorded scene can be replayed
// onto it. It shares the Painter's caches (glyph runs, image buffers). Used
// by the GPU present path (M5); the Painter's own surface is untouched.
func (p *Painter) GPUCanvas(dc *gg.Context) Canvas { return &ggCanvas{p: p, dc: dc} }

func (c *ggCanvas) Clear(col Color) {
	// gg's Context.Clear clears the CPU pixmap to transparent, which the GPU
	// backend never sees (it composites via its own target) — so a colored
	// clear is lost there and reads as transparent on the CPU path too. Paint
	// the whole surface instead: a device-space filled rect goes through
	// whichever rasterizer is active, so the background is correct on both.
	c.dc.Push()
	c.dc.Identity() // device space: ignore the current scale/transform
	c.dc.SetColor(col.nrgba())
	c.dc.DrawRectangle(0, 0, float64(c.dc.Width()), float64(c.dc.Height()))
	_ = c.dc.Fill()
	c.dc.Pop()
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

// FillRRectGradient fills a rounded rect with a linear gradient. On the CPU
// backend it uses gg's analytic gradient brush (one smooth fill). On the GPU
// backend, whose tile rasterizer only understands a single solid brush color,
// it falls back to compositing thin interpolated solid strips clipped to the
// rounded rect — which the GPU does support — so gradients render correctly on
// both. (A native GPU gradient shader would remove the strip overdraw; tracked.)
func (c *ggCanvas) FillRRectGradient(r geom.Rect, radius float32, from, to Color, horizontal bool) {
	if c.gpuActive() {
		c.gradientStrips(r, radius, from, to, horizontal)
		return
	}
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

// gpuActive reports whether this canvas rasterizes on the GPU: a process-global
// accelerator is registered and this context isn't opted out of it. The CPU
// present path (and the default build, with no accelerator) returns false.
func (c *ggCanvas) gpuActive() bool {
	return gg.Accelerator() != nil && !c.dc.GPUDisabled()
}

// gradientStrips approximates a two-stop linear gradient as a run of solid
// strips clipped to the rounded rect. gradientStripCount strips over a UI-sized
// band put each strip well under a pixel of color step, so banding is not
// visible; the rrect clip keeps the corners crisp.
func (c *ggCanvas) gradientStrips(r geom.Rect, radius float32, from, to Color, horizontal bool) {
	const n = gradientStripCount
	c.dc.Push()
	c.dc.ClipRoundRect(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
	span := r.Dy()
	if horizontal {
		span = r.Dx()
	}
	step := span / n
	for i := 0; i < n; i++ {
		t := (float32(i) + 0.5) / n
		c.dc.SetColor(Lerp(from, to, t).nrgba())
		if horizontal {
			c.dc.DrawRectangle(float64(r.Min.X+float32(i)*step), float64(r.Min.Y), float64(step)+1, float64(r.Dy()))
		} else {
			c.dc.DrawRectangle(float64(r.Min.X), float64(r.Min.Y+float32(i)*step), float64(r.Dx()), float64(step)+1)
		}
		c.dc.Fill()
	}
	c.dc.Pop() // restores the pre-clip state
}

// gradientStripCount is the number of solid strips a GPU gradient is composited
// from — high enough that the color step is sub-pixel for typical UI bands.
const gradientStripCount = 64

func (c *ggCanvas) FillPath(p *Path, col Color) {
	if p == nil || len(p.verbs) == 0 {
		return
	}
	c.dc.SetColor(col.nrgba())
	j := 0
	for _, v := range p.verbs {
		switch v {
		case verbMove:
			c.dc.MoveTo(float64(p.pts[j].X), float64(p.pts[j].Y))
			j++
		case verbLine:
			c.dc.LineTo(float64(p.pts[j].X), float64(p.pts[j].Y))
			j++
		case verbClose:
			c.dc.ClosePath()
		}
	}
	c.dc.Fill()
}

func (c *ggCanvas) StrokePath(p *Path, width float32, col Color) {
	if p == nil || len(p.verbs) == 0 {
		return
	}
	c.dc.SetColor(col.nrgba())
	c.dc.SetLineWidth(float64(width))
	c.dc.SetLineCap(gg.LineCapRound)
	c.dc.SetLineJoin(gg.LineJoinRound)
	j := 0
	for _, v := range p.verbs {
		switch v {
		case verbMove:
			c.dc.MoveTo(float64(p.pts[j].X), float64(p.pts[j].Y))
			j++
		case verbLine:
			c.dc.LineTo(float64(p.pts[j].X), float64(p.pts[j].Y))
			j++
		case verbClose:
			c.dc.ClosePath()
		}
	}
	c.dc.Stroke()
	c.dc.SetLineCap(gg.LineCapButt) // restore defaults so plain Line() is unaffected
	c.dc.SetLineJoin(gg.LineJoinMiter)
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

// TextIn shapes s (bidi, fallback, positional forms — see the text package)
// and blits its cached device-resolution image (the run cache), which is
// rasterized once by filling the glyph outlines — correct scripts,
// measurement/rendering parity, gg's analytic AA quality.
func (c *ggCanvas) TextIn(font, s string, pos geom.Pt, size float32, col Color) {
	if c.gpuActive() {
		// GPU path: fill the shaped glyph outlines directly into the frame's
		// shape queue (one fill per run) so text composites to the surface with
		// every other shape in a single render pass. The bitmap-blit path below
		// draws each run as a DrawImage, which forces a mid-frame accelerator
		// flush that drops the queued shapes — fatal on the direct-surface path.
		// Filling outlines is also resolution-independent (no per-scale raster).
		c.fillGlyphs(font, s, pos, size, col)
		return
	}
	run := c.p.runFor(font, s, size, col)
	if run == nil {
		return
	}
	c.dc.DrawImageEx(run.buf, gg.DrawImageOptions{
		X: float64(pos.X + run.offX), Y: float64(pos.Y + run.offY),
		DstWidth: float64(run.dstW), DstHeight: float64(run.dstH),
	})
}

// fillGlyphs shapes s and fills its glyph outlines directly on the GPU-backed
// context at logical coordinates (the context's device scale maps them to
// physical pixels). pos.Y is the baseline. One Fill per run keeps the whole
// run a single multi-contour path, which the analytic/SDF filler resolves with
// nonzero winding — glyphs solid, gaps empty.
func (c *ggCanvas) fillGlyphs(font, s string, pos geom.Pt, size float32, col Color) {
	line := c.p.ShapeIn(font, s, size)
	if len(line.Glyphs) == 0 {
		return
	}
	c.dc.SetColor(col.nrgba())

	// Preferred: gg's glyph-mask GPU text tier. It rasterizes each glyph once
	// into a device-resolution atlas (crisp AA, cached across frames) and
	// batches the quads into the render pass. Needs a gg face with matching
	// GIDs, so only when every glyph came from that family's font.
	if face := c.p.ggFaceFor(font, size); face != nil && c.p.glyphsFromFamily(font, line.Glyphs) {
		glyphs := make([]ggtext.ShapedGlyph, len(line.Glyphs))
		for i, g := range line.Glyphs {
			glyphs[i] = ggtext.ShapedGlyph{
				GID:      ggtext.GlyphID(g.GID),
				Cluster:  g.Cluster,
				X:        float64(g.X),
				Y:        float64(g.Y),
				XAdvance: float64(g.Advance),
			}
		}
		// DrawShapedGlyphs self-falls-back to outline fills if the mask tier is
		// unavailable, so this is never worse than the manual path below.
		c.dc.DrawShapedGlyphs(glyphs, face, float64(pos.X), float64(pos.Y))
		return
	}

	// Fallback: fill the glyph outlines directly (resolution-independent, but
	// re-tessellated per frame). Used for fallback-font runs (CJK, symbols).
	c.dc.ClearPath()
	sink := ggSink{c.dc}
	for _, g := range line.Glyphs {
		g.Font.AppendGlyphPath(sink, g.GID, size, pos.X+g.X, pos.Y+g.Y)
	}
	c.dc.Fill()
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
	// Force the analytic scanline filler. A glyph run is a dense multi-contour
	// path; RasterizerAuto routes complex paths to the tile-based CoverageFiller
	// whenever gg's GPU accelerator is imported (GetCoverageFiller != nil), and
	// that filler mishandles multi-contour winding — filling the gaps between
	// glyphs solid, so text renders as an opaque block. AnalyticFiller is the
	// correct path for multi-contour fills (gg's own software.go says so).
	scratch.SetRasterizerMode(gg.RasterizerAnalytic)
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

// imgBuf returns the cached gg texture for img (shared by Image and
// DrawSprite), building it on first use.
func (c *ggCanvas) imgBuf(img image.Image) *gg.ImageBuf {
	buf, ok := c.p.imgBufs[img]
	if !ok {
		if len(c.p.imgBufs) > 256 {
			clear(c.p.imgBufs)
		}
		buf = gg.ImageBufFromImage(img)
		c.p.imgBufs[img] = buf
	}
	return buf
}

func (c *ggCanvas) DrawSprite(atlas image.Image, s Sprite) {
	if atlas == nil || s.Dst.IsEmpty() || s.Src.Empty() {
		return
	}
	interp := gg.InterpBilinear
	if s.Nearest {
		interp = gg.InterpNearest
	}
	alpha := s.Alpha
	if alpha == 0 {
		alpha = 1
	}
	opts := gg.DrawImageOptions{
		X: float64(s.Dst.Min.X), Y: float64(s.Dst.Min.Y),
		DstWidth: float64(s.Dst.Dx()), DstHeight: float64(s.Dst.Dy()),
		Interpolation: interp, Opacity: float64(alpha), BlendMode: gg.BlendNormal,
	}
	var buf *gg.ImageBuf
	if s.Tint.A > 0 {
		buf = c.p.tinted(atlas, s.Src, s.Tint) // pre-multiplied sub-region; no SrcRect
	} else {
		buf = c.imgBuf(atlas)
		src := s.Src
		opts.SrcRect = &src
	}
	if s.FlipX || s.Rotation != 0 {
		cx := float64(s.Dst.Min.X) + float64(s.Dst.Dx())/2
		cy := float64(s.Dst.Min.Y) + float64(s.Dst.Dy())/2
		c.dc.Push()
		if s.Rotation != 0 {
			c.dc.RotateAbout(float64(s.Rotation), cx, cy)
		}
		if s.FlipX {
			c.dc.Translate(cx, 0)
			c.dc.Scale(-1, 1)
			c.dc.Translate(-cx, 0)
		}
		c.dc.DrawImageEx(buf, opts)
		c.dc.Pop()
		return
	}
	c.dc.DrawImageEx(buf, opts)
}

func (c *ggCanvas) Image(img image.Image, dst geom.Rect) {
	if img == nil || dst.IsEmpty() {
		return
	}
	buf := c.imgBuf(img)
	c.dc.DrawImageEx(buf, gg.DrawImageOptions{
		X: float64(dst.Min.X), Y: float64(dst.Min.Y),
		DstWidth: float64(dst.Dx()), DstHeight: float64(dst.Dy()),
		// Opacity must be set explicitly: the GPU backend treats the zero value
		// as fully transparent (the CPU path treated it as opaque), so leaving
		// it unset blitted nothing on GPU. Match DrawSprite's defaults.
		Interpolation: gg.InterpBilinear, Opacity: 1, BlendMode: gg.BlendNormal,
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

func (c *ggCanvas) BackdropBlur(r geom.Rect, radius float32) {
	c.dc.BackdropBlur(float64(r.Min.X), float64(r.Min.Y), float64(r.Dx()), float64(r.Dy()), float64(radius))
}

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
