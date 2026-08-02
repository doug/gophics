// Package scene provides display lists: recorded paint commands that can be
// replayed onto any paint.Canvas. This is the M1 layer that decouples what
// the render tree paints from how a backend draws it (PLAN.md §5) — the
// foundation for damage tracking, repaint caching, and alternative backends.
package scene

import (
	"image"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// List is a recorded sequence of paint commands.
type List struct {
	ops       []op
	hasLayers bool // an opacity group was recorded (partial replay unsafe)
}

// Recorder returns a paint.Canvas that appends into the list.
func (l *List) Recorder() paint.Canvas { return recorder{l} }

// Reset clears the list for re-recording, keeping capacity.
func (l *List) Reset() { l.ops, l.hasLayers = l.ops[:0], false }

// HasLayers reports whether an opacity group was recorded. Such frames must
// repaint in full — culled partial replay would composite an incomplete
// layer.
func (l *List) HasLayers() bool { return l.hasLayers }

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

// fillPathOp holds a retained path by pointer plus the generation captured at
// record time, so opEqual (a == b) compares identity + gen + color — safe
// because a *paint.Path pointer is comparable where the path's slices are not.
type fillPathOp struct {
	p   *paint.Path
	gen uint64
	col paint.Color
}

type strokePathOp struct {
	p     *paint.Path
	gen   uint64
	width float32
	col   paint.Color
}

type textOp struct {
	font string
	s    string
	pos  geom.Pt
	size float32
	col  paint.Color
}

type imageOp struct {
	img image.Image
	dst geom.Rect
}

type pushClipOp struct{ r geom.Rect }

type pushClipRRectOp struct {
	r      geom.Rect
	radius float32
}

type popClipOp struct{}

type pushOpacityOp struct{ alpha float32 }

type popOpacityOp struct{}

type pushTransformOp struct{ t paint.Transform }

type popTransformOp struct{}

func (o clearOp) replay(c paint.Canvas) { c.Clear(o.col) }
func (o rectOp) replay(c paint.Canvas)  { c.FillRect(o.r, o.col) }
func (o rrectOp) replay(c paint.Canvas) { c.FillRRect(o.r, o.radius, o.col) }
func (o rrectGradientOp) replay(c paint.Canvas) {
	c.FillRRectGradient(o.r, o.radius, o.from, o.to, o.horizontal)
}
func (o strokeRRectOp) replay(c paint.Canvas)   { c.StrokeRRect(o.r, o.radius, o.width, o.col) }
func (o lineOp) replay(c paint.Canvas)          { c.Line(o.a, o.b, o.width, o.col) }
func (o fillPathOp) replay(c paint.Canvas)      { c.FillPath(o.p, o.col) }
func (o strokePathOp) replay(c paint.Canvas)    { c.StrokePath(o.p, o.width, o.col) }
func (o textOp) replay(c paint.Canvas)          { c.TextIn(o.font, o.s, o.pos, o.size, o.col) }
func (o imageOp) replay(c paint.Canvas)         { c.Image(o.img, o.dst) }
func (o pushClipOp) replay(c paint.Canvas)      { c.PushClip(o.r) }
func (o pushClipRRectOp) replay(c paint.Canvas) { c.PushClipRRect(o.r, o.radius) }
func (o popClipOp) replay(c paint.Canvas)       { c.PopClip() }
func (o pushOpacityOp) replay(c paint.Canvas)   { c.PushOpacity(o.alpha) }
func (o popOpacityOp) replay(c paint.Canvas)    { c.PopOpacity() }
func (o pushTransformOp) replay(c paint.Canvas) { c.PushTransform(o.t) }
func (o popTransformOp) replay(c paint.Canvas)  { c.PopTransform() }

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

func (r recorder) FillPath(p *paint.Path, col paint.Color) {
	if p == nil || p.Empty() {
		return
	}
	r.l.ops = append(r.l.ops, fillPathOp{p, p.Gen(), col})
}

func (r recorder) StrokePath(p *paint.Path, width float32, col paint.Color) {
	if p == nil || p.Empty() {
		return
	}
	r.l.ops = append(r.l.ops, strokePathOp{p, p.Gen(), width, col})
}

func (r recorder) Text(s string, pos geom.Pt, size float32, col paint.Color) {
	r.l.ops = append(r.l.ops, textOp{"", s, pos, size, col})
}

func (r recorder) TextIn(font, s string, pos geom.Pt, size float32, col paint.Color) {
	r.l.ops = append(r.l.ops, textOp{font, s, pos, size, col})
}

func (r recorder) Image(img image.Image, dst geom.Rect) {
	r.l.ops = append(r.l.ops, imageOp{img, dst})
}

func (r recorder) PushClip(rect geom.Rect) { r.l.ops = append(r.l.ops, pushClipOp{rect}) }
func (r recorder) PushClipRRect(rect geom.Rect, radius float32) {
	r.l.ops = append(r.l.ops, pushClipRRectOp{rect, radius})
}
func (r recorder) PopClip() { r.l.ops = append(r.l.ops, popClipOp{}) }

func (r recorder) PushOpacity(alpha float32) {
	r.l.hasLayers = true
	r.l.ops = append(r.l.ops, pushOpacityOp{alpha})
}
func (r recorder) PopOpacity() { r.l.ops = append(r.l.ops, popOpacityOp{}) }

func (r recorder) PushTransform(t paint.Transform) {
	// A transform reshapes the coordinate space of every op inside it, so
	// damage-culled partial replay can't reason about their bounds — force a
	// full-surface repaint for the frame (as with opacity groups).
	r.l.hasLayers = true
	r.l.ops = append(r.l.ops, pushTransformOp{t})
}
func (r recorder) PopTransform() { r.l.ops = append(r.l.ops, popTransformOp{}) }
