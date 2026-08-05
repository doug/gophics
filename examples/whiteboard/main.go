// Command whiteboard is a freehand vector sketchpad: drag to draw smooth
// round-capped strokes, pick a color and width, erase, and undo/redo. It is the
// driver example for interactive paint.Path work — each stroke is a retained
// polyline path built live from pointer drags and stroked by the GPU — plus the
// press/drag/release gesture path and a snapshot undo stack.
//
//	go run ./examples/whiteboard
package main

import (
	"log"
	"math"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
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
	eraserFg  = paint.RGB(0.55, 0.57, 0.62)
	eraseW    = float32(26)
)

// stroke is one committed mark: a retained polyline path plus its style. Once
// committed the path is never mutated, so its pointer stays stable for the
// scene's damage tracking.
type stroke struct {
	path *paint.Path
	col  paint.Color
	w    float32
	n    int // point count (1 = a tapped dot)
}

type Board struct{}

func (Board) CreateState() widget.State { return &board{} }

type board struct {
	widget.StateBase[Board]
	strokes []stroke
	cur     *stroke // in-progress stroke during a drag
	last    geom.Pt
	undo    [][]stroke // snapshots for undo
	redo    [][]stroke

	col    paint.Color
	w      float32
	eraser bool
	ctx    widget.Ctx

	// hit-test rects, set during draw
	swatch    [numColors]geom.Rect
	eraserBtn geom.Rect
	widthBtn  [3]geom.Rect
	undoBtn   geom.Rect
	redoBtn   geom.Rect
	clearBtn  geom.Rect
	drawArea  geom.Rect
}

// stateHook, if set, receives the state on mount — for tests to drive input.
var stateHook func(*board)

func (s *board) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.col = palette[0]
	s.w = widths[1]
	s.strokes = sampleStrokes()
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *board) snapshot() []stroke { return append([]stroke(nil), s.strokes...) }

// pushUndo records the current strokes so the next mutation can be reversed.
func (s *board) pushUndo() {
	s.undo = append(s.undo, s.snapshot())
	s.redo = nil
}

func (s *board) doUndo() {
	if len(s.undo) == 0 {
		return
	}
	s.redo = append(s.redo, s.snapshot())
	s.strokes = s.undo[len(s.undo)-1]
	s.undo = s.undo[:len(s.undo)-1]
	s.ctx.Invalidate()
}

func (s *board) doRedo() {
	if len(s.redo) == 0 {
		return
	}
	s.undo = append(s.undo, s.snapshot())
	s.strokes = s.redo[len(s.redo)-1]
	s.redo = s.redo[:len(s.redo)-1]
	s.ctx.Invalidate()
}

func (s *board) doClear() {
	if len(s.strokes) == 0 {
		return
	}
	s.pushUndo()
	s.strokes = nil
	s.ctx.Invalidate()
}

func (s *board) startStroke(p geom.Pt) {
	col, w := s.col, s.w
	if s.eraser {
		col, w = paper, eraseW
	}
	path := paint.NewPath()
	path.MoveTo(p)
	s.cur = &stroke{path: path, col: col, w: w, n: 1}
	s.last = p
	s.ctx.Invalidate()
}

func (s *board) extendStroke(p geom.Pt) {
	if s.cur == nil || !s.drawArea.Contains(p) {
		return
	}
	if dist(p, s.last) < 1.2 { // skip near-duplicate points
		return
	}
	s.cur.path.LineTo(p)
	s.cur.n++
	s.last = p
	s.ctx.Invalidate()
}

func (s *board) endStroke() {
	if s.cur == nil {
		return
	}
	if s.cur.n == 1 {
		s.cur.path.LineTo(s.last) // a tap: degenerate segment renders as a round dot
	}
	s.pushUndo()
	s.strokes = append(s.strokes, *s.cur)
	s.cur = nil
	s.ctx.Invalidate()
}

