package apptest

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// Settle steps simulated frames until nothing is animating, so a test can
// write
//
//	a.TapLabel("Open")
//	a.Settle()
//	a.TapLabel("Back")
//
// instead of stepping a guessed number of frames after every navigation. A
// tap during a page slide misses because the target is not there yet; this
// is the test-side answer.
//
// Bounded at five seconds of simulated time. Running out is a test failure
// rather than a silent return: a ticker that never stops (a refresh spinner
// held open, a drag never released) is a fact the test should learn, and a
// test that wants such a state should step frames explicitly.
func (a *App) Settle() {
	a.tb.Helper()
	const dt = 1.0 / 60
	const limit = 5.0
	// Render first: a tap that pushed a page has only marked the tree dirty,
	// and the transition's ticker does not exist until the rebuild runs.
	// Stepping before it would find nothing animating and return at once.
	a.Render()
	for elapsed := 0.0; elapsed < limit; elapsed += dt {
		active := a.Step(dt)
		a.Render() // a ticker's final tick may still have moved something
		if !active {
			return
		}
	}
	a.tb.Fatalf("apptest: Settle: still animating after %.0fs of simulated time — something ticks forever (a spinner, an unreleased drag?); step frames explicitly for that state", limit)
}

// ScrollTo scrolls the viewport(s) enclosing the node labelled label until
// the node lies inside every one of them, the way a user would scroll a row
// into view before tapping it. Nested viewports — a horizontal strip in a
// vertical page — are handled innermost first. It fails if there is no such
// node, if it is not inside a scrollable viewport at all, or if scrolling
// stops moving it before it is visible.
//
// The scrolling is real input: wheel events dispatched over the viewport, so
// what the test exercises is what the app does.
func (a *App) ScrollTo(label string) {
	a.tb.Helper()
	// Each pass scrolls one viewport by the amount that would reveal the node,
	// then re-reads everything, because moving one viewport moves the rects
	// of all the ones inside it. A pass that changes nothing is the stop.
	for range 64 {
		n := a.MustNode(label)
		vps, all := a.viewportsAround(n)
		if len(vps) == 0 {
			if n.Offscreen {
				a.tb.Fatalf("apptest: ScrollTo %q: the node is off screen (rect %v) but inside no scroll viewport, so nothing can bring it into view", label, n.Rect)
			}
			return
		}
		moved := false
		for j, vp := range vps {
			// Only the part of this viewport that its own ancestors show can
			// take a wheel event; if none of it is showing, an outer pass
			// reveals it first and this pass reaches it next time round.
			visible := vp
			for _, outer := range vps[j+1:] {
				visible = visible.Intersect(outer)
			}
			if visible.IsEmpty() {
				continue
			}
			d := revealDelta(n.Rect, vp)
			if d == (geom.Pt{}) {
				continue
			}
			a.ScrollAt(wheelPoint(visible, vp, all), d)
			moved = true
			break
		}
		if !moved {
			if n.Offscreen {
				a.tb.Fatalf("apptest: ScrollTo %q: inside every enclosing viewport yet still off screen (rect %v) — something other than a scroll clips it", label, n.Rect)
			}
			return
		}
		if after := a.MustNode(label); after.Rect == n.Rect {
			a.tb.Fatalf("apptest: ScrollTo %q: scrolling did not move it (rect %v) — the viewport is at its limit, or the wheel landed on something else", label, n.Rect)
		}
	}
	a.tb.Fatalf("apptest: ScrollTo %q: did not converge", label)
}

