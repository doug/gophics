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
//
// Two kinds of subtree are left out of the cycle. A box that hides its
// subtree (focusHider — an offstage Navigator page) is mounted but neither
// painted nor hit-testable, and Tab landing there would put the caret where
// the user cannot see it. And each overlay entry is a focus scope of its own:
// when the topmost entry with any focusable in it is a dialog, Tab cycles
// among the dialog's fields rather than wandering into the page under its
// scrim; an entry with nothing to focus (a snackbar, a tooltip, a menu of
// buttons) does not capture Tab, so the page keeps its cycle.

// focusHider is a render box that keeps its whole subtree out of focus
// traversal while it reports true.
type focusHider interface{ hidesFocus() bool }

// focusScope marks the root of a Tab cycle. It is a stateless wrapper so the
// marker costs an element and no box; the overlay puts one around each entry.
type focusScope struct{ child Widget }

func (f focusScope) Build(Ctx) Widget { return f.child }

// focusables collects the focusable handlers of the topmost scope that has
// any, in build order.
//
// The handlers are collected by pointer, and the pointer is what focus is
// tracked by: a rebuild overwrites an InteractiveBox's Gestures in place, so
// the address survives while the closures inside it do not. Collecting values
// would compare unequal against KeyboardTarget on the very next frame.
func (o *Owner) focusables() []*Gestures {
	// scopes[0] is the tree outside any overlay entry; each focusScope met on
	// the walk opens another. They are appended in build order, and later
	// overlay entries are built after earlier ones, so the last non-empty
	// scope is the topmost entry with something to focus.
	scopes := [][]*Gestures{nil}
	var walk func(e *element, scope int)
	walk = func(e *element, scope int) {
		if e == nil {
			return
		}
		if h, ok := e.box.(focusHider); ok && h.hidesFocus() {
			return
		}
		if _, ok := e.widget.(focusScope); ok {
			scopes = append(scopes, nil)
			scope = len(scopes) - 1
		}
		if ib, ok := e.box.(*InteractiveBox); ok && ib.Gestures.focusable() {
			scopes[scope] = append(scopes[scope], &ib.Gestures)
		}
		walk(e.child, scope)
		for _, k := range e.kids {
			walk(k, scope)
		}
	}
	walk(o.root, 0)
	for i := len(scopes) - 1; i >= 0; i-- {
		if len(scopes[i]) > 0 {
			return scopes[i]
		}
	}
	return nil
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
