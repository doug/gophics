package widget

import (
	"strings"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/text"
)

// TextField is an editable single-line text input: click-to-caret, drag and
// shift-arrow selection, grapheme-aware editing, clipboard shortcuts, and
// caret-keep-visible scrolling. It renders only text, selection, and caret —
// wrap it in Decorated/Padding for chrome, using OnFocus for focus styling.
//
// It is a controlled component: Value is the source of truth. OnChange
// reports edits; the parent re-renders with the new Value (set Value to ""
// after OnSubmit to clear the field).
//
// Known limits (PLAN.md §6.1): LTR caret geometry, no IME composition UI
// yet (committed text works), single line.
type TextField struct {
	Value       string
	Placeholder string
	Size        float32 // 0 → 14
	OnChange    func(string)
	OnSubmit    func(string)
	OnFocus     func(bool)

	TextColor        paint.Color
	PlaceholderColor paint.Color
	CaretColor       paint.Color
	SelectionColor   paint.Color
}

func (f TextField) size() float32 {
	if f.Size == 0 {
		return 14
	}
	return f.Size
}

func (f TextField) CreateState() State { return &textFieldState{} }

type textFieldState struct {
	StateBase[TextField]
	ed      text.Editor
	focused bool
	scrollX float32
}

func (s *textFieldState) change(ctx Ctx) {
	if f := s.W(); f.OnChange != nil {
		f.OnChange(s.ed.Text())
	}
	s.SetState(nil)
}

func (s *textFieldState) line(ctx Ctx) text.Line {
	return ctx.Painter().Shape(s.ed.Text(), s.W().size())
}

func (s *textFieldState) Build(ctx Ctx) Widget {
	f := s.W()
	if f.Value != s.ed.Text() {
		s.ed.SetText(f.Value)
		s.ed.End(false)
	}

	onKey := func(k shell.Key) {
		if k.Kind != shell.KeyPress {
			return
		}
		shift := k.Mods&shell.ModShift != 0
		switch k.Code {
		case shell.KeyLeft:
			s.ed.Move(-1, shift)
			s.SetState(nil)
		case shell.KeyRight:
			s.ed.Move(1, shift)
			s.SetState(nil)
		case shell.KeyHome:
			s.ed.Home(shift)
			s.SetState(nil)
		case shell.KeyEnd:
			s.ed.End(shift)
			s.SetState(nil)
		case shell.KeyBackspace:
			s.ed.DeleteBackward()
			s.change(ctx)
		case shell.KeyDelete:
			s.ed.DeleteForward()
			s.change(ctx)
		case shell.KeyEnter:
			if f.OnSubmit != nil {
				f.OnSubmit(s.ed.Text())
			}
		case shell.KeyEscape:
			s.ed.MoveTo(s.ed.Caret, false) // collapse selection
			s.SetState(nil)
		case shell.KeyA:
			if k.Mods.Command() {
				s.ed.SelectAll()
				s.SetState(nil)
			}
		case shell.KeyC:
			if k.Mods.Command() && s.ed.HasSelection() {
				if cb := ctx.Clipboard(); cb != nil {
					_ = cb.ClipboardWrite(s.ed.SelectedText())
				}
			}
		case shell.KeyX:
			if k.Mods.Command() && s.ed.HasSelection() {
				if cb := ctx.Clipboard(); cb != nil {
					_ = cb.ClipboardWrite(s.ed.SelectedText())
				}
				s.ed.Insert("")
				s.change(ctx)
			}
		case shell.KeyV:
			if k.Mods.Command() {
				if cb := ctx.Clipboard(); cb != nil {
					if t, err := cb.ClipboardRead(); err == nil && t != "" {
						s.ed.Insert(sanitize(t))
						s.change(ctx)
					}
				}
			}
		}
	}

	onText := func(t string) {
		if t = sanitize(t); t != "" {
			s.ed.Insert(t)
			s.change(ctx)
		}
	}

	indexAt := func(x float32) int {
		return s.line(ctx).IndexAt(x + s.scrollX)
	}

	return Interactive{
		Handler: Handler{
			OnPress: func(p geom.Pt) {
				s.ed.MoveTo(indexAt(p.X), false)
				s.SetState(nil)
			},
			OnDrag: func(p, _ geom.Pt) {
				s.ed.MoveTo(indexAt(p.X), true)
				s.SetState(nil)
			},
			OnText: onText,
			OnKey:  onKey,
			OnFocus: func(v bool) {
				s.focused = v
				if f.OnFocus != nil {
					f.OnFocus(v)
				}
				s.SetState(nil)
			},
		},
		Child: fieldView{state: s},
	}
}

