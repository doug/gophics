// Command todo is the todo example, written against the widget layer.
//
// It exercises the framework end to end: stateful widgets with keyed
// reconciliation, per-row hover animations (anim.Controller tickers),
// tap-to-focus with focus-aware visuals, a scrolling viewport, and text
// input — all testable headless (see render_test.go).
package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/anim"
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
	hover int // last hovered row index (observed by tests)
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
		{text: "Animate, focus, scroll", done: true},
		{text: "Ship the web shell", done: false},
	}
}

func (s *todoState) Build(ctx widget.Ctx) widget.Widget {
	left := 0
	rows := make([]widget.Widget, 0, 2*len(s.items))
	for i, it := range s.items {
		if !it.done {
			left++
		}
		i := i
		rows = append(rows, widget.WithKey{Key: it.text, Child: todoRow{
			Item:     it,
			OnToggle: func() { s.SetState(func() { s.items[i].done = !s.items[i].done }) },
			OnDelete: func() { s.SetState(func() { s.remove(i) }) },
			OnHover:  func() { s.hover = i },
			OnLeave:  func() { s.leave(i) },
		}}, widget.Sized{H: 8})
	}
	list := widget.Column(rows...)
	list.CrossAlign = layout.CrossStretch

	footer := fmt.Sprintf("%d left · click row to toggle · hover shows delete", left)
	col := widget.Column(
		widget.Text{S: "gossamer · todo", Size: 15, Color: colDim},
		widget.Sized{H: 16},
		inputField{
			Value:  s.input,
			OnText: s.onText,
			OnKey:  s.onKey,
		},
		widget.Sized{H: 12},
		widget.Expand(widget.Scroll{Child: list}),
		widget.Sized{H: 12},
		widget.Text{S: footer, Size: 12, Color: colDim},
	)
	col.CrossAlign = layout.CrossStretch
	return widget.Padding{All: 20, Child: col}
}

// inputField is the entry box: focus-aware border, caret when focused.
type inputField struct {
	Value  string
	OnText func(string)
	OnKey  func(shell.Key)
}

func (f inputField) CreateState() widget.State { return &inputState{} }

type inputState struct {
	widget.StateBase[inputField]
	focused bool
}

func (s *inputState) Build(widget.Ctx) widget.Widget {
	f := s.W()
	border, borderW := colDim, float32(1)
	if s.focused {
		border, borderW = colAccent, 1.5
	}
	label := widget.Text{S: f.Value, Color: colText}
	var caret widget.Widget = widget.Sized{W: 2}
	if s.focused {
		caret = widget.Canvas{W: 2, H: 18, Draw: func(c paint.Canvas, r geom.Rect) {
			c.FillRect(r, colAccent)
		}}
	}
	content := widget.Row(label, caret, widget.Expand(widget.Sized{}))
	if f.Value == "" {
		placeholder := widget.Text{S: "What needs doing?  (Enter adds)", Color: colDim}
		content = widget.Row(caret, placeholder, widget.Expand(widget.Sized{}))
	}
	return widget.Interactive{
		Handler: widget.Handler{
			OnText:  f.OnText,
			OnKey:   f.OnKey,
			OnFocus: func(v bool) { s.SetState(func() { s.focused = v }) },
		},
		Child: widget.Decorated{
			Color: colCard, Radius: 8, BorderColor: border, BorderWidth: borderW,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(14, 12),
				Child:  content,
			},
		},
	}
}

// todoRow is one list row, with its own animated hover state.
type todoRow struct {
	Item     item
	OnToggle func()
	OnDelete func()
	OnHover  func()
	OnLeave  func()
}

func (r todoRow) CreateState() widget.State { return &rowState{} }

type rowState struct {
	widget.StateBase[todoRow]
	ctx     widget.Ctx
	hover   *anim.Controller
	hovered bool
}

func (s *rowState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.hover = &anim.Controller{
		Duration: 120 * time.Millisecond,
		OnChange: func() { s.SetState(nil) },
	}
	ctx.AddTicker(s.hover)
}

func (s *rowState) Dispose() { s.ctx.RemoveTicker(s.hover) }

func (s *rowState) Build(widget.Ctx) widget.Widget {
	r := s.W()
	bg := paint.Lerp(colCard, colCardHov, s.hover.Value())
	label := widget.Text{S: r.Item.text, Color: colText}
	if r.Item.done {
		label.Color, label.Strike = colDim, true
	}
	var del widget.Widget = widget.Sized{W: 20, H: 20}
	if s.hovered {
		del = widget.Interactive{
			Handler: widget.Handler{OnTap: r.OnDelete},
			Child:   widget.Text{S: "×", Size: 16, Color: colDanger},
		}
	}
	return widget.Interactive{
		Handler: widget.Handler{
			OnTap: r.OnToggle,
			OnEnter: func() {
				r.OnHover()
				s.SetState(func() { s.hovered = true })
				s.hover.Forward()
				s.ctx.Invalidate()
			},
			OnExit: func() {
				r.OnLeave()
				s.SetState(func() { s.hovered = false })
				s.hover.Reverse()
				s.ctx.Invalidate()
			},
		},
		Child: widget.Decorated{
			Color: bg, Radius: 8,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(12, 12),
				Child: widget.Row(
					checkbox(r.Item.done),
					widget.Sized{W: 12},
					label,
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
			Child: widget.Canvas{W: 20, H: 20, Draw: func(c paint.Canvas, r geom.Rect) {
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
