package widget

import "github.com/doug/gophics/shell"

// Autocomplete is a text field that offers suggestions as you type.
//
// The widget owns the interaction — which suggestion is highlighted, whether
// the list is showing, what the keyboard does — and nothing else. Candidates
// come from Suggest, so the source can be a slice, a prefix tree or a network
// call the app debounces itself, and rows are the app's widgets, so the look
// belongs to the app.
type Autocomplete struct {
	// Value is the current text. Autocomplete is controlled: the app holds the
	// string and updates it in OnChange, as TextField does.
	Value       string
	Placeholder string
	// Suggest returns candidates for the current input, best first. Returning
	// none closes the list. It is called during build, so it should be cheap —
	// filter an in-memory slice here and do anything slower outside.
	Suggest func(input string) []string
	// OnChange fires on every edit, including one caused by picking.
	OnChange func(string)
	// OnPick fires when a suggestion is chosen, by click or by Enter.
	OnPick func(string)
	// MaxVisible caps the list (0 → 8). A suggestion list longer than the
	// screen is worse than a short one, and the tail is never read.
	MaxVisible int
	// Row renders one suggestion. Defaults to plain text, which keeps the
	// widget layer free of styling.
	Row func(s string, highlighted bool) Widget
}

func (a Autocomplete) CreateState() State { return &autocompleteState{highlight: -1} }

type autocompleteState struct {
	StateBase[Autocomplete]
	// open is false until the user types or moves, so a field that merely has
	// focus does not drop a list over the content beneath it.
	open bool
	// highlight indexes the visible suggestions, or -1 for none. It is an
	// index rather than a string because the same suggestion can appear twice
	// and because the keyboard moves by position.
	highlight int
}

func (s *autocompleteState) Build(ctx Ctx) Widget {
	w := s.W()
	list := s.visible()

	field := TextField{
		Value:       w.Value,
		Placeholder: w.Placeholder,
		OnChange: func(v string) {
			// Typing reopens the list on the best match. The previous choice
			// is dropped — it almost certainly does not match the new input —
			// but the highlight lands on the new first suggestion rather than
			// on nothing, because a list with no selection gives the user
			// nothing to Tab into and no indication that Enter will do
			// anything. Escape is how you get back to "just my text".
			s.SetState(func() { s.open, s.highlight = true, 0 })
			if f := w.OnChange; f != nil {
				f(v)
			}
		},
		OnSubmit: func(v string) { s.commit(v) },
		// The list is driven from here rather than from a wrapper around the
		// field. Keyboard events reach exactly one widget — the focused one,
		// with no bubbling — and once the user clicks into the field, that is
		// the field. A wrapper's OnKey never fires while anyone is typing,
		// which is why Up, Down, Tab and Escape did nothing.
		OnKeyPreview: s.onKey,
	}

	rows := []Widget{field}
	if s.open && len(list) > 0 {
		for i, sug := range list {
			rows = append(rows, s.row(i, sug))
		}
	}
	return Column(rows...)
}

// visible is the suggestion list as shown, already capped.
func (s *autocompleteState) visible() []string {
	w := s.W()
	if w.Suggest == nil {
		return nil
	}
	out := w.Suggest(w.Value)
	if n := w.MaxVisible; n > 0 && len(out) > n {
		return out[:n]
	}
	if len(out) > 8 {
		return out[:8]
	}
	return out
}

func (s *autocompleteState) row(i int, sug string) Widget {
	content := Widget(Text{S: sug})
	if f := s.W().Row; f != nil {
		content = f(sug, i == s.highlight)
	}
	return Interactive{
		Gestures: Gestures{OnTap: func() { s.commit(sug) }},
		Child:    content,
	}
}

// commit accepts a suggestion: the text becomes it, the list closes.
func (s *autocompleteState) commit(v string) {
	s.SetState(func() { s.open, s.highlight = false, -1 })
	w := s.W()
	if f := w.OnChange; f != nil {
		f(v)
	}
	if f := w.OnPick; f != nil {
		f(v)
	}
}

// onKey drives the list from the keyboard.
//
// Enter and Tab both take the highlighted suggestion; Up and Down move it;
// Escape closes the list. Once the list is closed, Enter falls through to the
// field's OnSubmit, so a user who does not want any of the suggestions can
// dismiss them and submit exactly what they typed. That escape route is what
// makes it safe for the highlight to default to the first match instead of to
// nothing.
func (s *autocompleteState) onKey(k shell.Key) bool {
	// Releases would double every step: a held Down would move twice per press.
	// They are reported as unhandled so the field still sees them.
	if k.Kind != shell.KeyPress {
		return false
	}
	list := s.visible()
	armed := s.open && s.highlight >= 0 && s.highlight < len(list)
	switch k.Code {
	case shell.KeyDown:
		if len(list) == 0 {
			return false
		}
		s.move(1, len(list))
		return true
	case shell.KeyUp:
		if len(list) == 0 {
			return false
		}
		s.move(-1, len(list))
		return true
	case shell.KeyEscape:
		// Only consumed while the list is showing; otherwise Escape belongs to
		// the field, which uses it to collapse a selection.
		if !s.open {
			return false
		}
		s.SetState(func() { s.open, s.highlight = false, -1 })
		return true
	case shell.KeyEnter, shell.KeyTab:
		// Both accept the highlighted suggestion. Neither is consumed when
		// there is nothing to accept: Enter then submits what was typed, and
		// Tab stays the focus key rather than trapping the user in the field.
		if !armed {
			return false
		}
		s.commit(list[s.highlight])
		return true
	}
	return false
}

// move steps the highlight, opening the list if it was closed.
func (s *autocompleteState) move(delta, n int) {
	if n == 0 {
		return
	}
	next := nextHighlight(s.highlight, delta, n)
	s.SetState(func() { s.open, s.highlight = true, next })
}

// nextHighlight steps an index through n items, wrapping at both ends.
//
// Wrapping rather than stopping: a list short enough to show is short enough to
// cycle, and stopping at the bottom leaves a user pressing Down with nothing
// happening. Starting from "none", Down selects the first and Up the last,
// which is what every native combo box does.
func nextHighlight(cur, delta, n int) int {
	if n <= 0 {
		return -1
	}
	if cur < 0 {
		if delta > 0 {
			return 0
		}
		return n - 1
	}
	next := (cur + delta) % n
	if next < 0 {
		next += n
	}
	return next
}
