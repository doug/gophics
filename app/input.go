package app

import (
	"slices"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Pointer, keyboard and gesture dispatch: how a shell event becomes a tap,
// drag, focus change or key delivered to the widget tree.

// longPressPending reports whether a time-based gesture (long-press or a
// deferred single-tap) is running — the shell keeps frames coming while it
// is so the timers advance.
func (c *core) longPressPending() bool {
	return (c.longPress != nil && !c.moved && !c.longFired) || c.pendingTap != nil
}

// TickGestures advances time-based gestures by dt seconds: fires OnLongPress
// for a held unmoved press, and flushes a deferred single-tap once the
// double-tap window elapses.
func (c *core) TickGestures(dt float64) {
	if c.longPress != nil && !c.moved && !c.longFired {
		c.pressHeld += dt
		if c.pressHeld >= c.longPressSeconds() {
			c.longFired = true
			c.pressed = nil // long-press consumes the gesture; no tap
			if h := c.longPress.GestureHandler(); h.OnLongPress != nil {
				h.OnLongPress()
			}
		}
	}
	if c.pendingTap != nil {
		c.tapElapsed += dt
		if c.tapElapsed >= c.doubleTapWindow() {
			tap := c.pendingTap
			c.pendingTap = nil
			if h := tap.GestureHandler(); h.OnTap != nil {
				h.OnTap()
			}
		}
	}
}

// doubleTapSlop is how far (logical px) the second tap may land from the first
// and still count as a double-tap — a double-click is at ~one spot, not across
// the widget.
const doubleTapSlop = 8

// fireTap handles a completed tap on box at pos: immediate for a plain tap; for
// a double-tap-capable box, completes a double only if the second tap is near
// the first, else defers the single.
func (c *core) fireTap(box widget.GestureTarget, pos geom.Pt) {
	h := box.GestureHandler()
	if h.OnDoubleTap == nil {
		if h.OnTap != nil {
			h.OnTap()
		}
		return
	}
	if c.pendingTap == box && near(pos, c.pendingTapPos, doubleTapSlop) {
		c.pendingTap = nil // second tap in window and place: it's a double
		h.OnDoubleTap()
		return
	}
	// A new first-tap that isn't completing the pending one as a double (a
	// different box, or the same box too far away): flush the still-pending tap's
	// OnTap now so it isn't silently dropped when we overwrite it below.
	if c.pendingTap != nil {
		if ph := c.pendingTap.GestureHandler(); ph.OnTap != nil {
			ph.OnTap()
		}
	}
	c.pendingTap, c.pendingTapPos, c.tapElapsed = box, pos, 0 // first tap: defer OnTap
}

func near(a, b geom.Pt, slop float32) bool {
	dx, dy := a.X-b.X, a.Y-b.Y
	return dx*dx+dy*dy <= slop*slop
}

// hitInteractive pairs a gesture target with the hit position in its
// local coordinates.
type hitInteractive struct {
	box   widget.GestureTarget
	local geom.Pt
}

// Semantics returns the semantics tree of the current layout (a11y
// foundation). Call after a frame (or Headless.Render).
func (c *core) Semantics() []layout.SemNode {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	return layout.CollectSemantics(box)
}

// interactivesAt returns the gesture targets under p, topmost first.
// Pending rebuilds are flushed AND laid out first: hit geometry (child
// offsets, sizes) is only valid after layout, and events can arrive
// between a state change and its frame. When nothing is pending and the
// size is unchanged the layout pass is skipped outright — a pointer move
// over a clean tree costs a hit test, not a layout walk. The returned
// slice is scratch reused across calls; callers must not retain it.
func (c *core) interactivesAt(p geom.Pt) []hitInteractive {
	needsLayout := c.Owner.NeedsBuild() // check before RootBox flushes builds
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	if !c.size.IsEmpty() && (needsLayout || c.size != c.lastLayout) {
		box.Layout(layout.Tight(c.size))
		c.lastLayout = c.size
	}
	c.hits = c.hits[:0]
	for _, h := range layout.HitTest(box, p) {
		if gt, ok := h.Box.(widget.GestureTarget); ok {
			c.hits = append(c.hits, hitInteractive{gt, h.Pos})
		}
	}
	return c.hits
}

// Pointer dispatches a pointer event: hover enter/exit, drag, scroll,
// tap on press+release over the same Interactive, and tap-to-focus.
func (c *core) Pointer(e shell.Pointer) {
	if c.Owner.Input != nil {
		c.Owner.Input.HandlePointer(e)
	}
	switch e.Kind {
	case shell.PointerMove:
		delta := e.Pos.Sub(c.lastPos)
		c.lastPos = e.Pos
		if c.inspect {
			c.Owner.RequestFrameThreadSafe() // repaint so the highlight tracks
		}
		// Slop detection runs for any active press, so a move cancels a
		// pending tap or long-press even on a widget with no drag handler.
		if !c.moved && (c.pressed != nil || c.longPress != nil || len(c.dragCandidates) > 0) {
			d := e.Pos.Sub(c.downPos)
			if sl := c.slop(); d.X*d.X+d.Y*d.Y > sl*sl {
				c.moved = true
				c.pressed = nil
				c.longPress = nil
				// A priority candidate (e.g. a text selection whose press landed
				// on text) grabs the drag regardless of depth or axis, so it
				// beats a deeper scroll. Otherwise commit to the deepest
				// candidate whose axis matches the drag's dominant direction (an
				// unconstrained one always matches), so nested
				// horizontal/vertical drags disambiguate.
				for _, h := range c.dragCandidates {
					if dp := h.box.GestureHandler().DragPriority; dp != nil && dp(c.downTouch) {
						c.dragging = h.box
						c.dragOrigin = c.downPos.Sub(h.local)
						break
					}
				}
				if c.dragging == nil {
					for _, h := range c.dragCandidates {
						hd := h.box.GestureHandler()
						// A handler that does not want this gesture steps
						// aside rather than winning it on depth and then
						// ignoring it (see Handler.DragClaims).
						if dc := hd.DragClaims; dc != nil && !dc(c.downTouch) {
							continue
						}
						if hd.DragAxis.Accepts(d.X, d.Y) {
							c.dragging = h.box
							// h.local is the box-local point at press, so the box
							// origin comes from downPos (not the current move pos).
							c.dragOrigin = c.downPos.Sub(h.local)
							break
						}
					}
				}
				c.dragCandidates = c.dragCandidates[:0]
				// The press became a drag or a scroll: end any highlight it
				// put up — except on the box that won the drag, whose press
				// has not ended at all. It is now dragging and will get
				// OnRelease; telling it the press ended here would have it
				// tear down the gesture on the very move that started it. It
				// stays on the list so it still hears about pointer-up.
				c.firePressEndExcept(c.dragging)
			}
		}
		if c.moved && c.dragging != nil {
			if h := c.dragging.GestureHandler(); h.OnDrag != nil {
				// Local position via the press-time origin: drags keep
				// delivering even when the pointer leaves the box.
				h.OnDrag(e.Pos.Sub(c.dragOrigin), delta)
			}
		}
		// Build the new hover set in the scratch buffer (never aliasing
		// c.hovered — the two swap each event), diff, then swap.
		now := c.hoverScratch[:0]
		for _, h := range c.interactivesAt(e.Pos) {
			now = append(now, h.box)
		}
		for _, b := range c.hovered {
			if h := b.GestureHandler(); !slices.Contains(now, b) && h.OnExit != nil {
				h.OnExit()
			}
		}
		for _, b := range now {
			if h := b.GestureHandler(); !slices.Contains(c.hovered, b) && h.OnEnter != nil {
				h.OnEnter()
			}
		}
		c.hovered, c.hoverScratch = now, c.hovered

	case shell.PointerScroll:
		// Route by where the pointer is *now*. Every shell sets Pos on a
		// scroll; using the last position a *move* happened to report meant a
		// wheel that arrived without a preceding move went to whatever was
		// under the stale position, which is usually nothing. Keeping lastPos
		// in step also means a drag starting right after a scroll begins from
		// the right place.
		c.lastPos = e.Pos
		for _, h := range c.interactivesAt(e.Pos) {
			if hd := h.box.GestureHandler(); hd.OnScroll != nil {
				hd.OnScroll(e.Scroll)
				return
			}
		}

	case shell.PointerDown:
		if e.Button != 0 {
			return
		}
		c.downPos, c.lastPos, c.moved = e.Pos, e.Pos, false
		c.downTouch = e.Source == shell.SourceTouch
		c.pressed, c.dragging, c.longPress = nil, nil, nil
		c.dragCandidates = c.dragCandidates[:0]
		c.pressBoxes = c.pressBoxes[:0]
		c.pressHeld, c.longFired = 0, false
		hits := c.interactivesAt(e.Pos)
		for _, h := range hits {
			hd := h.box.GestureHandler()
			if hd.OnPress != nil {
				hd.OnPress(h.local)
			}
			if hd.OnPressEnd != nil {
				c.pressBoxes = append(c.pressBoxes, h.box)
			}
			if c.pressed == nil && (hd.OnTap != nil || hd.OnDoubleTap != nil) {
				c.pressed = h.box
			}
			if c.longPress == nil && hd.OnLongPress != nil {
				c.longPress = h.box
			}
			// Defer drag commitment: collect every candidate (deepest first)
			// and pick one by direction on the first slop-crossing move, so a
			// horizontal Dismissible and a vertical Scroll can nest.
			if hd.OnDrag != nil {
				c.dragCandidates = append(c.dragCandidates, h)
			}
		}
		c.focusFrom(hits)

	case shell.PointerUp:
		if e.Button != 0 {
			return
		}
		pressed, dragging := c.pressed, c.dragging
		c.pressed, c.dragging, c.longPress = nil, nil, nil
		c.dragCandidates = c.dragCandidates[:0]
		if dragging != nil {
			if h := dragging.GestureHandler(); h.OnRelease != nil {
				h.OnRelease()
			}
		}
		if pressed != nil {
			for _, h := range c.interactivesAt(e.Pos) {
				if h.box == pressed {
					c.fireTap(pressed, e.Pos)
					break
				}
			}
		}
		c.firePressEnd() // end any press highlight, tapped or not
	}
}

// firePressEnd notifies every box that received OnPress this gesture that the
// press has concluded (up, cancel, or a drag steal), then clears the list. It
// is idempotent: the second call in a gesture finds an empty list.
func (c *core) firePressEnd() { c.firePressEndExcept(nil) }

// firePressEndExcept is firePressEnd with one box held back — the one that has
// just taken the drag, which is retained for the end of the gesture rather
// than notified now.
func (c *core) firePressEndExcept(skip widget.GestureTarget) {
	kept := c.pressBoxes[:0]
	for _, b := range c.pressBoxes {
		if skip != nil && b == skip {
			kept = append(kept, b) // still owed an OnPressEnd at pointer-up
			continue
		}
		if h := b.GestureHandler(); h.OnPressEnd != nil {
			h.OnPressEnd()
		}
	}
	c.pressBoxes = kept
}

// focusFrom moves keyboard focus to the topmost focusable hit.
//
// A press that hits nothing focusable releases a *text* target, and keeps any
// other. That asymmetry is deliberate. On a phone the soft keyboard is raised
// by a focused field and dismissed by it losing focus, so a rule that never
// released focus left no way to put the keyboard away at all: it covered half
// the screen until the app was closed. Tapping outside a field to dismiss is
// also what both the web and the platforms do.
//
// It is narrowed to targets that capture text (OnText) so that a widget which
// only reads keys — the edit menu, a key-driven canvas — keeps focus when the
// user presses elsewhere. Nothing about those involves a keyboard that needs
// dismissing.
func (c *core) focusFrom(hits []hitInteractive) {
	for _, hit := range hits {
		h := hit.box.GestureHandler()
		if h.OnText == nil && h.OnKey == nil {
			continue
		}
		old := c.Owner.KeyboardTarget
		if old == h {
			return
		}
		c.Owner.KeyboardTarget = h
		if old != nil && old.OnFocus != nil {
			old.OnFocus(false)
		}
		if h.OnFocus != nil {
			h.OnFocus(true)
		}
		return
	}

	// Nothing focusable under the press.
	if old := c.Owner.KeyboardTarget; old != nil && old.OnText != nil {
		c.Owner.KeyboardTarget = nil
		if old.OnFocus != nil {
			old.OnFocus(false)
		}
	}
}

// Keyboard dispatches key/text events to the current keyboard target.
func (c *core) Keyboard(e shell.Event) {
	// Feed held-state polling first, before the focus early-return — a game
	// canvas polls keys with no focused widget.
	if in := c.Owner.Input; in != nil {
		if k, ok := e.(shell.Key); ok {
			in.HandleKey(k)
		}
		t := c.Owner.KeyboardTarget
		in.SetTextCapturing(t != nil && t.OnText != nil)
	}
	t := c.Owner.KeyboardTarget

	// Tab moves focus, before the focused widget sees it.
	//
	// Ahead of the dispatch because traversal is the app's business, not any
	// one widget's: a field cannot know what comes after it. A widget that
	// wants Tab for itself says so with ConsumesTab — a multiline field
	// indents — and keeps it. With nothing focused Tab still moves, so Tab
	// into a screen works like Tab within one.
	if k, ok := e.(shell.Key); ok && k.Code == shell.KeyTab && k.Kind == shell.KeyPress {
		if t == nil || t.ConsumesTab == nil || !t.ConsumesTab() {
			if c.Owner.MoveFocus(k.Mods&shell.ModShift == 0) {
				c.Owner.RebuildAll()
			}
			return
		}
	}

	if t == nil {
		return
	}
	switch e := e.(type) {
	case shell.Text:
		if t.OnText != nil {
			t.OnText(e.S)
		}
	case shell.Key:
		if t.OnKey != nil {
			t.OnKey(e)
		}
	case shell.Composition:
		if t.OnComposition != nil {
			t.OnComposition(e)
		}
	}
}
