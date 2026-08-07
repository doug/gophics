// Command whiteboard is an Excalidraw-style sketchpad: a hand-drawn ("rough")
// shape tool set plus a smooth, variable-width freehand pen, over a toolbar with
// color swatches, three stroke widths, undo/redo, and clear.
//
// Two rendering ideas drive it:
//
//   - Rough shapes. Rectangles, diamonds, ellipses, lines and arrows are drawn
//     as slightly wobbled paths, each edge stroked twice with independent
//     jitter — the overlapping double pass is what reads as hand-drawn. The
//     wobble is deterministic per element: every element carries a stable int64
//     seed, and each frame re-seeds a math/rand PRNG from it, so shapes never
//     shimmer between frames.
//   - Freehand pen. Raw drag points become a filled outline (perfect-freehand
//     style): a left and right rail offset from the centerline by a per-point
//     half-width that tapers up at the start and down at the end, joined by
//     rounded caps. A single tap renders as a filled dot.
//
// Elements are retained in a slice with a snapshot undo/redo stack; the eraser
// removes whole elements by hit-testing their outline.
//
//	go run ./examples/whiteboard
package main

import (
	"log"
	"math"
	"math/rand"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// tool is the active drawing mode selected in the toolbar.
type tool int

const (
	toolPen tool = iota
	toolRect
	toolDiamond
	toolEllipse
	toolLine
	toolArrow
	toolEraser
	numTools
)

const (
	numColors = 6
	toolbarH  = 56
)

var (
	palette = [numColors]paint.Color{
		paint.RGB(0.11, 0.12, 0.14), // ink
		paint.RGB(0.90, 0.24, 0.24), // red
		paint.RGB(0.20, 0.50, 0.95), // blue
		paint.RGB(0.18, 0.70, 0.42), // green
		paint.RGB(0.97, 0.60, 0.18), // orange
		paint.RGB(0.60, 0.35, 0.85), // purple
	}
	widths = [3]float32{2.5, 5, 11}

	paper     = paint.RGB(0.99, 0.99, 0.99)
	toolbarBg = paint.RGB(0.96, 0.96, 0.97)
	borderCol = paint.RGB(0.85, 0.86, 0.88)
	ringCol   = paint.RGB(0.20, 0.50, 0.95)
	btnBg     = paint.RGB(1, 1, 1)
	btnFg     = paint.RGB(0.28, 0.30, 0.34)
	activeFg  = paint.RGB(1, 1, 1)
)

// element is one committed drawing: its tool, geometry, style and a stable
// random seed. For freehand pen, pts holds the raw centerline; for shapes, a and
// b are two opposite corners (or line/arrow endpoints). cache is the built path
// (a filled outline for the pen, a wobbled stroke network for shapes); it is
// rebuilt from the same seed, so it is byte-for-byte stable across frames.
type element struct {
	tool  tool
	pts   []geom.Pt
	a, b  geom.Pt
	col   paint.Color
	w     float32
	seed  int64
	cache *paint.Path
}

func (e *element) rng() *rand.Rand { return rand.New(rand.NewSource(e.seed)) }

// build (re)generates the element's retained path. Rough shapes seed their
// wobble from e.seed so the path is identical every rebuild.
func (e *element) build() {
	p := paint.NewPath()
	switch e.tool {
	case toolPen:
		buildFreehand(p, e.pts, e.w)
	case toolLine:
		roughLineInto(p, e.a, e.b, e.rng())
	case toolArrow:
		roughArrowInto(p, e.a, e.b, e.rng())
	case toolRect:
		roughRectInto(p, bounds(e.a, e.b), e.rng())
	case toolDiamond:
		roughDiamondInto(p, bounds(e.a, e.b), e.rng())
	case toolEllipse:
		roughEllipseInto(p, bounds(e.a, e.b), e.rng())
	}
	e.cache = p
}

// hit reports whether p lands on the element's outline (used by the eraser).
func (e *element) hit(p geom.Pt) bool {
	tol := e.w/2 + 6
	switch e.tool {
	case toolPen:
		return polyHit(e.pts, p, e.w*2+4, false) // the pen paints a thick band
	case toolLine, toolArrow:
		return segDist(e.a, e.b, p) <= tol
	case toolRect:
		return polyHit(rectPts(bounds(e.a, e.b)), p, tol, true)
	case toolDiamond:
		return polyHit(diamondPts(bounds(e.a, e.b)), p, tol, true)
	case toolEllipse:
		return polyHit(ellipsePts(bounds(e.a, e.b), 28), p, tol, true)
	}
	return false
}

type Board struct{}

func (Board) CreateState() widget.State { return &board{} }

type board struct {
	widget.StateBase[Board]
	elements []*element
	cur      *element // in-progress element during a drag
	last     geom.Pt
	erased   bool // an erase gesture has already pushed its undo snapshot
	undo     [][]*element
	redo     [][]*element

	col  paint.Color
	w    float32
	tool tool
	ctx  widget.Ctx

	// hit-test rects, set during draw
	swatch   [numColors]geom.Rect
	toolBtn  [numTools]geom.Rect
	widthBtn [3]geom.Rect
	undoBtn  geom.Rect
	redoBtn  geom.Rect
	clearBtn geom.Rect
	drawArea geom.Rect
}

// stateHook, if set, receives the state on mount — for tests to drive input.
var stateHook func(*board)

func (s *board) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.col = palette[0]
	s.w = widths[1]
	s.tool = toolPen
	s.elements = sampleElements()
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *board) snapshot() []*element { return append([]*element(nil), s.elements...) }

