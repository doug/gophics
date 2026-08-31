package app

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
	"golang.org/x/image/font/gofont/goregular"
)

// acRoot is a mounted Autocomplete over a fixed candidate list.
type acRoot struct{ picked *string }

func (r acRoot) CreateState() widget.State { return &acRootState{picked: r.picked} }

type acRootState struct {
	widget.StateBase[acRoot]
	value  string
	picked *string
}

func (s *acRootState) Build(widget.Ctx) widget.Widget {
	return widget.Autocomplete{
		Value: s.value,
		Suggest: func(in string) []string {
			if in == "" {
				return nil
			}
			var out []string
			for _, c := range []string{"alpha", "alphabet", "beta"} {
				if len(in) <= len(c) && c[:len(in)] == in {
					out = append(out, c)
				}
			}
			return out
		},
		OnChange: func(v string) { s.SetState(func() { s.value = v }) },
		OnPick:   func(v string) { *s.picked = v },
	}
}

func acHarness(t *testing.T) (*Headless, *string) {
	t.Helper()
	picked := new(string)
	h, err := NewHeadless(acRoot{picked: picked}, Config{
		Size: geom.Size{W: 300, H: 300}, Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, picked
}

// Tab completes the highlighted suggestion. Without it the only keyboard way
// to accept one is Enter, which in most fields means "submit".
func TestAutocompleteTabCompletes(t *testing.T) {
	h, picked := acHarness(t)
	h.Type("al")
	h.Key(shell.KeyTab)
	if *picked != "alpha" {
		t.Fatalf("Tab picked %q, want %q", *picked, "alpha")
	}
}

// Typing highlights the best match rather than clearing the selection: a list
// with nothing selected gives the user nothing to Tab into, and no sign that
// the keyboard will do anything at all.
func TestAutocompleteEnterTakesTheFirstMatch(t *testing.T) {
	h, picked := acHarness(t)
	h.Type("al")
	h.Key(shell.KeyEnter)
	if *picked != "alpha" {
		t.Fatalf("Enter picked %q, want the first match %q", *picked, "alpha")
	}
}

// Down moves the highlight before accepting it.
func TestAutocompleteArrowsMoveTheSelection(t *testing.T) {
	h, picked := acHarness(t)
	h.Type("al")
	h.Key(shell.KeyDown)
	h.Key(shell.KeyTab)
	if *picked != "alphabet" {
		t.Fatalf("Down then Tab picked %q, want %q", *picked, "alphabet")
	}
}

// Escape gives back "just my text": it closes the list, and Enter then belongs
// to the field. That route is what makes defaulting the highlight to the first
// match safe rather than presumptuous.
func TestAutocompleteEscapeReleasesTheSelection(t *testing.T) {
	h, picked := acHarness(t)
	h.Type("al")
	h.Key(shell.KeyEscape)
	h.Key(shell.KeyEnter)
	// Enter now submits what was typed. What it must not do is substitute a
	// suggestion the user has just dismissed.
	if *picked == "alpha" || *picked == "alphabet" {
		t.Fatalf("Enter substituted the suggestion %q after the list was dismissed", *picked)
	}
	if *picked != "al" {
		t.Fatalf("Enter yielded %q, want the typed text %q", *picked, "al")
	}
}

// Tab must not be swallowed when there is nothing to complete, or the field is
// a keyboard trap.
func TestAutocompleteTabIsFreeWithNoSuggestions(t *testing.T) {
	h, picked := acHarness(t)
	h.Type("zzz")
	h.Key(shell.KeyTab)
	if *picked != "" {
		t.Fatalf("Tab picked %q with no suggestions", *picked)
	}
}
