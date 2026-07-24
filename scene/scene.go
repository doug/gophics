// Package scene provides display lists: recorded paint commands that can be
// replayed onto any paint.Canvas. This is the M1 layer that decouples what
// the render tree paints from how a backend draws it (PLAN.md §5) — the
// foundation for damage tracking, repaint caching, and alternative backends.
package scene

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// List is a recorded sequence of paint commands.
type List struct {
	ops []op
}

// Recorder returns a paint.Canvas that appends into the list.
func (l *List) Recorder() paint.Canvas { return recorder{l} }

// Reset clears the list for re-recording, keeping capacity.
func (l *List) Reset() { l.ops = l.ops[:0] }

// Len returns the number of recorded commands.
func (l *List) Len() int { return len(l.ops) }

// Replay draws the recorded commands onto c in order.
func (l *List) Replay(c paint.Canvas) {
	for _, o := range l.ops {
		o.replay(c)
	}
}

type op interface{ replay(c paint.Canvas) }

type clearOp struct{ col paint.Color }

type rectOp struct {
	r   geom.Rect
	col paint.Color
}

type rrectOp struct {
	r      geom.Rect
	radius float32
	col    paint.Color
}

type rrectGradientOp struct {
	r          geom.Rect
	radius     float32
	from, to   paint.Color
	horizontal bool
}

type strokeRRectOp struct {
	r             geom.Rect
	radius, width float32
	col           paint.Color
}

type lineOp struct {
	a, b  geom.Pt
	width float32
	col   paint.Color
}

type textOp struct {
	s    string
	pos  geom.Pt
	size float32
	col  paint.Color
}

type pushClipOp struct{ r geom.Rect }

type popClipOp struct{}

func (o clearOp) replay(c paint.Canvas) { c.Clear(o.col) }
func (o rectOp) replay(c paint.Canvas)  { c.FillRect(o.r, o.col) }
func (o rrectOp) replay(c paint.Canvas) { c.FillRRect(o.r, o.radius, o.col) }
func (o rrectGradientOp) replay(c paint.Canvas) {
	c.FillRRectGradient(o.r, o.radius, o.from, o.to, o.horizontal)
}
func (o strokeRRectOp) replay(c paint.Canvas) { c.StrokeRRect(o.r, o.radius, o.width, o.col) }
func (o lineOp) replay(c paint.Canvas)        { c.Line(o.a, o.b, o.width, o.col) }
func (o textOp) replay(c paint.Canvas)        { c.Text(o.s, o.pos, o.size, o.col) }
func (o pushClipOp) replay(c paint.Canvas)    { c.PushClip(o.r) }
func (o popClipOp) replay(c paint.Canvas)     { c.PopClip() }

type recorder struct{ l *List }

func (r recorder) Clear(col paint.Color) { r.l.ops = append(r.l.ops, clearOp{col}) }

func (r recorder) FillRect(rect geom.Rect, col paint.Color) {
	r.l.ops = append(r.l.ops, rectOp{rect, col})
}

func (r recorder) FillRRect(rect geom.Rect, radius float32, col paint.Color) {
	r.l.ops = append(r.l.ops, rrectOp{rect, radius, col})
}

func (r recorder) FillRRectGradient(rect geom.Rect, radius float32, from, to paint.Color, horizontal bool) {
	r.l.ops = append(r.l.ops, rrectGradientOp{rect, radius, from, to, horizontal})
}

func (r recorder) StrokeRRect(rect geom.Rect, radius, width float32, col paint.Color) {
	r.l.ops = append(r.l.ops, strokeRRectOp{rect, radius, width, col})
}

func (r recorder) Line(a, b geom.Pt, width float32, col paint.Color) {
	r.l.ops = append(r.l.ops, lineOp{a, b, width, col})
}

func (r recorder) Text(s string, pos geom.Pt, size float32, col paint.Color) {
	r.l.ops = append(r.l.ops, textOp{s, pos, size, col})
}

func (r recorder) PushClip(rect geom.Rect) { r.l.ops = append(r.l.ops, pushClipOp{rect}) }
func (r recorder) PopClip()                { r.l.ops = append(r.l.ops, popClipOp{}) }
