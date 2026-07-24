// Command todo is a hand-rolled todo app: the first vertical slice through
// the whole gossamer stack — shell events, paint (gg CPU rasterizer → GPU
// surface), text, and hit testing.
//
// Everything here is deliberately manual (layout arithmetic, hit tests,
// invalidation). As the layout/widget layers land (PLAN.md M2–M3), this
// example shrinks until it is only widget declarations; its diff history is
// the framework's progress report.
package main

import (
	"fmt"
	"log"
	"strings"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/shell/desktop"
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

const (
	pad    = 20
	fieldH = 44
	rowH   = 44
	rowGap = 8
)

type item struct {
	text string
	done bool
}

type app struct {
	painter *paint.Painter
	size    geom.Size

	items []item
	input string
	hover int // hovered row index, -1 for none

	// rects recomputed every frame, used for hit testing between frames
	fieldRect geom.Rect
	rowRects  []geom.Rect
}

func newApp() *app {
	p := paint.NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		log.Fatal(err)
	}
	return &app{
		painter: p,
		hover:   -1,
		items: []item{
			{text: "Plan the port", done: true},
			{text: "Open a window", done: true},
			{text: "Build the todo example", done: false},
			{text: "Grow widgets until this file shrinks", done: false},
		},
	}
}

func (a *app) Frame(w shell.Window, f shell.Frame, dt float64) {
	a.size = f.Size()
	c := a.painter.Begin(f)
	a.layout()
	a.draw(c)
	if err := a.painter.End(f); err != nil {
		log.Printf("paint: %v", err)
	}
	// No Invalidate here: rendering is on-demand. Events that change state
	// request the next frame; idle means zero GPU work.
}

func (a *app) layout() {
	content := geom.RectFromSize(a.size)
	content = geom.InsetsAll(pad).Inset(content)

	a.fieldRect = geom.RectXYWH(content.Min.X, content.Min.Y+40, content.Dx(), fieldH)

	a.rowRects = a.rowRects[:0]
	y := a.fieldRect.Max.Y + pad
	for range a.items {
		a.rowRects = append(a.rowRects, geom.RectXYWH(content.Min.X, y, content.Dx(), rowH))
		y += rowH + rowGap
	}
}

func (a *app) draw(c *paint.Canvas) {
	c.Clear(colBg)

	c.Text("gossamer · todo", geom.Pt{X: pad, Y: pad + 8}, 15, colDim)

	// Input field.
	c.FillRRect(a.fieldRect, 8, colCard)
	c.StrokeRRect(a.fieldRect, 8, 1, colAccent)
	tx := a.fieldRect.Min.X + 14
	ty := a.fieldRect.Min.Y + fieldH/2 + 5
	if a.input == "" {
		c.Text("What needs doing?  (Enter adds)", geom.Pt{X: tx, Y: ty}, 14, colDim)
	} else {
		c.Text(a.input, geom.Pt{X: tx, Y: ty}, 14, colText)
	}
	// Caret.
	cw, _ := c.MeasureText(a.input, 14)
	caretX := tx + cw + 1
	c.Line(geom.Pt{X: caretX, Y: ty - 12}, geom.Pt{X: caretX, Y: ty + 3}, 1.5, colAccent)

	// Rows.
	for i, r := range a.rowRects {
		it := a.items[i]
		bg := colCard
		if i == a.hover {
			bg = colCardHov
		}
		c.FillRRect(r, 8, bg)

		// Checkbox.
		cb := a.checkboxRect(r)
		if it.done {
			c.FillRRect(cb, 5, colAccent)
			// Check mark.
			c.Line(geom.Pt{X: cb.Min.X + 4, Y: cb.Min.Y + 10}, geom.Pt{X: cb.Min.X + 8, Y: cb.Min.Y + 14}, 2, colBg)
			c.Line(geom.Pt{X: cb.Min.X + 8, Y: cb.Min.Y + 14}, geom.Pt{X: cb.Min.X + 16, Y: cb.Min.Y + 5}, 2, colBg)
		} else {
			c.StrokeRRect(cb, 5, 1.5, colDim)
		}

		// Label.
		col := colText
		if it.done {
			col = colDim
		}
		lx := cb.Max.X + 12
		ly := r.Min.Y + rowH/2 + 5
		c.Text(it.text, geom.Pt{X: lx, Y: ly}, 14, col)
		if it.done {
			lw, _ := c.MeasureText(it.text, 14)
			c.Line(geom.Pt{X: lx, Y: ly - 4}, geom.Pt{X: lx + lw, Y: ly - 4}, 1, colDim)
		}

		// Delete button on hover.
		if i == a.hover {
			d := a.deleteRect(r)
			c.Text("×", geom.Pt{X: d.Min.X + 6, Y: d.Min.Y + 16}, 16, colDanger)
		}
	}

	// Footer.
	left := 0
	for _, it := range a.items {
		if !it.done {
			left++
		}
	}
	footer := fmt.Sprintf("%d left · click row to toggle · hover shows delete", left)
	fy := a.size.H - pad + 4
	c.Text(footer, geom.Pt{X: pad, Y: fy}, 12, colDim)
}

func (a *app) checkboxRect(row geom.Rect) geom.Rect {
	return geom.RectXYWH(row.Min.X+12, row.Min.Y+(rowH-20)/2, 20, 20)
}

func (a *app) deleteRect(row geom.Rect) geom.Rect {
	return geom.RectXYWH(row.Max.X-32, row.Min.Y+(rowH-20)/2, 20, 20)
}

func (a *app) Event(w shell.Window, e shell.Event) {
	switch e := e.(type) {
	case shell.Text:
		s := strings.Map(func(r rune) rune {
			if r < ' ' {
				return -1
			}
			return r
		}, e.S)
		if s != "" {
			a.input += s
			w.Invalidate()
		}

	case shell.Key:
		if e.Kind != shell.KeyPress {
			return
		}
		switch e.Code {
		case shell.KeyEnter:
			if t := strings.TrimSpace(a.input); t != "" {
				a.items = append(a.items, item{text: t})
				a.input = ""
				w.Invalidate()
			}
		case shell.KeyBackspace:
			if a.input != "" {
				r := []rune(a.input)
				a.input = string(r[:len(r)-1])
				w.Invalidate()
			}
		case shell.KeyEscape:
			if a.input != "" {
				a.input = ""
				w.Invalidate()
			}
		}

	case shell.Pointer:
		switch e.Kind {
		case shell.PointerMove:
			hover := -1
			for i, r := range a.rowRects {
				if r.Contains(e.Pos) {
					hover = i
					break
				}
			}
			if hover != a.hover {
				a.hover = hover
				w.Invalidate()
			}
		case shell.PointerDown:
			if e.Button != 0 {
				return
			}
			for i, r := range a.rowRects {
				if !r.Contains(e.Pos) {
					continue
				}
				if a.deleteRect(r).Contains(e.Pos) {
					a.items = append(a.items[:i], a.items[i+1:]...)
					a.hover = -1
				} else {
					a.items[i].done = !a.items[i].done
				}
				w.Invalidate()
				return
			}
		}

	case shell.Resize:
		w.Invalidate()
	}
}

func main() {
	err := desktop.Run(newApp(), shell.Config{
		Title:     "gossamer todo",
		Size:      geom.Size{W: 440, H: 560},
		Resizable: true,
	})
	if err != nil {
		log.Fatal(err)
	}
}