// viewportsAround finds the box behind semantics node n and returns the
// root-space rects of the clipping viewports above it, innermost first, along
// with every clipping viewport in the tree (for wheelPoint to steer around).
//
// The semantics tree does not carry boxes, so the match is by what both sides
// compute identically: role and root-space rect, plus the label where the box
// states one (an interactive node's label is usually inherited from its text,
// so a blank box label is not a mismatch).
func (a *App) viewportsAround(n layout.SemNode) (enclosing, all []geom.Rect) {
	root := a.Owner().RootBox()
	if root == nil {
		return nil, nil
	}
	var walk func(b layout.Box, at geom.Pt, clips []geom.Rect) bool
	walk = func(b layout.Box, at geom.Pt, clips []geom.Rect) bool {
		rect := geom.Rect{Min: at, Max: at.Add(b.Size().Pt())}
		if s, ok := b.(layout.Semantic); ok {
			info := s.Semantics()
			if info.Hidden {
				return false
			}
			if info.Role == n.Role && rect == n.Rect && (info.Label == "" || info.Label == n.Label) {
				enclosing = make([]geom.Rect, 0, len(clips))
				for i := len(clips) - 1; i >= 0; i-- {
					enclosing = append(enclosing, clips[i])
				}
				return true
			}
		}
		if c, ok := b.(layout.Clipper); ok && c.ClipsChildren() {
			all = append(all, rect)
			// Full-slice expression: the recursion below must not share the
			// backing array between siblings.
			clips = append(clips[:len(clips):len(clips)], rect)
		}
		found := false
		if v, ok := b.(layout.ChildVisitor); ok {
			v.VisitChildren(func(child layout.Box, off geom.Pt) {
				if !found {
					found = walk(child, at.Add(off), clips)
				}
			})
		}
		return found
	}
	walk(root, geom.Pt{}, nil)
	return enclosing, all
}

// revealDelta is the wheel delta that brings r inside vp: how far the content
// must move on each axis, in the direction content follows a finger (positive
// is down/right). Zero on an axis that already fits. A rect bigger than the
// viewport is aligned by its start edge, the side a reader looks at first.
func revealDelta(r, vp geom.Rect) geom.Pt {
	return geom.Pt{
		X: revealSpan(r.Min.X, r.Max.X, vp.Min.X, vp.Max.X),
		Y: revealSpan(r.Min.Y, r.Max.Y, vp.Min.Y, vp.Max.Y),
	}
}

func revealSpan(lo, hi, vlo, vhi float32) float32 {
	switch {
	case lo < vlo:
		return vlo - lo
	case hi > vhi:
		d := vhi - hi
		if lo+d < vlo {
			d = vlo - lo
		}
		return d
	}
	return 0
}

// wheelPoint picks where inside visible (a part of viewport vp) to dispatch
// the wheel. A scroll event goes to the topmost handler under the pointer and
// stops there, so a horizontal strip lying under the centre of a vertical page
// would swallow the page's wheel; the point is moved off any viewport nested
// inside vp when one is in the way. The centre when nothing is.
func wheelPoint(visible, vp geom.Rect, all []geom.Rect) geom.Pt {
	var nested []geom.Rect
	for _, r := range all {
		if r != vp && !containsRect(r, vp) && r.Overlaps(vp) {
			nested = append(nested, r)
		}
	}
	const inset = 2
	c := centerOf(visible)
	candidates := []geom.Pt{
		c,
		{X: c.X, Y: visible.Min.Y + inset}, {X: c.X, Y: visible.Max.Y - inset},
		{X: visible.Min.X + inset, Y: c.Y}, {X: visible.Max.X - inset, Y: c.Y},
		{X: visible.Min.X + inset, Y: visible.Min.Y + inset}, {X: visible.Max.X - inset, Y: visible.Min.Y + inset},
		{X: visible.Min.X + inset, Y: visible.Max.Y - inset}, {X: visible.Max.X - inset, Y: visible.Max.Y - inset},
	}
	for _, p := range candidates {
		if !visible.Contains(p) {
			continue
		}
		clear := true
		for _, r := range nested {
			if r.Contains(p) {
				clear = false
				break
			}
		}
		if clear {
			return p
		}
	}
	return c
}

func containsRect(outer, inner geom.Rect) bool {
	return outer.Min.X <= inner.Min.X && outer.Min.Y <= inner.Min.Y &&
		outer.Max.X >= inner.Max.X && outer.Max.Y >= inner.Max.Y
}
