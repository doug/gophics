package scene

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/paint"
)

// Measurer supplies text extents for damage computation; paint.Painter
// implements it.
//
// It is an alias, not a second declaration: the same three methods are now
// part of paint.Canvas (so a custom painter can measure what it draws), and
// two identical interfaces would drift.
type Measurer = paint.Measurer

// Diff compares l against prev and returns the damage: the union of the
// bounds of every op that changed, was added, or was removed, plus whether
// anything changed at all. Equal lists return (zero, false).
//
// The comparison is positional over the longest common prefix and suffix —
// cheap and tight for localized changes (the common case: one widget
// repainted). Ops between the prefix and suffix contribute their bounds
// from both lists. A changed opacity or blend push/pop damages its whole
// group's content bounds (both lists), so a fading widget repaints only its
// rect — and a widget that changes blend mode repaints everything that mode
// governs, which is the part the bounds of the boundary op alone would miss.
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

	// A backdrop blur later in the frame reads what was painted under it, so
	// a change beneath one reaches further than its own bounds: the blur
	// spreads it by the kernel's reach, and a partial replay would blur the
	// stale, already-tinted pixels outside the damage clip back into the
	// panel. Any blur whose sampling footprint (its rect grown by that reach)
	// a change touches is repainted whole. Blurs in the common prefix precede
	// every change and cannot be affected.
	blurs := blursFrom(b, i)

	var damage geom.Rect
	unbounded := false
	add := func(ops []op, lo, hi int, pos func(k int) int) {
		for k := lo; k < hi; k++ {
			var r geom.Rect
			switch ops[k].kind {
			case opClear, opPushClip, opPushClipRRect, opPopClip,
				opPushTransform, opPopTransform:
				// Structural change: damage everything.
				unbounded = true
				continue
			case opPushOpacity:
				// A changed/added/removed group boundary: the whole group's
				// content is (re)composited, so damage its content bounds.
				r = groupBounds(ops, k, +1, opPushOpacity, opPopOpacity, m, &unbounded)
			case opPopOpacity:
				r = groupBounds(ops, k, -1, opPushOpacity, opPopOpacity, m, &unbounded)
			case opPushBlend, opPopBlend:
				// A blend boundary paints nothing of its own, so its bounds
				// are empty — and taking them literally is the trap this
				// case exists to avoid. A mode change leaves every shape
				// inside the group byte-identical, so the positional diff
				// matches them all in its common suffix and the only op in
				// the changed window is the boundary itself: empty damage,
				// and the frame keeps the shapes it painted under the old
				// mode. The fix is the one opacity already uses — damage the
				// group's content, because changing the mode changes what
				// all of it paints.
				dir := +1
				if ops[k].kind == opPopBlend {
					dir = -1
				}
				r = groupBounds(ops, k, dir, opPushBlend, opPopBlend, m, &unbounded)
			default:
				r = opBounds(&ops[k], m)
			}
			damage = damage.Union(r)
			blurs.touch(r, pos(k))
		}
	}
	// A removed op has no position in b; it precedes everything after the
	// common prefix, so every blur there may have sampled it.
	add(a, i, ja, func(int) int { return i - 1 })
	add(b, i, jb, func(k int) int { return k })
	if unbounded {
		return geom.Rect{Max: geom.Pt{X: layoutInf, Y: layoutInf}}, true
	}
	return blurs.extend(damage), true
}

var layoutInf = float32(1 << 30)

// blurSet is the backdrop blurs a diff has to account for, in list order.
type blurSet []blurRef

type blurRef struct {
	idx       int       // position in the current list
	rect      geom.Rect // the blurred panel
	footprint geom.Rect // rect grown by the kernel's reach: what the blur samples
	dirty     bool      // something it samples changed
}

// blursFrom collects the backdrop blurs in ops at index from onwards.
func blursFrom(ops []op, from int) blurSet {
	var s blurSet
	for k := from; k < len(ops); k++ {
		if o := &ops[k]; o.kind == opBackdropBlur {
			// The CPU blur is several box passes of the radius, so a panel
			// pixel depends on backdrop well beyond one radius; growing by
			// the radius alone left a change between one and three radii
			// out repainted on its own, with the panel keeping a blur of the
			// old colour. The reach comes from gg so the two cannot drift.
			g := float32(gg.BackdropBlurReach(float64(o.f1))) + 1
			s = append(s, blurRef{idx: k, rect: o.r, footprint: geom.Rect{
				Min: geom.Pt{X: o.r.Min.X - g, Y: o.r.Min.Y - g},
				Max: geom.Pt{X: o.r.Max.X + g, Y: o.r.Max.Y + g},
			}})
		}
	}
	return s
}

