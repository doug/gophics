// Command todo is the todo example, written against the widget layer.
//
// Compare with the previous revision of this file (git log): the same app
// hand-rolled against paint/shell was ~250 lines of manual layout math, hit
// testing, and invalidation. All of that now lives in the framework.
package main

import (
	"fmt"
	"log"
	"strings"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/widget"
)

var (
	colBg      = paint.RGB(0.07, 0.08, 0.11)
	colCard    = paint.RGB(0.12, 0.14, 0.19)
	colCardHov = paint.RGB(0.15, 0.17, 0.23)
	colAccent  = paint.RGB(0.36, 0.62, 0.98)
	colText    = paint.RGB(0.92, 0.93, 0.95)
	colDim     = paint.RGB(0.52, 0.55, 0.62)
	colDanger  = paint.RGB(0.90, 0.42, 0.42)
)

type item struct {
	text string
	done bool
}

// Todo is the root widget.
type Todo struct{}

func (Todo) CreateState() widget.State { return &todoState{} }

type todoState struct {
	widget.StateBase[Todo]
	items []item
	input string
	hover int
}

// stateHook lets tests observe the mounted state.
var stateHook func(*todoState)

func (s *todoState) Init(widget.Ctx) {
	if stateHook != nil {
		stateHook(s)
	}
	s.hover = -1
	s.items = []item{
		{text: "Plan the port", done: true},
		{text: "Open a window", done: true},
		{text: "Build the todo example", done: true},
		{text: "Grow widgets until this file shrinks", done: true},
		{text: "Ship the web shell", done: false},
	}
}

func (s *todoState) Build(ctx widget.Ctx) widget.Widget {
	left := 0
	rows := make([]widget.Widget, 0, len(s.items)+4)
	rows = append(rows,
		widget.Text{S: "gossamer · todo", Size: 15, Color: colDim},
		widget.Sized{H: 16},
		s.inputField(),
		widget.Sized{H: 12},
	)
	for i, it := range s.items {
		if !it.done {
			left++
		}
		rows = append(rows, widget.WithKey{Key: it.text, Child: s.row(i)}, widget.Sized{H: 8})
	}
	footer := fmt.Sprintf("%d left · click row to toggle · hover shows delete", left)
	rows = append(rows,
		widget.Expand(widget.Sized{}),
		widget.Text{S: footer, Size: 12, Color: colDim},
	)

	col := widget.Column(rows...)
	col.CrossAlign = layout.CrossStretch
	return widget.Padding{All: 20, Child: col}
}

func (s *todoState) inputField() widget.Widget {
	label := widget.Text{S: s.input, Color: colText}
	if s.input == "" {
		label = widget.Text{S: "What needs doing?  (Enter adds)", Color: colDim}
	}
	return widget.Interactive{
		Handler: widget.Handler{OnText: s.onText, OnKey: s.onKey},
		Child: widget.Decorated{
			Color: colCard, Radius: 8, BorderColor: colAccent, BorderWidth: 1,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(14, 12),
				Child:  widget.Row(label, widget.Expand(widget.Sized{})),
			},
		},
	}
}

func (s *todoState) row(i int) widget.Widget {
	it := s.items[i]
	bg := colCard
	if i == s.hover {
		bg = colCardHov
	}
	labelColor := colText
	if it.done {
		labelColor = colDim
	}
	var del widget.Widget = widget.Sized{W: 20, H: 20}
	if i == s.hover {
		del = widget.Interactive{
			Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.remove(i) }) }},
			Child:   widget.Text{S: "×", Size: 16, Color: colDanger},
		}
	}
	return widget.Interactive{
		Handler: widget.Handler{
			OnTap:   func() { s.SetState(func() { s.items[i].done = !s.items[i].done }) },
			OnEnter: func() { s.SetState(func() { s.hover = i }) },
			OnExit:  func() { s.SetState(func() { s.leave(i) }) },
		},
		Child: widget.Decorated{
			Color: bg, Radius: 8,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(12, 12),
				Child: widget.Row(
					checkbox(it.done),
					widget.Sized{W: 12},
					widget.Text{S: it.text, Color: labelColor},
					widget.Expand(widget.Sized{}),
					del,
				),
			},
		},
	}
}

func checkbox(done bool) widget.Widget {
	if done {
		return widget.Decorated{
			Color: colAccent, Radius: 5,
			Child: widget.Canvas{W: 20, H: 20, Draw: func(c *paint.Canvas, r geom.Rect) {
				c.Line(r.Min.Add(geom.Pt{X: 4, Y: 10}), r.Min.Add(geom.Pt{X: 8, Y: 14}), 2, colBg)
				c.Line(r.Min.Add(geom.Pt{X: 8, Y: 14}), r.Min.Add(geom.Pt{X: 16, Y: 5}), 2, colBg)
			}},
		}
	}
	return widget.Decorated{
		BorderColor: colDim, BorderWidth: 1.5, Radius: 5,
		Child: widget.Sized{W: 20, H: 20},
	}
}

func (s *todoState) onText(t string) {
	t = strings.Map(func(r rune) rune {
		if r < ' ' {
			return -1
		}
		return r
	}, t)
	if t != "" {
		s.SetState(func() { s.input += t })
	}
}

func (s *todoState) onKey(k shell.Key) {
	if k.Kind != shell.KeyPress {
		return
	}
	switch k.Code {
	case shell.KeyEnter:
		if t := strings.TrimSpace(s.input); t != "" {
			s.SetState(func() {
				s.items = append(s.items, item{text: t})
				s.input = ""
			})
		}
	case shell.KeyBackspace:
		if s.input != "" {
			s.SetState(func() {
				r := []rune(s.input)
				s.input = string(r[:len(r)-1])
			})
		}
	case shell.KeyEscape:
		s.SetState(func() { s.input = "" })
	}
}

func (s *todoState) remove(i int) {
	s.items = append(s.items[:i], s.items[i+1:]...)
	s.hover = -1
}

func (s *todoState) leave(i int) {
	if s.hover == i {
		s.hover = -1
	}
}

func main() {
	err := app.Run(Todo{}, app.Config{
		Title:      "gossamer todo",
		Size:       geom.Size{W: 440, H: 560},
		Background: colBg,
		Font:       goregular.TTF,
	})
	if err != nil {
		log.Fatal(err)
	}
}
