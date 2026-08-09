package scene

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Measurer supplies text extents for damage computation; paint.Painter
// implements it.
type Measurer interface {
	MeasureWidthIn(font, s string, size float32) float32
	MetricsIn(font string, size float32) paint.TextMetrics
}

// Diff compares l against prev and returns the damage: the union of the
// bounds of every op that changed, was added, or was removed, plus whether
// anything changed at all. Equal lists return (zero, false).
//
// The comparison is positional over the longest common prefix and suffix —
// cheap and tight for localized changes (the common case: one widget
// repainted). Ops between the prefix and suffix contribute their bounds
// from both lists. A changed opacity push/pop damages its whole group's
// content bounds (both lists), so a fading widget repaints only its rect.
// Any other difference in clip/transform structure inside the changed
// window falls back to unbounded damage (the caller clamps to the surface).
func (l *List) Diff(prev *List, m Measurer) (geom.Rect, bool) {
	a, b := prev.ops, l.ops

	// Common prefix.
	i := 0
	for i < len(a) && i < len(b) && opEqual(&a[i], &b[i]) {
		i++
	}
	if i == len(a) && i == len(b) {
		return geom.Rect{}, false
	}
	// Common suffix (not overlapping the prefix).
	ja, jb := len(a), len(b)
	for ja > i && jb > i && opEqual(&a[ja-1], &b[jb-1]) {
		ja--
		jb--
	}

	var damage geom.Rect
	unbounded := false
	add := func(ops []op, lo, hi int) {
		for k := lo; k < hi; k++ {
			switch ops[k].kind {
			case opClear, opPushClip, opPushClipRRect, opPopClip,
				opPushTransform, opPopTransform:
				// Structural change: damage everything.
				unbounded = true
			case opPushOpacity:
				// A changed/added/removed group boundary: the whole group's
				// content is (re)composited, so damage its content bounds.
				damage = damage.Union(groupBounds(ops, k, +1, m, &unbounded))
			case opPopOpacity:
				damage = damage.Union(groupBounds(ops, k, -1, m, &unbounded))
			default:
				damage = damage.Union(opBounds(&ops[k], m))
			}
		}
	}
	add(a, i, ja)
	add(b, i, jb)
	if unbounded {
		return geom.Rect{Max: geom.Pt{X: layoutInf, Y: layoutInf}}, true
	}
	return damage, true
}

var layoutInf = float32(1 << 30)

// groupBounds returns the union of the content bounds of the opacity group
// whose push (dir=+1) or pop (dir=-1) sits at ops[k], scanning toward the
// matching partner. Bounds are valid because opacity groups record in
// surface coordinates. Clip ops inside the group are skipped — clips only
// shrink content, so the un-clipped union is a conservative superset. A
// transform or clear inside the group, or an unbalanced group, sets
// *unbounded instead (the transform's content bounds are in another space).
func groupBounds(ops []op, k, dir int, m Measurer, unbounded *bool) geom.Rect {
	var b geom.Rect
	depth := 1
	for j := k + dir; j >= 0 && j < len(ops); j += dir {
		o := &ops[j]
		switch o.kind {
		case opPushOpacity:
			depth += dir
		case opPopOpacity:
			depth -= dir
		case opPushTransform, opPopTransform, opClear:
			*unbounded = true
			return geom.Rect{}
		case opPushClip, opPushClipRRect, opPopClip:
			// Clips only shrink; ignoring them keeps a superset.
		default:
			b = b.Union(opBounds(o, m))
		}
		if depth == 0 {
			return b
		}
	}
	*unbounded = true // unbalanced group
	return geom.Rect{}
}