func sanitize(t string) string {
	return strings.Map(func(r rune) rune {
		if r < ' ' || r == 0x7f {
			return -1
		}
		return r
	}, t)
}

// fieldView is the render widget painting the field content from state.
type fieldView struct {
	state *textFieldState
}

func (v fieldView) createBox(ctx Ctx) layout.Box {
	return &fieldBox{state: v.state, painter: ctx.Painter()}
}
func (v fieldView) updateBox(ctx Ctx, b layout.Box) {
	fb := b.(*fieldBox)
	fb.state, fb.painter = v.state, ctx.Painter()
}
func (v fieldView) childWidgets() []Widget          { return nil }
func (v fieldView) attach(layout.Box, []layout.Box) {}

type fieldBox struct {
	state   *textFieldState
	painter *paint.Painter
	size    geom.Size
}

func (b *fieldBox) Layout(cs layout.Constraints) geom.Size {
	f := b.state.W()
	m := b.painter.Metrics(f.size())
	want := geom.Size{
		W: b.painter.MeasureWidth(b.state.ed.Text(), f.size()),
		H: m.Ascent + m.Descent,
	}
	// A text field fills its available width (clicks in the empty area
	// must land on the field); it shrink-wraps only when unbounded.
	if cs.BoundedW() {
		want.W = cs.Max.W
	}
	b.size = cs.Constrain(want)

	// Keep the caret visible: adjust scrollX so it lies within the box.
	caretX := b.painter.Shape(b.state.ed.Text(), f.size()).CaretX(b.state.ed.Caret)
	if caretX-b.state.scrollX > b.size.W-2 {
		b.state.scrollX = caretX - b.size.W + 2
	}
	if caretX-b.state.scrollX < 0 {
		b.state.scrollX = caretX
	}
	if b.state.scrollX < 0 {
		b.state.scrollX = 0
	}
	return b.size
}

func (b *fieldBox) Size() geom.Size { return b.size }

func (b *fieldBox) Paint(c paint.Canvas, at geom.Pt) {
	f := b.state.W()
	sz := f.size()
	m := b.painter.Metrics(sz)
	line := b.painter.Shape(b.state.ed.Text(), sz)
	origin := geom.Pt{X: at.X - b.state.scrollX, Y: at.Y}
	baseline := at.Y + m.Ascent

	c.PushClip(geom.Rect{Min: at, Max: at.Add(b.size.Pt())})

	if start, end := b.state.ed.Selection(); start != end {
		x0 := origin.X + line.CaretX(start)
		x1 := origin.X + line.CaretX(end)
		c.FillRect(geom.Rect{
			Min: geom.Pt{X: x0, Y: at.Y},
			Max: geom.Pt{X: x1, Y: at.Y + b.size.H},
		}, f.SelectionColor)
	}

	if b.state.ed.Len() == 0 && f.Placeholder != "" {
		c.Text(f.Placeholder, geom.Pt{X: origin.X, Y: baseline}, sz, f.PlaceholderColor)
	} else {
		c.Text(b.state.ed.Text(), geom.Pt{X: origin.X, Y: baseline}, sz, f.TextColor)
	}

	if b.state.focused && !b.state.ed.HasSelection() {
		x := origin.X + line.CaretX(b.state.ed.Caret)
		c.Line(geom.Pt{X: x, Y: at.Y}, geom.Pt{X: x, Y: at.Y + b.size.H}, 1.5, f.CaretColor)
	}

	c.PopClip()
}

func (b *fieldBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H {
		*hits = append(*hits, layout.Hit{Box: b, Pos: p})
	}
}
