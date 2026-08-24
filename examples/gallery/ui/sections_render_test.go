package ui

import (
	"testing"

	"github.com/doug/gophics/widget"
)

// Every catalog entry must build and render. A section that panics or comes up
// blank is worse than a missing one: the catalog promises the capability works
// and then demonstrates that it does not.
//
// This is a smoke test by construction — it renders each page and asks only
// that something legible came out. What it really guards is the registration
// step, where a section is added to the list and its page never opened again.
func TestEverySectionRenders(t *testing.T) {
	all := sections()
	if len(all) < 15 {
		t.Fatalf("only %d sections registered; the catalog lost entries", len(all))
	}

	// What an empty section looks like: the scaffold's own chrome and nothing
	// else. Measuring against this is what makes the check real — counting
	// labels alone passes for a section whose body renders nothing, because
	// the title and subtitle are always there.
	baseline := func(title, subtitle string) int {
		a := galleryApp(t, sectionPage{title: title, subtitle: subtitle, body: widget.Sized{}})
		a.Render()
		return len(a.Labels())
	}

	for _, sec := range all {
		t.Run(sec.title, func(t *testing.T) {
			a := galleryApp(t, sec.page())
			a.Render()

			got := len(a.Labels())
			if got == 0 {
				t.Fatalf("%q rendered no semantics at all — the page is blank or "+
					"panicked during build", sec.title)
			}
			// The baseline only means something for pages built on the shared
			// scaffold. The Navigator demo is its own page, with no empty form
			// to compare against, so for that one "rendered anything" is the
			// whole check.
			sp, ok := sec.page().(sectionPage)
			if !ok {
				return
			}
			if want := baseline(sp.title, sp.subtitle); got <= want {
				t.Errorf("%q produced %d semantics nodes against %d for an empty "+
					"body — the section itself rendered nothing", sec.title, got, want)
			}
		})
	}
}

// The widgets the catalog exists to demonstrate must actually be reachable from
// it. This pins the ones that were missing — a capability with no demo is one
// nobody finds, and the gallery is the only place most of them are exercised
// outside their own unit tests.
func TestCatalogCoversTheWidgetCapabilities(t *testing.T) {
	titles := map[string]bool{}
	for _, s := range sections() {
		titles[s.title] = true
	}
	for _, want := range []string{
		"Tree",
		"Autocomplete",
		"Reorderable list",
		"Drag & drop",
		"Rich text & selection",
		"Transform",
		"Right to left",
	} {
		if !titles[want] {
			t.Errorf("the catalog has no %q section", want)
		}
	}
}
