// Command counter is the canonical minimal gophics app referenced by SKILL.md:
// a stateful widget whose state is mutated through SetState. It is compiled by
// `go build ./...` in CI, so the snippet in SKILL.md can never drift from a
// building API.
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Counter is the widget: immutable configuration (here, none).
type Counter struct{}

// CreateState marks Counter as stateful and returns its mutable companion.
func (Counter) CreateState() widget.State { return &counterState{} }

type counterState struct {
	widget.StateBase[Counter] // gives W() (current config) and SetState()
	n                         int
}

func (s *counterState) Build(ctx widget.Ctx) widget.Widget {
	return widget.Column(
		widget.Text{S: fmt.Sprintf("count: %d", s.n), Size: 28, Color: paint.RGB(0.92, 0.93, 0.95)},
		widget.Interactive{
			Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.n++ }) }},
			Child:   widget.Text{S: "increment", Size: 18, Color: paint.RGB(0.36, 0.62, 0.98)},
		},
	)
}

func main() {
	if err := app.Run(Counter{}, app.Config{
		Title:      "gophics counter",
		Size:       geom.Size{W: 320, H: 200},
		Background: paint.RGB(0.07, 0.08, 0.11),
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