// touch marks every blur painted after position pos whose footprint the
// changed rect r overlaps.
func (s blurSet) touch(r geom.Rect, pos int) {
	for k := range s {
		if s[k].idx > pos && !s[k].footprint.Intersect(r).IsEmpty() {
			s[k].dirty = true
		}
	}
}

// extend adds the panel of every dirty blur to damage. A repainted panel is
// itself a change that a later blur may sample, so the walk is in list order
// and each repainted panel touches the blurs after it.
func (s blurSet) extend(damage geom.Rect) geom.Rect {
	for k := range s {
		if !s[k].dirty {
			continue
		}
		damage = damage.Union(s[k].rect)
		s[k+1:].touch(s[k].rect, s[k].idx)
	}
	return damage
}

// groupBounds returns the union of the content bounds of the group whose push
// (dir=+1) or pop (dir=-1) sits at ops[k], scanning toward the matching
// partner. push/pop name the pair that delimits it — opacity groups and blend
// groups are both measured this way, and nesting is counted only in the kind
// asked about, so an opacity group containing a blend group (or the reverse)
// still finds its own partner.
//
// Bounds are valid because both group kinds record in surface coordinates.
// Clip ops inside the group are skipped — clips only shrink content, so the
// un-clipped union is a conservative superset. A transform or clear inside
// the group, or an unbalanced group, sets *unbounded instead (the transform's
// content bounds are in another space).
func groupBounds(ops []op, k, dir int, push, pop opKind, m Measurer, unbounded *bool) geom.Rect {
	var b geom.Rect
	depth := 1
	// Cases test o.kind rather than switching on it: push and pop are
	// parameters here, and a switch on o.kind would need them as case
	// expressions beside the fixed kinds below, which overlap them.
	for j := k + dir; j >= 0 && j < len(ops); j += dir {
		o := &ops[j]
		switch {
		case o.kind == push:
			depth += dir
		case o.kind == pop:
			depth -= dir
		case o.kind == opPushTransform, o.kind == opPopTransform, o.kind == opClear:
			*unbounded = true
			return geom.Rect{}
		case o.kind == opPushClip, o.kind == opPushClipRRect, o.kind == opPopClip:
			// Clips only shrink; ignoring them keeps a superset.
		case o.kind == opPushOpacity, o.kind == opPopOpacity,
			o.kind == opPushBlend, o.kind == opPopBlend:
			// The other group kind's boundary. It paints nothing itself and
			// its own content is visited op by op here anyway, so it
			// contributes no bounds — and must not be sent back through
			// groupBounds, which would rescan the same ops.
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
// opacity, blend, and transform structure is always preserved; other ops are
// culled by bounds. The caller is expected to clip c to damage first — culling
// saves command execution (text shaping, path setup), clipping guarantees
// pixel correctness (including opacity groups: draws into the layer honor the
// clip, and the layer composites source-over, so pixels outside damage are
// untouched).
//
// # Blend modes and partial replay
//
// A blend mode reads the destination, so it is fair to ask whether a damaged
// replay can reproduce a full repaint at all. It can, for every mode, and the
// reason is worth writing down because the answer for opBackdropBlur — the
// other destination-reading op — is the opposite.
//
// Every blend mode is a pure per-pixel function of (source, destination) at
// one pixel: no mode samples a neighbourhood. So clipping the replay to the
// damage rect does not change what any op does at a pixel *inside* the rect —
// the clip only removes coverage outside it — and the ops that were culled
// paint nothing inside it by definition of their bounds. Both paths start
// from the same retained surface and apply the same sequence, so inside
// damage the pixels are identical; outside it the retained pixels are correct
// because Diff puts every changed pixel inside. A backdrop blur breaks that
// argument only because its footprint is wider than its own bounds, which is
// what blursFrom has to compensate for; blending has no such reach and needs
// no widening.
//
// The three things that do have to hold are all here or in Diff: push/pop
// blend are never culled (below), so the stack stays balanced however much of
// the list is skipped; a changed mode damages the group's whole content (see
// Diff); and an op whose destination changed is itself replayed, because the
// change is inside damage and so the op's bounds intersect it.
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
		case opPushClip, opPushClipRRect, opPopClip, opClear,
			opPushOpacity, opPopOpacity, opPushBlend, opPopBlend:
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
	case opFillGradient:
		// By identity, like a path or a mark batch — but for a stronger
		// reason than either. A Gradient is immutable once built, so an equal
		// pointer *is* an equal ramp: unlike a retained *Path, it cannot have
		// been mutated behind the op, which is why no generation counter and
		// no recorded-bounds fallback are needed here. A caller that rebuilds
		// its gradient every frame gets a new pointer and a repaint, which is
		// the price of not keeping one; a caller that keeps one gets the
		// no-change answer it should.
		return a.grad == b.grad && a.r == b.r && a.f1 == b.f1
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
	case opMarks:
		// By identity, like a path: a batch is thousands of coordinates, and
		// re-scanning them every frame to prove nothing moved would cost more
		// than redrawing. The bounds go in too, so a batch mutated in place
		// still repaints when its extent changes.
		return a.marks == b.marks && a.r == b.r
	case opPushClip:
		return a.r == b.r
	case opPushClipRRect:
		return a.r == b.r && a.f1 == b.f1
	case opPopClip, opPopOpacity, opPopTransform, opPopBlend:
		return true
	case opPushOpacity:
		return a.f1 == b.f1
	case opPushBlend:
		// The mode is part of the op, so two frames that differ only in it
		// diff as changed. It has to be: a blend mode decides what every
		// shape in the group paints, so the same shapes under a different
		// mode are a different picture, not an interchangeable one.
		return a.blend == b.blend
	case opPushTransform:
		return a.xform == b.xform
	case opBackdropBlur:
		return a.r == b.r && a.f1 == b.f1
	}
	return false
}

func opBounds(o *op, m Measurer) geom.Rect {
	switch o.kind {
	case opBackdropBlur, opFillRect, opFillRRect, opFillRRectGradient,
		opFillGradient, opImage:
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
		return spriteBounds(o.sprite)
	case opMarks:
		return inflate(o.r, 1) // recorded bounds; +1px for edge AA
	case opFillPath:
		return inflate(o.r, 1) // record-time bounds; +1px for fill AA
	case opStrokePath:
		return inflate(o.r, o.f2) // record-time bounds; half-width + AA
	case opText:
		w := m.MeasureWidthIn(o.str1, o.str2, o.f1)
		mt := m.MetricsIn(o.str1, o.f1)
		pos := o.r.Min
		r := geom.Rect{
			Min: geom.Pt{X: pos.X, Y: pos.Y - mt.Ascent},
			Max: geom.Pt{X: pos.X + w, Y: pos.Y + mt.Descent},
		}
		// The metrics box is where the text is laid out; the ink can leave it
		// (accents above the ascender, italic overhangs, a taller fallback
		// font), and the run cache blits the ink box with a 2px pad, so damage
		// covers that or the overhang is left stale.
		if ink, ok := m.InkBoundsIn(o.str1, o.str2, o.f1); ok {
			r = r.Union(ink.Translate(pos))
		}
		return inflate(r, 2)
	}
	return geom.Rect{}
}

// spriteBounds is where a sprite paints. DrawSprite rotates about Dst's
// centre, so a rotated sprite's corners lie outside Dst: its bounds are the
// axis-aligned box of the rotated rectangle, plus an AA pixel. Damage taken
// from Dst alone left the corners stale on a partial replay, and ReplayDamage
// could cull the sprite entirely.
func spriteBounds(s paint.Sprite) geom.Rect {
	if s.Rotation == 0 {
		return s.Dst
	}
	c := float32(math.Abs(math.Cos(float64(s.Rotation))))
	sn := float32(math.Abs(math.Sin(float64(s.Rotation))))
	hw, hh := s.Dst.Dx()/2, s.Dst.Dy()/2
	rw, rh := hw*c+hh*sn, hw*sn+hh*c
	cx, cy := s.Dst.Min.X+hw, s.Dst.Min.Y+hh
	return inflate(geom.Rect{
		Min: geom.Pt{X: cx - rw, Y: cy - rh},
		Max: geom.Pt{X: cx + rw, Y: cy + rh},
	}, 0)
}

// inflate grows r by half the stroke width plus an AA pixel on every side.
func inflate(r geom.Rect, strokeWidth float32) geom.Rect {
	g := strokeWidth/2 + 1
	return geom.Rect{
		Min: geom.Pt{X: r.Min.X - g, Y: r.Min.Y - g},
		Max: geom.Pt{X: r.Max.X + g, Y: r.Max.Y + g},
	}
}
