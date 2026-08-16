// Command counter is the smallest complete gophics app, and the one the
// homepage and the getting-started guide both show.
//
// It is a real example rather than a snippet in the docs so that it compiles
// in CI and its screenshot is generated from this exact source — a sample that
// only exists as HTML drifts from the API, and this one already had.
//
// The whole program:
//
//	go run ./examples/counter          # a window
//	GOOS=js GOARCH=wasm go build       # the same UI in a browser
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Counter is a widget: a plain struct value describing what to show.
type Counter struct{ Start int }

func (Counter) CreateState() widget.State { return &counterState{} }

// counterState holds what changes. StateBase[Counter] gives it SetState and a
// typed W() — the current Counter value, with no cast.
type counterState struct {
	widget.StateBase[Counter]
	n int
}

func (s *counterState) Init(widget.Ctx) { s.n = s.W().Start }

func (s *counterState) Build(ctx widget.Ctx) widget.Widget {
	// Pinned to the light theme rather than following the system setting.
	//
	// Most apps should follow it — theme.Of does that by default, and the
	// other examples rely on it. This one is the figure on the home page,
	// shown beside a still of itself, and a screenshot cannot follow anybody's
	// colour scheme. Pinning both to light keeps the pair honest; the
	// alternative is shipping two stills and picking between them, which is a
	// lot of machinery for a counter.
	th := theme.Light()
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Center(widget.Column(
			widget.Text{S: "TAPS", Size: th.Type.Caption, Color: th.Muted},
			widget.Sized{H: 4},
			widget.Text{
				S:     fmt.Sprintf("%d", s.n),
				Size:  th.Type.Display,
				Font:  theme.FontBold,
				Color: th.Text,
			},
			widget.Sized{H: 18},
			theme.Button{
				Label:   "Increment",
				Primary: true,
				OnTap:   func() { s.SetState(func() { s.n++ }) },
			},
		))}}
}

func main() {
	err := app.Run(Counter{Start: 3}, app.Config{
		Title: "counter",
		Size:  geom.Size{W: 320, H: 220},
		// Light in both schemes, matching the pinned theme above.
		Background: theme.Light().Bg,
		Font:       goregular.TTF,
		FontFamilies: map[string][]byte{
			theme.FontBold: gobold.TTF,
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}
