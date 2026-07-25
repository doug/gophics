package scene

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
)

// Measurer supplies text extents for damage computation; paint.Painter
// implements it.
type Measurer interface {
	MeasureWidth(s string, size float32) float32
	Metrics(size float32) paint.TextMetrics
}

// Diff compares l against prev and returns the damage: the union of the
// bounds of every op that changed, was added, or was removed, plus whether
// anything changed at all. Equal lists return (zero, false).
//
// The comparison is positional over the longest common prefix and suffix —
// cheap and tight for localized changes (the common case: one widget
// repainted). Ops between the prefix and suffix contribute their bounds
// from both lists. Any difference in clip structure inside the changed
// window falls back to unbounded damage (the caller clamps to the surface).
func (l *List) Diff(prev *List, m Measurer) (geom.Rect, bool) {
	a, b := prev.ops, l.ops

	// Common prefix.
	i := 0
	for i < len(a) && i < len(b) && opEqual(a[i], b[i]) {
		i++
	}
	if i == len(a) && i == len(b) {
		return geom.Rect{}, false
	}
	// Common suffix (not overlapping the prefix).
	ja, jb := len(a), len(b)
	for ja > i && jb > i && opEqual(a[ja-1], b[jb-1]) {
		ja--
		jb--
	}

	var damage geom.Rect
	unbounded := false
	add := func(ops []op) {
		for _, o := range ops {
			switch o.(type) {
			case clearOp, pushClipOp, popClipOp:
				// Structural change: damage everything.
				unbounded = true
			default:
				damage = damage.Union(opBounds(o, m))
			}
		}
	}
	add(a[i:ja])
	add(b[i:jb])
	if unbounded {
		return geom.Rect{Max: geom.Pt{X: layoutInf, Y: layoutInf}}, true
	}
	return damage, true
}

var layoutInf = float32(1 << 30)

// ReplayDamage replays only the ops intersecting damage onto c. Clip
// structure is always preserved; other ops are culled by bounds. The caller
// is expected to clip c to damage first — culling saves command execution
// (text shaping, path setup), clipping guarantees pixel correctness.
func (l *List) ReplayDamage(c paint.Canvas, damage geom.Rect, m Measurer) {
	for _, o := range l.ops {
		switch o.(type) {
		case pushClipOp, popClipOp, clearOp:
			o.replay(c)
		default:
			if opBounds(o, m).Intersect(damage).IsEmpty() {
				continue
			}
			o.replay(c)
		}
	}
}

func opEqual(a, b op) bool { return a == b }

func opBounds(o op, m Measurer) geom.Rect {
	switch o := o.(type) {
	case rectOp:
		return o.r
	case rrectOp:
		return o.r
	case rrectGradientOp:
		return o.r
	case strokeRRectOp:
		return inflate(o.r, o.width)
	case lineOp:
		r := geom.Rect{
			Min: geom.Pt{X: min32(o.a.X, o.b.X), Y: min32(o.a.Y, o.b.Y)},
			Max: geom.Pt{X: max32(o.a.X, o.b.X), Y: max32(o.a.Y, o.b.Y)},
		}
		return inflate(r, o.width)
	case textOp:
		w := m.MeasureWidth(o.s, o.size)
		mt := m.Metrics(o.size)
		return geom.Rect{
			Min: geom.Pt{X: o.pos.X, Y: o.pos.Y - mt.Ascent},
			Max: geom.Pt{X: o.pos.X + w, Y: o.pos.Y + mt.Descent},
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

func min32(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func max32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}
