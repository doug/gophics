package widget

// Keyboard focus traversal: Tab moves to the next focusable widget, Shift-Tab
// to the previous.
//
// The order is the order the widgets were built in, taken by walking the
// element tree depth-first. That is the tree the app author wrote, so it is
// also the order they read the form in — the same reasoning that makes DOM
// order the right answer on the web. Deriving it from geometry instead would
// have to invent a reading direction and would disagree with the source
// wherever a layout reorders visually.

// focusables collects every focusable handler in build order.
//
// The handlers are collected by pointer, and the pointer is what focus is
// tracked by: a rebuild overwrites an InteractiveBox's Gestures in place, so
// the address survives while the closures inside it do not. Collecting values
// would compare unequal against KeyboardTarget on the very next frame.
func (o *Owner) focusables() []*Gestures {
	var out []*Gestures
	var walk func(e *element)
	walk = func(e *element) {
		if e == nil {
			return
		}
		if ib, ok := e.box.(*InteractiveBox); ok && ib.Gestures.focusable() {
			out = append(out, &ib.Gestures)
		}
		walk(e.child)
		for _, k := range e.kids {
			walk(k)
		}
	}
	walk(o.root)
	return out
}

// MoveFocus moves keyboard focus to the next focusable widget in build order,
// or the previous one when forward is false, and reports whether it moved.
//
// Wraps at both ends. A form is a cycle rather than a line: Tab off the last
// field returning to the first is what every platform does, and stopping dead
// there leaves the user pressing a key that does nothing with no indication
// why.
//
// With nothing focused it takes the first (or last, going backwards), so Tab
// into a screen works the way Tab within one does.
func (o *Owner) MoveFocus(forward bool) bool {
	list := o.focusables()
	if len(list) == 0 {
		return false
	}

	next := 0
	if !forward {
		next = len(list) - 1
	}
	if cur := o.KeyboardTarget; cur != nil {
		at := -1
		for i, g := range list {
			if g == cur {
				at = i
				break
			}
		}
		if at >= 0 {
			if forward {
				next = (at + 1) % len(list)
			} else {
				next = (at - 1 + len(list)) % len(list)
			}
		}
	}

	target := list[next]
	if target == o.KeyboardTarget {
		return false // only one focusable; nothing to move to
	}

	old := o.KeyboardTarget
	o.KeyboardTarget = target
	// Same order the pointer path uses: the field being left hears first, so a
	// soft keyboard is handed over rather than dropped and re-raised.
	if old != nil && old.OnFocus != nil {
		old.OnFocus(false)
	}
	if target.OnFocus != nil {
		target.OnFocus(true)
	}
	return true
}
