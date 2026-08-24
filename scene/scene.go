// Package scene provides display lists: recorded paint commands that can be
// replayed onto any paint.Canvas. This is the M1 layer that decouples what
// the render tree paints from how a backend draws it — the
// foundation for damage tracking, repaint caching, and alternative backends.
package scene

import (
	"image"
	"reflect"
	"sync/atomic"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// List is a recorded sequence of paint commands.
type List struct {
	ops       []op
	hasLayers bool // a transform was recorded (damage bounds invalid → full repaint)
	// clipStack/xformDepth back Recorder().ClipBounds() so containers can cull
	// off-screen children during the record pass. clipStack holds the running
	// clip intersection; xformDepth counts active transforms (culling is disabled
	// while any is active). See ClipBounds in recorder.
	clipStack  []geom.Rect
	xformDepth int
}

// Recorder returns a paint.Canvas that appends into the list.
func (l *List) Recorder() paint.Canvas { return recorder{l} }

// Reset clears the list for re-recording, keeping capacity.
func (l *List) Reset() {
	// Zero the retained ops so pointers held by dead entries (*paint.Path,
	// image.Image, text strings) don't outlive their frame in the reused array.
	clear(l.ops)
	l.ops, l.hasLayers = l.ops[:0], false
	l.clipStack, l.xformDepth = l.clipStack[:0], 0
}

func (l *List) pushClip(r geom.Rect) {
	cur := geom.Unbounded
	if n := len(l.clipStack); n > 0 {
		cur = l.clipStack[n-1]
	}
	l.clipStack = append(l.clipStack, cur.Intersect(r))
}

// HasLayers reports whether the frame recorded anything that forces a full
// repaint instead of damage-culled partial replay: a transform group, whose
// inside ops are recorded in a different coordinate space, so their bounds
// can't feed the surface-space damage rect. When true the caller must replay
// the whole list (do not call ReplayDamage). Opacity groups no longer set
// this: their content bounds are surface-space, so Diff computes tight
// damage for them and partial replay composites correctly.
func (l *List) HasLayers() bool { return l.hasLayers }

// Len returns the number of recorded commands.
func (l *List) Len() int { return len(l.ops) }

// BackdropBlurs is how many backdrop blurs this frame records.
//
// It is the one op whose cost is not its own: a blur re-reads everything drawn
// behind it, so the price is the content underneath, paid again per blur. That
// makes the count worth asserting on directly — a frame can grow one of these
// without any visible change and lose several milliseconds to it.
func (l *List) BackdropBlurs() int {
	n := 0
	for _, o := range l.ops {
		if o.kind == opBackdropBlur {
			n++
		}
	}
	return n
}

// Replay draws the recorded commands onto c in order.
func (l *List) Replay(c paint.Canvas) {
	for i := range l.ops {
		l.ops[i].replay(c)
	}
}

// opKind tags which paint command an op holds.
type opKind uint8

const (
	opClear opKind = iota
	opFillRect
	opFillRRect
	opFillRRectGradient
	opStrokeRRect
	opLine
	opFillPath
	opStrokePath
	opText
	opImage
	opSprite
	opPushClip
	opPushClipRRect
	opPopClip
	opPushOpacity
	opPopOpacity
	opPushTransform
	opPopTransform
	opBackdropBlur
)

// op is one recorded paint command as a tagged value (kind + union fields),
// stored by value in List.ops — no per-op heap allocation when recording.
// Field use by kind:
//
//	r      rect for fills/strokes/clips/blur; image dst; line endpoints
//	       (Min=a, Max=b, unnormalized); record-time path bounds; text
//	       baseline-left in Min (Max unused)
//	f1     corner radius (rrect/clip-rrect), blur radius, line width,
//	       text size, opacity alpha
//	f2     stroke width (strokeRRect/strokePath)
//	col    fill/stroke/text color; gradient "from"; clear color
//	col2   gradient "to"
//	str1   text font family
//	str2   text string
//	path   retained path (fill/stroke path); see the comment on FillPath below
//	gen    path generation captured at record time
//	img    image (opImage) or sprite atlas (opSprite), kept for replay
//	imgKey identity key for img computed at record time (see imageKey)
//	sprite sprite blit parameters (opSprite)
//	xform  affine transform (opPushTransform)
//	horiz  gradient axis (opFillRRectGradient)
type op struct {
	kind       opKind
	horiz      bool
	f1, f2     float32
	r          geom.Rect
	col, col2  paint.Color
	str1, str2 string
	path       *paint.Path
	gen        uint64
	img        image.Image
	imgKey     uintptr
	sprite     paint.Sprite
	xform      paint.Transform
}

func (o *op) replay(c paint.Canvas) {
	switch o.kind {
	case opClear:
		c.Clear(o.col)
	case opFillRect:
		c.FillRect(o.r, o.col)
	case opFillRRect:
		c.FillRRect(o.r, o.f1, o.col)
	case opFillRRectGradient:
		c.FillRRectGradient(o.r, o.f1, o.col, o.col2, o.horiz)
	case opStrokeRRect:
		c.StrokeRRect(o.r, o.f1, o.f2, o.col)
	case opLine:
		c.Line(o.r.Min, o.r.Max, o.f1, o.col)
	case opFillPath:
		c.FillPath(o.path, o.col)
	case opStrokePath:
		c.StrokePath(o.path, o.f2, o.col)
	case opText:
		c.TextIn(o.str1, o.str2, o.r.Min, o.f1, o.col)
	case opImage:
		c.Image(o.img, o.r)
	case opSprite:
		c.DrawSprite(o.img, o.sprite)
	case opPushClip:
		c.PushClip(o.r)
	case opPushClipRRect:
		c.PushClipRRect(o.r, o.f1)
	case opPopClip:
		c.PopClip()
	case opPushOpacity:
		c.PushOpacity(o.f1)
	case opPopOpacity:
		c.PopOpacity()
	case opPushTransform:
		c.PushTransform(o.xform)
	case opPopTransform:
		c.PopTransform()
	case opBackdropBlur:
		c.BackdropBlur(o.r, o.f1)
	}
}

// imageKeyCounter mints never-repeating sentinel keys for images without a
// cheap stable identity (non-pointer dynamic types).
var imageKeyCounter atomic.Uintptr

// imageKey returns an identity key for img used by Diff instead of interface
// equality (which panics on non-comparable dynamic types). Pointer-typed
// images — every standard-library image (*image.RGBA, *image.NRGBA, …) — key
// by the pointer itself: the same image value compares equal across frames,
// and mutating pixels in place does NOT change the key (pass a new image
// value to signal a repaint). Any other dynamic type gets a fresh sentinel
// each record, so such ops always diff as changed — an extra repaint at
// worst, never a panic. Sentinels are odd; pointers are word-aligned (even),
// so the two key spaces never collide.
func imageKey(img image.Image) uintptr {
	if img == nil {
		return 0
	}
	if v := reflect.ValueOf(img); v.Kind() == reflect.Pointer {
		return v.Pointer()
	}
	return imageKeyCounter.Add(1)*2 + 1
}

type recorder struct{ l *List }

func (r recorder) Clear(col paint.Color) {
	r.l.ops = append(r.l.ops, op{kind: opClear, col: col})
}

func (r recorder) FillRect(rect geom.Rect, col paint.Color) {
	r.l.ops = append(r.l.ops, op{kind: opFillRect, r: rect, col: col})
}

func (r recorder) FillRRect(rect geom.Rect, radius float32, col paint.Color) {
	r.l.ops = append(r.l.ops, op{kind: opFillRRect, r: rect, f1: radius, col: col})
}

func (r recorder) FillRRectGradient(rect geom.Rect, radius float32, from, to paint.Color, horizontal bool) {
	r.l.ops = append(r.l.ops, op{kind: opFillRRectGradient, r: rect, f1: radius, col: from, col2: to, horiz: horizontal})
}

func (r recorder) StrokeRRect(rect geom.Rect, radius, width float32, col paint.Color) {
	r.l.ops = append(r.l.ops, op{kind: opStrokeRRect, r: rect, f1: radius, f2: width, col: col})
}

func (r recorder) Line(a, b geom.Pt, width float32, col paint.Color) {
	r.l.ops = append(r.l.ops, op{kind: opLine, r: geom.Rect{Min: a, Max: b}, f1: width, col: col})
}

// FillPath records a retained path by pointer plus the generation captured at
// record time, so opEqual compares identity + gen + color — safe because a
// *paint.Path pointer is comparable where the path's slices are not.
// The op's r holds the path's extent at record time: a retained path is
// mutated in place across frames, so reading path.Bounds() live during Diff
// would report the new extent for the old op too, and the region the path
// vacated would never enter the damage rect (stale pixels). Capturing it at
// record time keeps Diff honest.
func (r recorder) FillPath(p *paint.Path, col paint.Color) {
	if p == nil || p.Empty() {
		return
	}
	r.l.ops = append(r.l.ops, op{kind: opFillPath, path: p, gen: p.Gen(), r: p.Bounds(), col: col})
}

func (r recorder) StrokePath(p *paint.Path, width float32, col paint.Color) {
	if p == nil || p.Empty() {
		return
	}
	r.l.ops = append(r.l.ops, op{kind: opStrokePath, path: p, gen: p.Gen(), r: p.Bounds(), f2: width, col: col})
}

func (r recorder) TextIn(font, s string, pos geom.Pt, size float32, col paint.Color) {
	r.l.ops = append(r.l.ops, op{kind: opText, str1: font, str2: s, r: geom.Rect{Min: pos}, f1: size, col: col})
}

func (r recorder) Image(img image.Image, dst geom.Rect) {
	r.l.ops = append(r.l.ops, op{kind: opImage, img: img, imgKey: imageKey(img), r: dst})
}

// DrawSprite records a blit of a source region of a shared atlas. The atlas
// is keyed by identity like Image; the Sprite value is comparable as-is.
func (r recorder) DrawSprite(atlas image.Image, s paint.Sprite) {
	if atlas == nil || s.Dst.IsEmpty() {
		return
	}
	r.l.ops = append(r.l.ops, op{kind: opSprite, img: atlas, imgKey: imageKey(atlas), sprite: s})
}

// ClipBounds implements paint.Canvas: the current clip intersection in canvas
// coordinates, or geom.Unbounded when unclipped or while any transform is active.
func (r recorder) ClipBounds() geom.Rect {
	if r.l.xformDepth > 0 || len(r.l.clipStack) == 0 {
		return geom.Unbounded
	}
	return r.l.clipStack[len(r.l.clipStack)-1]
}

func (r recorder) PushClip(rect geom.Rect) {
	r.l.ops = append(r.l.ops, op{kind: opPushClip, r: rect})
	r.l.pushClip(rect)
}
func (r recorder) PushClipRRect(rect geom.Rect, radius float32) {
	r.l.ops = append(r.l.ops, op{kind: opPushClipRRect, r: rect, f1: radius})
	r.l.pushClip(rect)
}
func (r recorder) PopClip() {
	r.l.ops = append(r.l.ops, op{kind: opPopClip})
	if n := len(r.l.clipStack); n > 0 {
		r.l.clipStack = r.l.clipStack[:n-1]
	}
}

func (r recorder) PushOpacity(alpha float32) {
	// Opacity groups do NOT set hasLayers: their content is recorded in
	// surface coordinates, so Diff can bound damage to the group's content
	// (see groupBounds in diff.go) and partial replay composites correctly —
	// draws into the layer honor the active clip, and the layer composites
	// source-over, leaving pixels outside the damage clip untouched.
	r.l.ops = append(r.l.ops, op{kind: opPushOpacity, f1: alpha})
}
func (r recorder) PopOpacity() { r.l.ops = append(r.l.ops, op{kind: opPopOpacity}) }

func (r recorder) PushTransform(t paint.Transform) {
	// A transform reshapes the coordinate space of every op inside it, so
	// damage-culled partial replay can't reason about their bounds — force a
	// full-surface repaint for the frame.
	r.l.hasLayers = true
	r.l.ops = append(r.l.ops, op{kind: opPushTransform, xform: t})
	r.l.xformDepth++
}
func (r recorder) PopTransform() {
	r.l.ops = append(r.l.ops, op{kind: opPopTransform})
	if r.l.xformDepth > 0 {
		r.l.xformDepth--
	}
}

func (r recorder) BackdropBlur(rect geom.Rect, radius float32) {
	r.l.ops = append(r.l.ops, op{kind: opBackdropBlur, r: rect, f1: radius})
}
