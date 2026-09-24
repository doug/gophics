package widget

// Modal marks its subtree as a modal layer: while it is mounted, Escape goes
// to OnEscape instead of to the focused widget.
//
// Keys reach exactly one widget, the focused one. A dialog's scrim used to
// read Escape from its own OnKey, which only works while the scrim holds
// focus — and it does not once a field inside the dialog takes it (Autofocus,
// or a tap into the field), nor when the dialog was opened from a field that
// kept it (an OnSubmit). Escape then collapsed the field's selection and the
// dialog stayed up, though its doc promised otherwise.
//
// Dismissal is the app's business rather than any one widget's, by the same
// reasoning that moves Tab before the focused widget sees it: a field cannot
// know what is stacked above it. So the app runner delivers Escape to the
// innermost mounted Modal — the most recently mounted one, which is the entry
// on top of the overlay — ahead of the keyboard target. That is also what the
// platforms do: Escape in a sheet cancels the sheet whatever the state of a
// field inside it.
//
// Dialog, bottom sheet and select popup wrap their scrims in one. A widget
// that needs Escape for itself while a modal is up — an inline suggestion
// list — wraps itself in a Modal of its own while it is showing, so it is the
// innermost layer and takes the key first.
type Modal struct {
	// OnEscape runs when Escape is pressed while this is the innermost
	// modal that wants it. Nil declines the key: the next modal out gets
	// it, and with none left the focused widget does. A layer that cannot
	// be escaped from is spelled as a Modal with a no-op. Declining is how
	// a widget that is sometimes modal — a suggestion list that is only
	// showing some of the time — stays mounted as one Modal and claims the
	// key only while it has something to close, instead of wrapping and
	// unwrapping itself, which would remount its subtree.
	OnEscape func()
	Child    Widget
}

func (Modal) CreateState() State { return &modalState{} }

type modalState struct {
	StateBase[Modal]
	ctx Ctx
}

func (s *modalState) Init(ctx Ctx) {
	s.ctx = ctx
	o := ctx.el.owner
	o.modals = append(o.modals, s)
}

func (s *modalState) Dispose() {
	o := s.ctx.el.owner
	for i, m := range o.modals {
		if m == s {
			o.modals = append(o.modals[:i], o.modals[i+1:]...)
			return
		}
	}
}

func (s *modalState) Build(Ctx) Widget { return s.W().Child }

// TopModal returns the Escape handler of the innermost mounted Modal that
// has one, or nil when no modal layer wants the key. The app runner calls it
// before dispatching Escape to the keyboard target.
func (o *Owner) TopModal() func() {
	for i := len(o.modals) - 1; i >= 0; i-- {
		if f := o.modals[i].W().OnEscape; f != nil {
			return f
		}
	}
	return nil
}
