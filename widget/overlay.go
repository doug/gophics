package widget

// Overlay renders content above the entire widget tree — the foundation for
// dialogs, menus, tooltips, and snackbars. An OverlayHost is installed at
// the app root (app.NewCore wraps the tree in one), so any widget can reach
// the handle:
//
//	ov := ctx.MustOf[widget.Overlay]()
//	tok := ov.ShowFrom(ctx, myDialog)
//	...
//	tok.Dismiss()
//
// Entries stack in insertion order (last on top) and keep their state until
// dismissed. The theme package builds Dialog and Menu on this.
//
// An entry is mounted under the host, beside the app rather than inside the
// widget that opened it, so on its own it sees only what is provided above
// the host. ShowFrom gives an entry the opener's scope (see Scoped): Of and
// MustOf inside it resolve the opener's Nav, theme and Provides. Show keeps
// the unscoped behaviour for content that deliberately stands alone.

// OverlayHost provides an Overlay to its subtree and renders active entries
// above Child.
type OverlayHost struct{ Child Widget }

func (OverlayHost) CreateState() State { return &overlayState{} }

// Overlay is the handle for showing overlay entries.
type Overlay struct{ s *overlayState }

// OverlayToken identifies a shown entry, for updating or dismissing it.
type OverlayToken struct {
	s  *overlayState
	id int
}

// Show adds w as a new top-most entry and returns its token. The entry is
// unscoped: lookups from inside it see only what is provided above the
// OverlayHost. Use ShowFrom for content that belongs to the widget opening it.
func (o Overlay) Show(w Widget) OverlayToken {
	return OverlayToken{s: o.s, id: o.s.push(w, nil)}
}

// ShowFrom adds w as a new top-most entry with the scope of ctx: Of/MustOf
// inside w resolve through ctx's ancestors — the Nav of the page opening a
// dialog, an in-tree Provide, the theme — exactly as if w were built below
// ctx, while it still lays out and paints above the whole tree. The scope
// survives Update; it is a property of the entry, not of one widget value.
//
// Should the opener unmount while the entry is still shown, the entry falls
// back to the unscoped walk (see Scoped for why), so content meant to outlive
// its opener captures what it needs at show time.
func (o Overlay) ShowFrom(ctx Ctx, w Widget) OverlayToken {
	return OverlayToken{s: o.s, id: o.s.push(w, ctx.el)}
}

// Dismiss removes the entry.
func (t OverlayToken) Dismiss() {
	if t.s != nil {
		t.s.remove(t.id)
	}
}

// Update replaces the entry's content (e.g. an animating snackbar).
func (t OverlayToken) Update(w Widget) {
	if t.s != nil {
		t.s.update(t.id, w)
	}
}

type overlayEntry struct {
	id     int
	widget Widget
	origin *element // the opener, for ShowFrom; nil for an unscoped Show
}

type overlayState struct {
	StateBase[OverlayHost]
	entries []overlayEntry
	nextID  int
}

func (s *overlayState) push(w Widget, origin *element) int {
	id := s.nextID
	s.nextID++
	s.SetState(func() { s.entries = append(s.entries, overlayEntry{id, w, origin}) })
	return id
}

func (s *overlayState) remove(id int) {
	s.SetState(func() {
		for i, e := range s.entries {
			if e.id == id {
				s.entries = append(s.entries[:i], s.entries[i+1:]...)
				return
			}
		}
	})
}

func (s *overlayState) update(id int, w Widget) {
	s.SetState(func() {
		for i := range s.entries {
			if s.entries[i].id == id {
				s.entries[i].widget = w
				return
			}
		}
	})
}

func (s *overlayState) Build(Ctx) Widget {
	children := make([]Widget, 0, len(s.entries)+1)
	// Fill makes the app content fill the surface tightly, preserving the
	// constraints it would get as the untouched root (Stack loosens).
	children = append(children, Fill{Child: s.W().Child})
	for _, e := range s.entries {
		content := e.widget
		if e.origin != nil {
			// Wrapped here rather than in ShowFrom so an Update keeps the
			// scope: the token's caller replaces the content, not the entry.
			content = scopeBridge{origin: e.origin, child: content}
		}
		// Each entry is its own Tab cycle: a dialog with fields keeps focus
		// among them rather than letting Tab wander into the page beneath
		// its scrim (see focusables).
		children = append(children, WithKey{Key: e.id, Child: focusScope{child: content}})
	}
	content := Stack{Children: children}
	return Provide[Overlay]{Value: Overlay{s: s}, Child: content}
}