func (s *board) onPress(p geom.Pt) {
	if s.drawArea.Contains(p) {
		s.startStroke(p)
		return
	}
	s.cur = nil // a toolbar press is not a stroke
	for i := range palette {
		if s.swatch[i].Contains(p) {
			s.col, s.eraser = palette[i], false
			s.ctx.Invalidate()
			return
		}
	}
	switch {
	case s.eraserBtn.Contains(p):
		s.eraser = true
		s.ctx.Invalidate()
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
			OnDrag:    func(pos, _ geom.Pt) { s.extendStroke(pos) },
			OnRelease: func() { s.endStroke() }, // fires after a drag
			OnTap:     func() { s.endStroke() }, // fires after a tap (a dot); no-op on toolbar
			OnKey: func(k shell.Key) {
				if k.Kind == shell.KeyPress && (k.Code == shell.KeyBackspace || k.Code == shell.KeyDelete) {
					s.doUndo()
				}
			},
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
}

func (s *board) draw(c paint.Canvas, sz geom.Size) {
	s.drawArea = geom.RectXYWH(0, toolbarH, sz.W, sz.H-toolbarH)
	c.FillRect(s.drawArea, paper)

	for i := range s.strokes {
		st := &s.strokes[i]
		c.StrokePath(st.path, st.w, st.col)
	}
	if s.cur != nil {
		c.StrokePath(s.cur.path, s.cur.w, s.cur.col)
	}

	// Toolbar.
	c.FillRect(geom.RectXYWH(0, 0, sz.W, toolbarH), toolbarBg)
	c.Line(geom.Pt{X: 0, Y: toolbarH}, geom.Pt{X: sz.W, Y: toolbarH}, 1, borderCol)

	cy := float32(toolbarH) / 2
	x := float32(16)
	r := float32(13)
	for i := range palette {
		rect := geom.RectXYWH(x, cy-r, 2*r, 2*r)
		s.swatch[i] = rect
		if !s.eraser && s.col == palette[i] {
			c.FillRRect(geom.RectXYWH(x-3, cy-r-3, 2*r+6, 2*r+6), r+3, ringCol)
		}
		c.FillRRect(rect, r, palette[i])
		x += 2*r + 9
	}

	// Eraser swatch (paper fill with an outline; ring when active).
	x += 4
	erect := geom.RectXYWH(x, cy-r, 2*r, 2*r)
	s.eraserBtn = erect
	if s.eraser {
		c.FillRRect(geom.RectXYWH(x-3, cy-r-3, 2*r+6, 2*r+6), r+3, ringCol)
	}
	c.FillRRect(erect, r, paper)
	c.StrokeRRect(erect, r, 1.5, eraserFg)
	c.Text("E", geom.Pt{X: x + r - 4, Y: cy + 5}, 14, eraserFg)
	x += 2*r + 20

	// Width options: three dots sized by their stroke width.
	for i, w := range widths {
		cell := geom.RectXYWH(x, 0, 34, toolbarH)
		s.widthBtn[i] = cell
		if !s.eraser && s.w == w {
			c.FillRRect(geom.RectXYWH(x+3, cy-15, 28, 30), 8, ringCol)
		}
		dot := w + 3
		c.FillRRect(geom.RectXYWH(x+17-dot/2, cy-dot/2, dot, dot), dot/2, btnFg)
		x += 34
	}

	// Right-aligned action buttons.
	bw, bh := float32(64), float32(32)
	rx := sz.W - 16 - bw
	s.clearBtn = geom.RectXYWH(rx, cy-bh/2, bw, bh)
	s.redoBtn = geom.RectXYWH(rx-8-bw, cy-bh/2, bw, bh)
	s.undoBtn = geom.RectXYWH(rx-2*(8+bw), cy-bh/2, bw, bh)
	textBtn(c, s.undoBtn, "Undo", len(s.undo) > 0)
	textBtn(c, s.redoBtn, "Redo", len(s.redo) > 0)
	textBtn(c, s.clearBtn, "Clear", len(s.strokes) > 0)
}

func textBtn(c paint.Canvas, r geom.Rect, label string, enabled bool) {
	c.FillRRect(r, 7, btnBg)
	c.StrokeRRect(r, 7, 1, borderCol)
	fg := btnFg
	if !enabled {
		fg = paint.RGB(0.75, 0.77, 0.80)
	}
	w := float32(len(label)) * 7
	c.Text(label, geom.Pt{X: r.Min.X + (r.Dx()-w)/2, Y: r.Min.Y + r.Dy()/2 + 5}, 14, fg)
}

// sampleStrokes seeds the board with a little sketch so a fresh canvas (and the
// gallery thumbnail) isn't blank.
func sampleStrokes() []stroke {
	mk := func(col paint.Color, w float32, pts ...geom.Pt) stroke {
		p := paint.NewPath()
		p.MoveTo(pts[0])
		for _, q := range pts[1:] {
			p.LineTo(q)
		}
		return stroke{path: p, col: col, w: w, n: len(pts)}
	}
	arc := func(cx, cy, rad, a0, a1 float32, steps int) []geom.Pt {
		pts := make([]geom.Pt, 0, steps+1)
		for i := 0; i <= steps; i++ {
			a := float64(a0 + (a1-a0)*float32(i)/float32(steps))
			pts = append(pts, geom.Pt{X: cx + rad*float32(math.Cos(a)), Y: cy + rad*float32(math.Sin(a))})
		}
		return pts
	}
	cx, cy := float32(150), float32(170)
	return []stroke{
		mk(palette[0], 5, arc(cx, cy, 78, 0, 2*math.Pi, 40)...),                                                                                      // face
		mk(palette[2], 9, geom.Pt{X: cx - 30, Y: cy - 28}, geom.Pt{X: cx - 30, Y: cy - 6}),                                                           // left eye
		mk(palette[2], 9, geom.Pt{X: cx + 30, Y: cy - 28}, geom.Pt{X: cx + 30, Y: cy - 6}),                                                           // right eye
		mk(palette[1], 6, arc(cx, cy+6, 44, 0.35*math.Pi, 0.65*math.Pi, 20)...),                                                                      // smile
		mk(palette[4], 8, geom.Pt{X: 300, Y: 120}, geom.Pt{X: 340, Y: 90}, geom.Pt{X: 380, Y: 130}, geom.Pt{X: 420, Y: 95}, geom.Pt{X: 460, Y: 135}), // orange zigzag
		mk(palette[3], 5, arc(390, 230, 46, -0.1*math.Pi, 1.1*math.Pi, 26)...),                                                                       // green wave-ish arc
	}
}

func dist(a, b geom.Pt) float32 {
	dx, dy := a.X-b.X, a.Y-b.Y
	return float32(math.Hypot(float64(dx), float64(dy)))
}

func main() {
	if err := app.Run(Board{}, app.Config{
		Title:      "Whiteboard",
		Size:       geom.Size{W: 760, H: 460},
		Background: paper,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