// ReplayDamage replays only the ops intersecting damage onto c. Clip,
// opacity, and transform structure is always preserved; other ops are culled
// by bounds. The caller is expected to clip c to damage first — culling saves
// command execution (text shaping, path setup), clipping guarantees pixel
// correctness (including opacity groups: draws into the layer honor the clip,
// and the layer composites source-over, so pixels outside damage are
// untouched).
func (l *List) ReplayDamage(c paint.Canvas, damage geom.Rect, m Measurer) {
	transformDepth := 0
	for i := range l.ops {
		o := &l.ops[i]
		switch o.kind {
		case opPushTransform:
			transformDepth++
			o.replay(c)
		case opPopTransform:
			transformDepth--
			o.replay(c)
		case opPushClip, opPushClipRRect, opPopClip, opClear, opPushOpacity, opPopOpacity:
			o.replay(c)
		default:
			// opBounds is in record space; while a transform is active it does
			// not map to the surface-space damage rect, so culling by it could
			// drop content the transform brings on-surface. Only cull at
			// transform depth 0; the caller's clip still bounds pixels.
			if transformDepth == 0 && opBounds(o, m).Intersect(damage).IsEmpty() {
				continue
			}
			o.replay(c)
		}
	}
}

// opEqual compares two ops field-by-field for their kind. Images compare by
// the identity key captured at record time (see imageKey), never by interface
// equality — so non-comparable image types can't panic the diff.
func opEqual(a, b *op) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case opClear:
		return a.col == b.col
	case opFillRect:
		return a.r == b.r && a.col == b.col
	case opFillRRect:
		return a.r == b.r && a.f1 == b.f1 && a.col == b.col
	case opFillRRectGradient:
		return a.r == b.r && a.f1 == b.f1 && a.col == b.col && a.col2 == b.col2 && a.horiz == b.horiz
	case opStrokeRRect:
		return a.r == b.r && a.f1 == b.f1 && a.f2 == b.f2 && a.col == b.col
	case opLine:
		return a.r == b.r && a.f1 == b.f1 && a.col == b.col
	case opFillPath:
		return a.path == b.path && a.gen == b.gen && a.col == b.col
	case opStrokePath:
		return a.path == b.path && a.gen == b.gen && a.f2 == b.f2 && a.col == b.col
	case opText:
		return a.str1 == b.str1 && a.str2 == b.str2 && a.r.Min == b.r.Min && a.f1 == b.f1 && a.col == b.col
	case opImage:
		return a.imgKey == b.imgKey && a.r == b.r
	case opSprite:
		return a.imgKey == b.imgKey && a.sprite == b.sprite
	case opPushClip:
		return a.r == b.r
	case opPushClipRRect:
		return a.r == b.r && a.f1 == b.f1
	case opPopClip, opPopOpacity, opPopTransform:
		return true
	case opPushOpacity:
		return a.f1 == b.f1
	case opPushTransform:
		return a.xform == b.xform
	case opBackdropBlur:
		return a.r == b.r && a.f1 == b.f1
	}
	return false
}

func opBounds(o *op, m Measurer) geom.Rect {
	switch o.kind {
	case opBackdropBlur, opFillRect, opFillRRect, opFillRRectGradient, opImage:
		return o.r
	case opStrokeRRect:
		return inflate(o.r, o.f2)
	case opLine:
		a, b := o.r.Min, o.r.Max // unnormalized endpoints
		r := geom.Rect{
			Min: geom.Pt{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
			Max: geom.Pt{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
		}
		return inflate(r, o.f1)
	case opSprite:
		return o.sprite.Dst
	case opFillPath:
		return inflate(o.r, 1) // record-time bounds; +1px for fill AA
	case opStrokePath:
		return inflate(o.r, o.f2) // record-time bounds; half-width + AA
	case opText:
		w := m.MeasureWidthIn(o.str1, o.str2, o.f1)
		mt := m.MetricsIn(o.str1, o.f1)
		pos := o.r.Min
		return geom.Rect{
			Min: geom.Pt{X: pos.X, Y: pos.Y - mt.Ascent},
			Max: geom.Pt{X: pos.X + w, Y: pos.Y + mt.Descent},
		}
	}
	return geom.Rect{}
}

// inflate grows r by half the stroke width plus an AA pixel on every side.
func inflate(r geom.Rect, strokeWidth float32) geom.Rect {
	g := strokeWidth/2 + 1
	return geom.Rect{
		Min: geom.Pt{X: r.Min.X - g, Y: r.Min.Y - g},
		Max: geom.Pt{X: r.Max.X + g, Y: r.Max.Y + g},
	}
}