// pushUndo records the current elements so the next mutation can be reversed.
func (s *board) pushUndo() {
	s.undo = append(s.undo, s.snapshot())
	s.redo = nil
}

func (s *board) doUndo() {
	if len(s.undo) == 0 {
		return
	}
	s.redo = append(s.redo, s.snapshot())
	s.elements = s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.ctx.Invalidate()
}

func (s *board) doRedo() {
	if len(s.redo) == 0 {
		return
	}
	s.undo = append(s.undo, s.snapshot())
	s.elements = s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.ctx.Invalidate()
}

func (s *board) doClear() {
	if len(s.elements) == 0 {
		return
	}
	s.pushUndo()
	s.elements = nil
	s.ctx.Invalidate()
}

// startElement begins a new in-progress element under the pointer.
func (s *board) startElement(p geom.Pt) {
	e := &element{tool: s.tool, col: s.col, w: s.w, seed: rand.Int63()}
	if s.tool == toolPen {
		e.pts = []geom.Pt{p}
	} else {
		e.a, e.b = p, p
	}
	s.cur = e
	s.last = p
	s.ctx.Invalidate()
}

// drag advances the current gesture: pen appends a point, shapes move their end
// corner, the eraser removes whatever the pointer touches.
func (s *board) drag(p geom.Pt) {
	if s.tool == toolEraser {
		s.eraseAt(p)
		return
	}
	if s.cur == nil {
		return
	}
	if s.cur.tool == toolPen {
		if !s.drawArea.Contains(p) { // drop points that leave the canvas
			return
		}
		if dist(p, s.last) < 1.2 { // skip near-duplicate points
			return
		}
		s.cur.pts = append(s.cur.pts, p)
		s.cur.cache = nil
		s.last = p
		s.ctx.Invalidate()
		return
	}
	s.cur.b = clampToRect(p, s.drawArea)
	s.cur.cache = nil
	s.ctx.Invalidate()
}

// commit finishes the current gesture, retaining a non-degenerate element.
func (s *board) commit() {
	if s.cur == nil {
		return
	}
	e := s.cur
	s.cur = nil
	if e.tool != toolPen && dist(e.a, e.b) < 2 {
		s.ctx.Invalidate() // a bare click with a shape tool draws nothing
		return
	}
	e.build()
	s.pushUndo()
	s.elements = append(s.elements, e)
	s.ctx.Invalidate()
}

// eraseAt removes the topmost element under p; the first removal of a gesture
// pushes one undo snapshot so the whole gesture reverts as a unit.
func (s *board) eraseAt(p geom.Pt) {
	for i := len(s.elements) - 1; i >= 0; i-- {
		if s.elements[i].hit(p) {
			if !s.erased {
				s.pushUndo()
				s.erased = true
			}
			s.elements = append(s.elements[:i:i], s.elements[i+1:]...)
			s.ctx.Invalidate()
			return
		}
	}
}

