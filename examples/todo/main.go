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

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// BG is the window background used at Start, before a widget context exists
// (passed as Config.Background). Inside the tree every color comes from
// theme.Of(ctx), so the app follows the platform light/dark scheme for free.
var BG = theme.Light().Bg

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
	// editing is the row being edited, or -1. It lives here rather than in the
	// row because closing it is not the row's business: a tap on the page
	// background has to end the edit, and gophics leaves focus alone when a
	// press lands on nothing focusable, so there is no blur event to hang it
	// on. Keeping it here also makes two editors at once unrepresentable.
	editing int
	draft   string
}

// stateHook lets tests observe the mounted state.
var stateHook func(*todoState)

func (s *todoState) Init(widget.Ctx) {
	if stateHook != nil {
		stateHook(s)
	}
	s.hover = -1
	s.editing = -1
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
	// Resolve the theme from the platform color scheme and provide it to the
	// tree, so every widget below reads colors with theme.Of(ctx) and the whole
	// app follows light/dark automatically.
	th := theme.Auto(ctx)
	left := 0
	rows := make([]widget.Widget, 0, 2*len(s.items))
	for i, it := range s.items {
		if !it.done {
			left++
		}
		rows = append(rows, widget.WithKey{Key: it.text, Child: widget.Dismissible{
			OnDismissed: func() { s.SetState(func() { s.remove(i) }) },
			Background: widget.Fill{Color: th.Danger, Child: widget.Padding{
				Insets: geom.Insets{Left: 16, Right: 16},
				Child: widget.Row(
					widget.Text{S: "delete", Size: th.Type.Label, Color: th.OnPrimary},
					widget.Expand(widget.Sized{}),
					widget.Text{S: "delete", Size: th.Type.Label, Color: th.OnPrimary},
				),
			}},
			Child: todoRow{
				Item:    it,
				Editing: s.editing == i,
				Draft:   s.draft,
				// Toggling also ends an open edit, so clicking another row
				// commits the one in progress instead of leaving it behind.
				OnToggle: func() { s.SetState(func() { s.commitEdit(); s.items[i].done = !s.items[i].done }) },
				OnDelete: func() { s.SetState(func() { s.remove(i) }) },
				OnBegin:  func() { s.SetState(func() { s.editing, s.draft = i, s.items[i].text }) },
				OnDraft:  func(v string) { s.SetState(func() { s.draft = v }) },
				OnCommit: func() { s.SetState(s.commitEdit) },
				OnHover:  func() { s.hover = i },
				OnLeave:  func() { s.leave(i) },
			},
		}}, widget.Sized{H: 8})
	}
	list := widget.Column(rows...)
	list.CrossAlign = layout.CrossStretch

	footer := fmt.Sprintf("%d left · click toggles · double-click edits · hover shows delete", left)
	col := widget.Column(
		widget.Text{S: "gophics · todo", Size: th.Type.Body, Color: th.Muted},
		widget.Sized{H: 16},
		theme.Field{
			Value:       s.input,
			Placeholder: "What needs doing?  (Enter adds)",
			OnChange:    func(v string) { s.SetState(func() { s.input = v }) },
			OnSubmit:    s.submit,
		},
		widget.Sized{H: 12},
		widget.Expand(widget.Scroll{Child: list}),
		widget.Sized{H: 12},
		widget.Text{S: footer, Size: th.Type.Caption, Color: th.Muted},
	)
	col.CrossAlign = layout.CrossStretch
	// A tap that reaches the page background ends an open edit. gophics leaves
	// keyboard focus alone when a press lands on nothing focusable, so the
	// field itself never hears about the click — the page has to notice.
	var page widget.Widget = widget.Fill{Color: th.Bg, Child: widget.Padding{All: 20, Child: col}}
	if s.editing >= 0 {
		page = widget.Interactive{
			Gestures: widget.Gestures{OnTap: func() { s.SetState(s.commitEdit) }},
			Child:    page,
		}
	}
	return widget.Provide[theme.Theme]{Value: th, Child: page}
}

