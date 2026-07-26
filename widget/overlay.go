package widget

// Overlay renders content above the entire widget tree — the foundation for
// dialogs, menus, tooltips, and snackbars. An OverlayHost is installed at
// the app root (app.NewCore wraps the tree in one), so any widget can reach
// the handle:
//
//	ov := widget.MustOf[widget.Overlay](ctx)
//	tok := ov.Show(myDialog)
//	...
//	tok.Dismiss()
//
// Entries stack in insertion order (last on top) and keep their state until
// dismissed. The theme package builds Dialog and Menu on this.

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

// Show adds w as a new top-most entry and returns its token.
func (o Overlay) Show(w Widget) OverlayToken {
	return OverlayToken{s: o.s, id: o.s.push(w)}
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
}

type overlayState struct {
	StateBase[OverlayHost]
	entries []overlayEntry
	nextID  int
}

func (s *overlayState) push(w Widget) int {
	id := s.nextID
	s.nextID++
	s.SetState(func() { s.entries = append(s.entries, overlayEntry{id, w}) })
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
		children = append(children, WithKey{Key: e.id, Child: e.widget})
	}
	content := Stack{Children: children}
	return Provide[Overlay]{Value: Overlay{s: s}, Child: content}
}