func (s *board) onPress(p geom.Pt) {
	if s.drawArea.Contains(p) {
		s.erased = false
		if s.tool == toolEraser {
			s.eraseAt(p)
		} else {
			s.startElement(p)
		}
		return
	}
	s.cur = nil // a toolbar press is not a drawing
	for i := range palette {
		if s.swatch[i].Contains(p) {
			s.col = palette[i]
			s.ctx.Invalidate()
			return
		}
	}
	for i := tool(0); i < numTools; i++ {
		if s.toolBtn[i].Contains(p) {
			s.tool = i
			s.ctx.Invalidate()
			return
		}
	}
	switch {
	case s.widthBtn[0].Contains(p):
		s.w = widths[0]
		s.ctx.Invalidate()
	case s.widthBtn[1].Contains(p):
		s.w = widths[1]
		s.ctx.Invalidate()
	case s.widthBtn[2].Contains(p):
		s.w = widths[2]
		s.ctx.Invalidate()
	case s.undoBtn.Contains(p):
		s.doUndo()
	case s.redoBtn.Contains(p):
		s.doRedo()
	case s.clearBtn.Contains(p):
		s.doClear()
	}
}

func (s *board) Build(_ widget.Ctx) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{
			OnPress:   func(p geom.Pt) { s.onPress(p) },
			OnDrag:    func(pos, _ geom.Pt) { s.drag(pos) },
			OnRelease: func() { s.commit() }, // fires after a drag
			OnTap:     func() { s.commit() }, // fires after a tap (a dot)
			OnKey: func(k shell.Key) {
				if k.Kind == shell.KeyPress && (k.Code == shell.KeyBackspace || k.Code == shell.KeyDelete) {
					s.doUndo()
				}
			},
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
}

func (s *board) render(c paint.Canvas, e *element) {
	if e.cache == nil {
		e.build()
	}
	if e.tool == toolPen {
		c.FillPath(e.cache, e.col)
	} else {
		c.StrokePath(e.cache, e.w, e.col)
	}
}

func (s *board) draw(c paint.Canvas, sz geom.Size) {
	s.drawArea = geom.RectXYWH(0, toolbarH, sz.W, sz.H-toolbarH)
	c.FillRect(s.drawArea, paper)

	for _, e := range s.elements {
		s.render(c, e)
	}
	if s.cur != nil {
		s.render(c, s.cur)
	}

	s.drawToolbar(c, sz)
}

func (s *board) drawToolbar(c paint.Canvas, sz geom.Size) {
	c.FillRect(geom.RectXYWH(0, 0, sz.W, toolbarH), toolbarBg)
	c.Line(geom.Pt{X: 0, Y: toolbarH}, geom.Pt{X: sz.W, Y: toolbarH}, 1, borderCol)

	cy := float32(toolbarH) / 2
	x := float32(14)

	// Tool selector: a rounded square per tool with a small vector icon.
	const tb = 30
	for i := tool(0); i < numTools; i++ {
		rect := geom.RectXYWH(x, cy-tb/2, tb, tb)
		s.toolBtn[i] = rect
		fg := btnFg
		if s.tool == i {
			c.FillRRect(rect, 7, ringCol)
			fg = activeFg
		}
		toolIcon(c, i, geom.InsetsAll(8).Inset(rect), fg)
		x += tb + 4
	}

	x += 12 // separator

	// Color swatches.
	r := float32(9)
	for i := range palette {
		rect := geom.RectXYWH(x, cy-r, 2*r, 2*r)
		s.swatch[i] = rect
		if s.col == palette[i] {
			c.FillRRect(geom.RectXYWH(x-3, cy-r-3, 2*r+6, 2*r+6), r+3, ringCol)
		}
		c.FillRRect(rect, r, palette[i])
		x += 2*r + 8
	}

	x += 12 // separator

	// Width options: three dots sized by their stroke width.
	for i, w := range widths {
		cell := geom.RectXYWH(x, 0, 30, toolbarH)
		s.widthBtn[i] = cell
		if s.w == w {
			c.FillRRect(geom.RectXYWH(x+1, cy-15, 28, 30), 8, ringCol)
		}
		dot := w + 3
		fg := btnFg
		if s.w == w {
			fg = activeFg
		}
		c.FillRRect(geom.RectXYWH(x+15-dot/2, cy-dot/2, dot, dot), dot/2, fg)
		x += 30
	}

	// Right-aligned action buttons.
	bw, bh := float32(52), float32(32)
	rx := sz.W - 14 - bw
	s.clearBtn = geom.RectXYWH(rx, cy-bh/2, bw, bh)
	s.redoBtn = geom.RectXYWH(rx-6-bw, cy-bh/2, bw, bh)
	s.undoBtn = geom.RectXYWH(rx-2*(6+bw), cy-bh/2, bw, bh)
	s.textBtn(c, s.undoBtn, "Undo", len(s.undo) > 0)
	s.textBtn(c, s.redoBtn, "Redo", len(s.redo) > 0)
	s.textBtn(c, s.clearBtn, "Clear", len(s.elements) > 0)
}