// todoRow is one list row, with its own animated hover state.
type todoRow struct {
	Item     item
	Editing  bool
	Draft    string
	OnToggle func()
	OnDelete func()
	OnBegin  func()
	OnDraft  func(string)
	OnCommit func()
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

func (s *rowState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	r := s.W()
	bg := paint.Lerp(th.Surface, th.SurfaceHover, s.hover.Value())

	// Editing replaces the whole row rather than just the label: the row's own
	// tap-to-toggle and swipe-to-delete would otherwise still be live under a
	// text field, so dragging to select a word would delete the item.
	if r.Editing {
		return widget.Decorated{
			Color: bg, Radius: 8,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(6, 12),
				Child: theme.Field{
					Value:     r.Draft,
					Autofocus: true,
					OnChange:  r.OnDraft,
					OnSubmit:  func(string) { r.OnCommit() },
				},
			},
		}
	}

	text := widget.Text{S: r.Item.text, Color: th.Text}
	if r.Item.done {
		text.Color, text.Strike = th.Muted, true
	}
	// The double-click target is the text itself, not the row. Setting
	// OnDoubleTap defers that handler's single tap by the double-tap window to
	// disambiguate, and paying that delay on the checkbox — the one control
	// that should feel instant — to buy editing on the label is a bad trade.
	// Scoped here, a click on the box or the empty right-hand side still
	// toggles immediately.
	var label widget.Widget = widget.Interactive{
		Gestures: widget.Gestures{
			OnTap:       r.OnToggle,
			OnDoubleTap: r.OnBegin,
		},
		Child: text,
	}
	var del widget.Widget = widget.Sized{W: 20, H: 20}
	if s.hovered {
		del = widget.Interactive{
			Gestures: widget.Gestures{OnTap: r.OnDelete},
			Child:    widget.Text{S: "×", Size: 16, Color: th.Danger},
		}
	}
	return widget.Interactive{
		Gestures: widget.Gestures{
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
					checkbox(th, r.Item.done),
					widget.Sized{W: 12},
					label,
					widget.Expand(widget.Sized{}),
					del,
				),
			},
		},
	}
}

func checkbox(th theme.Theme, done bool) widget.Widget {
	if done {
		return widget.Decorated{
			Color: th.Primary, Radius: 5,
			Child: widget.Canvas{W: 20, H: 20, Draw: func(c paint.Canvas, size geom.Size) {
				r := geom.Rect{Max: size.Pt()}
				c.Line(r.Min.Add(geom.Pt{X: 4, Y: 10}), r.Min.Add(geom.Pt{X: 8, Y: 14}), 2, th.OnPrimary)
				c.Line(r.Min.Add(geom.Pt{X: 8, Y: 14}), r.Min.Add(geom.Pt{X: 16, Y: 5}), 2, th.OnPrimary)
			}},
		}
	}
	return widget.Decorated{
		BorderColor: th.Border, BorderWidth: 1.5, Radius: 5,
		Child: widget.Sized{W: 20, H: 20},
	}
}

func (s *todoState) submit(v string) {
	if t := strings.TrimSpace(v); t != "" {
		s.SetState(func() {
			s.items = append(s.items, item{text: t})
			s.input = ""
		})
	}
}

// commitEdit closes any open editor, keeping the draft if it says anything.
// An empty draft is "leave it alone", not a delete: deleting is the swipe or
// the ×, and losing a todo to a stray double-click and a held backspace would
// be a nasty surprise. Callers already hold SetState.
func (s *todoState) commitEdit() {
	if s.editing < 0 {
		return
	}
	if text := strings.TrimSpace(s.draft); text != "" && s.editing < len(s.items) {
		s.items[s.editing].text = text
	}
	s.editing, s.draft = -1, ""
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
		Title:      "gophics todo",
		Size:       geom.Size{W: 440, H: 560},
		Background: BG,
		Font:       goregular.TTF,
	})
	if err != nil {
		log.Fatal(err)
	}
}
