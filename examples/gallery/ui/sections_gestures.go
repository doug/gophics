package ui

// Pull-to-refresh and swipe-to-dismiss, one demo each.
//
// Both used to live inside the Navigation & gestures feed, alongside a
// Navigator, Hero transitions, selectable text and a like button. Five things
// in one page meant none of them was the subject: a reader who wanted to see
// how swipe-to-dismiss is wired had to find it inside a list that was also
// demonstrating four other ideas. They are separate sections now, each showing
// one widget doing one thing, which is how the rest of the catalog reads.

import (
	"fmt"

	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Pull to refresh ---------------------------------------------------------

// refreshSection is a LazyList wired to pull-to-refresh: drag down from the top
// and the list regenerates.
type refreshSection struct{}

func (refreshSection) CreateState() widget.State { return &refreshState{} }

type refreshState struct {
	widget.StateBase[refreshSection]
	cards      []card
	seed       int
	refreshing bool
	count      int // how many times it has refreshed, so the effect is legible
}

func (s *refreshState) Init(widget.Ctx) { s.cards = makeCards(6, 0) }

func (s *refreshState) refresh(ctx widget.Ctx) {
	s.SetState(func() { s.refreshing = true })
	// A real app fetches here and clears Refreshing when the work finishes.
	// This regenerates on the next frame so the spinner is visible for one.
	post := ctx.Post()
	post(func() {
		s.SetState(func() {
			s.seed += 6
			s.count++
			s.cards = makeCards(6, s.seed)
			s.refreshing = false
		})
	})
}

// refreshedLabel reads as a sentence at every count, rather than "0 time(s)".
func refreshedLabel(n int) string {
	if n == 1 {
		return "Refreshed once"
	}
	return fmt.Sprintf("Refreshed %d times", n)
}

func (s *refreshState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	list := widget.LazyList{
		Count:           len(s.cards),
		EstimatedExtent: 96,
		Refreshing:      s.refreshing,
		OnRefresh:       func() { s.refresh(ctx) },
		Build:           func(i int) widget.Widget { return cardTile(th, s.cards[i]) },
	}
	return sectionColumn(
		theme.Body("Drag down from the top of the list to refresh it."),
		widget.Sized{H: 4},
		widget.Text{
			Value: refreshedLabel(s.count),
			Size:  th.Type.Label,
			Color: th.Muted,
		},
		widget.Sized{H: 10},
		// A scrolling list needs a bounded height inside a scrolling page.
		// Tall enough that the cut-off row reads as "more below" rather than
		// as the demo running out.
		widget.Sized{H: 430, Child: list},
	)
}

// --- Swipe to dismiss --------------------------------------------------------

// dismissSection is a short list of Dismissible rows: swipe one aside and it is
// removed, with a panel showing behind it as it slides.
type dismissSection struct{}

func (dismissSection) CreateState() widget.State { return &dismissState{} }

type dismissState struct {
	widget.StateBase[dismissSection]
	cards   []card
	removed int
}

func (s *dismissState) Init(widget.Ctx) { s.cards = makeCards(5, 40) }

func (s *dismissState) remove(id int) {
	s.SetState(func() {
		for i, c := range s.cards {
			if c.id == id {
				s.cards = append(s.cards[:i], s.cards[i+1:]...)
				s.removed++
				return
			}
		}
	})
}

func (s *dismissState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)

	rows := make([]widget.Widget, 0, len(s.cards)+1)
	for _, c := range s.cards {
		// WithKey is what makes removal animate the right row: without a stable
		// key the reconciler matches rows by position, so removing the middle
		// one looks like the last one vanishing.
		rows = append(rows, widget.WithKey{Key: c.id, Child: widget.Dismissible{
			OnDismissed: func() { s.remove(c.id) },
			Background:  dismissPanel(th),
			Child:       cardTile(th, c),
		}})
	}
	if len(s.cards) == 0 {
		rows = append(rows, widget.Padding{All: 16,
			Child: theme.Body("All gone. Reopen this section to start over.")})
	}

	return sectionColumn(
		theme.Body("Swipe a row sideways to remove it."),
		widget.Sized{H: 4},
		widget.Text{
			Value: fmt.Sprintf("Removed %d of 5", s.removed),
			Size:  th.Type.Label,
			Color: th.Muted,
		},
		widget.Sized{H: 10},
		sectionColumn(rows...),
	)
}