func (s *board) textBtn(c paint.Canvas, r geom.Rect, label string, enabled bool) {
	c.FillRRect(r, 7, btnBg)
	c.StrokeRRect(r, 7, 1, borderCol)
	fg := btnFg
	if !enabled {
		fg = paint.RGB(0.75, 0.77, 0.80)
	}
	w := s.ctx.Painter().MeasureWidth(label, 13)
	c.TextIn("", label, geom.Pt{X: r.Min.X + (r.Dx()-w)/2, Y: r.Min.Y + r.Dy()/2 + 5}, 13, fg)
}

// toolIcon draws a small glyph for each tool inside r, in color col.
func toolIcon(c paint.Canvas, k tool, r geom.Rect, col paint.Color) {
	const lw = 1.8
	bl := geom.Pt{X: r.Min.X, Y: r.Max.Y}
	tr := geom.Pt{X: r.Max.X, Y: r.Min.Y}
	switch k {
	case toolPen:
		c.Line(bl, tr, lw, col) // a pen held nib-down
		nib := bl.Lerp(tr, 0.22)
		c.Line(bl, geom.Pt{X: nib.X, Y: bl.Y}, lw, col)
	case toolRect:
		c.StrokeRRect(r, 3, lw, col)
	case toolDiamond:
		for _, e := range diamondEdges(r) {
			c.Line(e[0], e[1], lw, col)
		}
	case toolEllipse:
		p := paint.NewPath()
		circleInto(p, center(r), r.Dx()/2, r.Dy()/2, 20)
		c.StrokePath(p, lw, col)
	case toolLine:
		c.Line(bl, tr, lw, col)
	case toolArrow:
		c.Line(bl, tr, lw, col)
		dir := norm(bl.Sub(tr))
		c.Line(tr, tr.Add(rot(dir, 0.5).Mul(6)), lw, col)
		c.Line(tr, tr.Add(rot(dir, -0.5).Mul(6)), lw, col)
	case toolEraser:
		block := geom.InsetsSymmetric(1, 4).Inset(r)
		c.StrokeRRect(block, 2, lw, col)
		c.Line(geom.Pt{X: block.Min.X, Y: center(block).Y}, geom.Pt{X: block.Max.X, Y: center(block).Y}, lw, col)
	}
}

func diamondEdges(r geom.Rect) [4][2]geom.Pt {
	p := diamondPts(r)
	return [4][2]geom.Pt{{p[0], p[1]}, {p[1], p[2]}, {p[2], p[3]}, {p[3], p[0]}}
}

// --- Rough (hand-drawn) shape building ------------------------------------
//
// Each builder appends wobbled subpaths to p; the caller strokes the whole
// network once with round caps/joins. Edges are laid down twice with
// independent jitter, and that overlap is what reads as hand-drawn.

// roughLineInto appends a wobbled a→b segment, twice. The segment is split into
// ~len/40 chunks (min 2); interior points are offset perpendicular by a small
// length-scaled jitter and threaded with QuadTo for a smooth waver. Endpoints
// jitter only slightly.
func roughLineInto(p *paint.Path, a, b geom.Pt, rng *rand.Rand) {
	d := b.Sub(a)
	length := vlen(d)
	if length < 0.5 {
		return
	}
	nrm := perp(d.Mul(1 / length))
	n := int(length / 40)
	if n < 2 {
		n = 2
	}
	amp := clampf(length*0.02, 0.5, 2)
	for pass := 0; pass < 2; pass++ {
		pts := make([]geom.Pt, n+1)
		for i := 0; i <= n; i++ {
			a2 := amp
			if i == 0 || i == n {
				a2 = amp * 0.35 // endpoints stay close to true
			}
			off := (rng.Float32()*2 - 1) * a2
			pts[i] = a.Lerp(b, float32(i)/float32(n)).Add(nrm.Mul(off))
		}
		quadThrough(p, pts)
	}
}

