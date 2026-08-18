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
			// Typing reopens the list and drops the highlight: the previous
			// choice almost certainly does not match the new input, and
			// carrying it over means Enter picks something unrelated.
			s.SetState(func() { s.open, s.highlight = true, -1 })
			if f := w.OnChange; f != nil {
				f(v)
			}
		},
		OnSubmit: func(v string) { s.commit(v) },
	}

	rows := []Widget{Interactive{Handler: Handler{OnKey: s.onKey}, Child: field}}
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
		Handler: Handler{OnTap: func() { s.commit(sug) }},
		Child:   content,
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
// Enter with something highlighted picks it; Enter with nothing highlighted is
// left to the field's OnSubmit, so a user who ignored the list still submits
// what they typed rather than having their text replaced by a guess.
func (s *autocompleteState) onKey(k shell.Key) {
	// Releases would double every step: a held Down would move twice per press.
	if k.Kind != shell.KeyPress {
		return
	}
	list := s.visible()
	switch k.Code {
	case shell.KeyDown:
		s.move(1, len(list))
	case shell.KeyUp:
		s.move(-1, len(list))
	case shell.KeyEscape:
		s.SetState(func() { s.open, s.highlight = false, -1 })
	case shell.KeyEnter:
		if s.open && s.highlight >= 0 && s.highlight < len(list) {
			s.commit(list[s.highlight])
		}
	}
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
