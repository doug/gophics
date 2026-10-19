// Command scrolllist is the cookbook's scrolling-list recipe: ten thousand
// rows where only the visible ones are ever built, plus a controller that
// animates back to the top.
//
//	go run ./docs/recipes/scrolllist
//
// Every recipe under docs/recipes/ is a real program rather than a snippet in
// an HTML page, for the reason the rest of this repo already gives: a listing
// that only exists as markup drifts from the API, and the docs here have
// shipped that drift before. docs/cookbook.html embeds these files from source
// and ./scripts/gates.sh fails when an embed goes stale.
package main

import (
	"fmt"
	"log"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// List is the root widget: immutable configuration, of which there is none.
type List struct{}

func (List) CreateState() widget.State { return &listState{} }

type listState struct {
	widget.StateBase[List]
	rows []string
	// The controller is a plain struct the state owns, not a widget. It has to
	// outlive a rebuild — the list reads its position through it and the app
	// drives it — so it lives here and Build passes its address.
	ctrl     widget.ScrollController
	selected int
}

func (s *listState) Init(widget.Ctx) {
	s.selected = -1
	s.rows = make([]string, 10000)
	for i := range s.rows {
		s.rows[i] = fmt.Sprintf("Row %d", i+1)
	}
}

func (s *listState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)

	// LazyList mounts the rows in the viewport plus a little overscan and
	// nothing else, so Count can be large without the frame cost following
	// it. Build is called for one index at a time and must not assume it runs
	// in order, or that it runs at all for a given row.
	list := widget.LazyList{
		Count:           len(s.rows),
		EstimatedExtent: 56, // assumed height of a row not yet measured
		Controller:      &s.ctrl,
		Build: func(i int) widget.Widget {
			// WithKey ties the row's element — and so its state and any
			// animation in flight — to the item rather than to the index.
			// Without it, inserting at the top slides every row's state down
			// by one and the wrong row stays highlighted.
			return widget.WithKey{Key: s.rows[i], Child: theme.ListTile{
				Title:    s.rows[i],
				Subtitle: "built only while visible",
				Selected: i == s.selected,
				OnTap:    func() { s.SetState(func() { s.selected = i }) },
			}}
		},
	}

	col := widget.Column(
		theme.Button{Label: "Back to top", OnTap: func() {
			s.ctrl.AnimateTo(0, 250*time.Millisecond)
		}},
		widget.Sized{H: 12},
		// Expand is the part worth remembering. A scrollable child handed
		// unbounded height tries to be as tall as its content and then never
		// scrolls; inside a Column it must be given the leftover space.
		widget.Expand(list),
	)
	col.CrossAlign = layout.CrossStretch

	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 16, Child: col}}}
}

func main() {
	if err := app.Run(List{}, config()); err != nil {
		log.Fatal(err)
	}
}

func config() app.Config {
	return app.Config{
		Title:          "recipe · scrolling list",
		Size:           geom.Size{W: 420, H: 560},
		Background:     theme.Light().Bg,
		BackgroundDark: theme.Dark().Bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}
}