// roughRectInto draws the four edges, each with corners that slightly
// overshoot/undershoot rather than meeting cleanly.
func roughRectInto(p *paint.Path, r geom.Rect, rng *rand.Rand) {
	tl := r.Min
	tr := geom.Pt{X: r.Max.X, Y: r.Min.Y}
	br := r.Max
	bl := geom.Pt{X: r.Min.X, Y: r.Max.Y}
	edge := func(a, b geom.Pt) {
		d := norm(b.Sub(a))
		a2 := a.Sub(d.Mul(rng.Float32() * 3)) // pull the ends past the corners
		b2 := b.Add(d.Mul(rng.Float32() * 3))
		roughLineInto(p, a2, b2, rng)
	}
	edge(tl, tr)
	edge(tr, br)
	edge(br, bl)
	edge(bl, tl)
}

// roughDiamondInto draws the 4 edges between the midpoints of r's sides.
func roughDiamondInto(p *paint.Path, r geom.Rect, rng *rand.Rand) {
	d := diamondPts(r)
	roughLineInto(p, d[0], d[1], rng)
	roughLineInto(p, d[1], d[2], rng)
	roughLineInto(p, d[2], d[3], rng)
	roughLineInto(p, d[3], d[0], rng)
}

// roughEllipseInto samples points around the ellipse, jitters each radially, and
// threads them with QuadTo; it overshoots the seam and draws the loop twice.
func roughEllipseInto(p *paint.Path, r geom.Rect, rng *rand.Rand) {
	cx, cy := center(r).X, center(r).Y
	rx, ry := r.Dx()/2, r.Dy()/2
	if rx < 0.5 || ry < 0.5 {
		return
	}
	const n = 22
	for pass := 0; pass < 2; pass++ {
		pts := make([]geom.Pt, 0, n+3)
		for i := 0; i <= n+2; i++ { // +2 samples overlap the start
			a := 2 * math.Pi * float64(i) / float64(n)
			jit := (rng.Float32()*2 - 1) * 1.6
			pts = append(pts, geom.Pt{
				X: cx + (rx+jit)*float32(math.Cos(a)),
				Y: cy + (ry+jit)*float32(math.Sin(a)),
			})
		}
		quadThrough(p, pts)
	}
}

// roughArrowInto is a rough shaft plus two short rough barbs at b.
func roughArrowInto(p *paint.Path, a, b geom.Pt, rng *rand.Rand) {
	roughLineInto(p, a, b, rng)
	d := b.Sub(a)
	length := vlen(d)
	if length < 1 {
		return
	}
	back := d.Mul(-1 / length)
	const ang = 28 * math.Pi / 180
	head := clampf(length*0.25, 8, 22)
	roughLineInto(p, b, b.Add(rot(back, ang).Mul(head)), rng)
	roughLineInto(p, b, b.Add(rot(back, -ang).Mul(head)), rng)
}

// quadThrough threads an open polyline as a smooth path: midpoint quadratics
// with the sample points as control points.
func quadThrough(p *paint.Path, pts []geom.Pt) {
	switch len(pts) {
	case 0:
		return
	case 1:
		p.MoveTo(pts[0])
		return
	case 2:
		p.MoveTo(pts[0]).LineTo(pts[1])
		return
	}
	p.MoveTo(pts[0])
	for i := 1; i < len(pts)-1; i++ {
		p.QuadTo(pts[i], pts[i].Lerp(pts[i+1], 0.5))
	}
	p.LineTo(pts[len(pts)-1])
}

// --- Freehand pen (perfect-freehand-style filled outline) -----------------

