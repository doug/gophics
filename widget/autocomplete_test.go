package widget

import "testing"

// Wrapping rather than stopping: a list short enough to show is short enough to
// cycle, and stopping at the bottom leaves a user pressing Down with nothing
// happening.
func TestAutocompleteHighlightWraps(t *testing.T) {
	// From "none", Down takes the first and Up the last — what every native
	// combo box does.
	if got := nextHighlight(-1, 1, 3); got != 0 {
		t.Errorf("Down from none = %d, want 0", got)
	}
	if got := nextHighlight(-1, -1, 3); got != 2 {
		t.Errorf("Up from none = %d, want the last (2)", got)
	}
	if got := nextHighlight(2, 1, 3); got != 0 {
		t.Errorf("Down from the last = %d, want to wrap to 0", got)
	}
	if got := nextHighlight(0, -1, 3); got != 2 {
		t.Errorf("Up from the first = %d, want to wrap to 2", got)
	}
	if got := nextHighlight(0, 1, 3); got != 1 {
		t.Errorf("Down = %d, want 1", got)
	}
}

// An empty list must yield "none" rather than an index into nothing.
func TestAutocompleteHighlightEmptyList(t *testing.T) {
	if got := nextHighlight(-1, 1, 0); got != -1 {
		t.Errorf("Down on an empty list = %d, want -1", got)
	}
	if got := nextHighlight(3, 1, 0); got != -1 {
		t.Errorf("a stale index against an empty list = %d, want -1", got)
	}
}

func acState(t *testing.T, w Autocomplete) *autocompleteState {
	t.Helper()
	s := &autocompleteState{highlight: -1}
	s.setWidget(w)
	return s
}

// The cap exists because a suggestion list longer than the screen is worse than
// a short one — the tail is never read.
func TestAutocompleteCapsSuggestions(t *testing.T) {
	many := func(string) []string {
		out := make([]string, 20)
		for i := range out {
			out[i] = "item"
		}
		return out
	}
	if got := len(acState(t, Autocomplete{Suggest: many}).visible()); got != 8 {
		t.Errorf("default cap gave %d rows, want 8", got)
	}
	if got := len(acState(t, Autocomplete{Suggest: many, MaxVisible: 3}).visible()); got != 3 {
		t.Errorf("MaxVisible 3 gave %d rows, want 3", got)
	}
}

// No Suggest at all must be inert rather than panicking: an autocomplete with
// nothing to suggest is just a text field.
func TestAutocompleteWithoutSuggest(t *testing.T) {
	if got := acState(t, Autocomplete{}).visible(); got != nil {
		t.Errorf("visible() = %v with no Suggest, want nil", got)
	}
}

// Suggest sees the current input, so filtering is the app's to define.
func TestAutocompleteSuggestSeesInput(t *testing.T) {
	var saw string
	s := acState(t, Autocomplete{
		Value:   "ba",
		Suggest: func(in string) []string { saw = in; return []string{"banana"} },
	})
	got := s.visible()
	if saw != "ba" {
		t.Errorf("Suggest saw %q, want the current value", saw)
	}
	if len(got) != 1 || got[0] != "banana" {
		t.Errorf("visible() = %v, want [banana]", got)
	}
}

// A fresh autocomplete starts closed, so a field that merely has focus does not
// drop a list over the content beneath it.
func TestAutocompleteStartsClosed(t *testing.T) {
	s := Autocomplete{}.CreateState().(*autocompleteState)
	if s.open {
		t.Error("the list is open before any input")
	}
	if s.highlight != -1 {
		t.Errorf("highlight = %d on a fresh state, want -1", s.highlight)
	}
}