// buildFreehand writes a filled, variable-width tapered outline for the raw pen
// points into p. Each point gets a half-width that ramps up over the first few
// points and down over the last few (and thins a little on fast segments); the
// centerline is offset ±half-width into a left and right rail, joined by rounded
// end and start caps. One point becomes a dot.
func buildFreehand(p *paint.Path, pts []geom.Pt, w float32) {
	base := w * 4 // pen strokes are chunkier than the hairline shapes
	if len(pts) == 0 {
		return
	}
	if len(pts) == 1 {
		circleInto(p, pts[0], base/2, base/2, 14)
		return
	}
	n := len(pts)
	const ramp = 4
	half := make([]float32, n)
	for i := range pts {
		tin := clampf(float32(i)/ramp, 0, 1)
		tout := clampf(float32(n-1-i)/ramp, 0, 1)
		t := tin
		if tout < t {
			t = tout
		}
		ease := t * t * (3 - 2*t) // smoothstep taper toward the ends
		speed := float32(0)
		if i > 0 {
			speed = dist(pts[i], pts[i-1])
		}
		sf := 1 - clampf((speed-6)/60, 0, 0.35) // thin slightly on fast strokes
		half[i] = clampf(base*0.5*ease*sf, 0.35, base)
	}

	left := make([]geom.Pt, n)
	right := make([]geom.Pt, n)
	for i := range pts {
		nrm := perp(tangent(pts, i))
		left[i] = pts[i].Add(nrm.Mul(half[i]))
		right[i] = pts[i].Sub(nrm.Mul(half[i]))
	}

	// Outline: left rail forward, end cap, right rail backward, start cap.
	out := make([]geom.Pt, 0, 2*n+16)
	out = append(out, left...)
	out = append(out, capPts(pts[n-1], tangent(pts, n-1), half[n-1], false)...)
	for i := n - 1; i >= 0; i-- {
		out = append(out, right[i])
	}
	out = append(out, capPts(pts[0], tangent(pts, 0), half[0], true)...)
	closedSmooth(p, out)
}

// tangent is the unit centerline direction at point i.
func tangent(pts []geom.Pt, i int) geom.Pt {
	n := len(pts)
	switch {
	case n < 2:
		return geom.Pt{X: 1}
	case i == 0:
		return norm(pts[1].Sub(pts[0]))
	case i == n-1:
		return norm(pts[i].Sub(pts[i-1]))
	default:
		return norm(pts[i+1].Sub(pts[i-1]))
	}
}

// capPts returns the interior arc points of a rounded cap of radius r at center,
// bulging along +dir for an end cap (start=false) or −dir for a start cap. The
// rail endpoints themselves are omitted (they're already in the outline).
func capPts(center, dir geom.Pt, r float32, start bool) []geom.Pt {
	const steps = 6
	nrm := perp(dir)
	out := make([]geom.Pt, 0, steps-1)
	for k := 1; k < steps; k++ {
		t := math.Pi * float64(k) / steps
		cs := float32(math.Cos(t))
		sn := float32(math.Sin(t))
		if start { // from −nrm through −dir to +nrm
			out = append(out, center.Sub(nrm.Mul(cs*r)).Sub(dir.Mul(sn*r)))
		} else { // from +nrm through +dir to −nrm
			out = append(out, center.Add(nrm.Mul(cs*r)).Add(dir.Mul(sn*r)))
		}
	}
	return out
}

// closedSmooth fills a closed loop through pts with midpoint quadratics.
func closedSmooth(p *paint.Path, pts []geom.Pt) {
	n := len(pts)
	if n < 3 {
		if n > 0 {
			p.MoveTo(pts[0])
		}
		for i := 1; i < n; i++ {
			p.LineTo(pts[i])
		}
		if n > 0 {
			p.Close()
		}
		return
	}
	p.MoveTo(pts[n-1].Lerp(pts[0], 0.5))
	for i := 0; i < n; i++ {
		p.QuadTo(pts[i], pts[i].Lerp(pts[(i+1)%n], 0.5))
	}
	p.Close()
}

// circleInto writes a smooth closed ellipse of radii rx, ry into p.
func circleInto(p *paint.Path, center geom.Pt, rx, ry float32, n int) {
	pts := make([]geom.Pt, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = geom.Pt{X: center.X + rx*float32(math.Cos(a)), Y: center.Y + ry*float32(math.Sin(a))}
	}
	closedSmooth(p, pts)
}

// --- small vector / geometry helpers --------------------------------------

func bounds(a, b geom.Pt) geom.Rect {
	return geom.Rect{
		Min: geom.Pt{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
		Max: geom.Pt{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
	}
}

func center(r geom.Rect) geom.Pt {
	return geom.Pt{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
}

func clampToRect(p geom.Pt, r geom.Rect) geom.Pt {
	return geom.Pt{X: clampf(p.X, r.Min.X, r.Max.X), Y: clampf(p.Y, r.Min.Y, r.Max.Y)}
}

func rectPts(r geom.Rect) []geom.Pt {
	return []geom.Pt{r.Min, {X: r.Max.X, Y: r.Min.Y}, r.Max, {X: r.Min.X, Y: r.Max.Y}}
}

func diamondPts(r geom.Rect) []geom.Pt {
	c := center(r)
	return []geom.Pt{{X: c.X, Y: r.Min.Y}, {X: r.Max.X, Y: c.Y}, {X: c.X, Y: r.Max.Y}, {X: r.Min.X, Y: c.Y}}
}

func ellipsePts(r geom.Rect, n int) []geom.Pt {
	c := center(r)
	rx, ry := r.Dx()/2, r.Dy()/2
	pts := make([]geom.Pt, n)
	for i := 0; i < n; i++ {
		a := 2 * math.Pi * float64(i) / float64(n)
		pts[i] = geom.Pt{X: c.X + rx*float32(math.Cos(a)), Y: c.Y + ry*float32(math.Sin(a))}
	}
	return pts
}

// polyHit reports whether p is within tol of any segment of the polyline (its
// closing edge too when closed).
func polyHit(pts []geom.Pt, p geom.Pt, tol float32, closed bool) bool {
	if len(pts) == 0 {
		return false
	}
	if len(pts) == 1 {
		return dist(pts[0], p) <= tol
	}
	for i := 0; i+1 < len(pts); i++ {
		if segDist(pts[i], pts[i+1], p) <= tol {
			return true
		}
	}
	if closed && len(pts) > 2 && segDist(pts[len(pts)-1], pts[0], p) <= tol {
		return true
	}
	return false
}

// segDist is the distance from p to segment a–b.
func segDist(a, b, p geom.Pt) float32 {
	ab := b.Sub(a)
	l2 := ab.X*ab.X + ab.Y*ab.Y
	if l2 == 0 {
		return dist(a, p)
	}
	t := clampf(((p.X-a.X)*ab.X+(p.Y-a.Y)*ab.Y)/l2, 0, 1)
	return dist(p, a.Add(ab.Mul(t)))
}

func vlen(p geom.Pt) float32 { return float32(math.Hypot(float64(p.X), float64(p.Y))) }

func norm(p geom.Pt) geom.Pt {
	l := vlen(p)
	if l == 0 {
		return geom.Pt{}
	}
	return geom.Pt{X: p.X / l, Y: p.Y / l}
}

func perp(p geom.Pt) geom.Pt { return geom.Pt{X: -p.Y, Y: p.X} }

func rot(v geom.Pt, t float32) geom.Pt {
	cs := float32(math.Cos(float64(t)))
	sn := float32(math.Sin(float64(t)))
	return geom.Pt{X: v.X*cs - v.Y*sn, Y: v.X*sn + v.Y*cs}
}

func clampf(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func dist(a, b geom.Pt) float32 {
	return float32(math.Hypot(float64(a.X-b.X), float64(a.Y-b.Y)))
}

// sampleElements seeds the board with a little rough sketch so a fresh canvas
// (and the gallery thumbnail) isn't blank.
func sampleElements() []*element {
	shape := func(t tool, a, b geom.Pt, col paint.Color, w float32, seed int64) *element {
		return &element{tool: t, a: a, b: b, col: col, w: w, seed: seed}
	}
	pen := func(col paint.Color, w float32, pts ...geom.Pt) *element {
		return &element{tool: toolPen, pts: pts, col: col, w: w}
	}
	squiggle := make([]geom.Pt, 0, 24)
	for i := 0; i <= 22; i++ {
		t := float32(i) / 22
		squiggle = append(squiggle, geom.Pt{
			X: 500 + t*180,
			Y: 300 + 26*float32(math.Sin(float64(t)*3*math.Pi)),
		})
	}
	return []*element{
		shape(toolRect, geom.Pt{X: 70, Y: 110}, geom.Pt{X: 230, Y: 220}, palette[2], widths[1], 1),
		shape(toolDiamond, geom.Pt{X: 270, Y: 110}, geom.Pt{X: 410, Y: 230}, palette[3], widths[1], 2),
		shape(toolEllipse, geom.Pt{X: 450, Y: 110}, geom.Pt{X: 610, Y: 220}, palette[4], widths[1], 3),
		shape(toolArrow, geom.Pt{X: 70, Y: 300}, geom.Pt{X: 240, Y: 360}, palette[1], widths[1], 4),
		shape(toolLine, geom.Pt{X: 270, Y: 360}, geom.Pt{X: 430, Y: 290}, palette[5], widths[0], 5),
		pen(palette[0], widths[1], squiggle...),
	}
}

func main() {
	if err := app.Run(Board{}, app.Config{
		Title:      "Whiteboard",
		Size:       geom.Size{W: 900, H: 560},
		Background: paper,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
