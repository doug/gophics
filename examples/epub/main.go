// Command epub is a self-contained EPUB reader: it builds a small sample book as
// a real .epub (zip + OPF + XHTML) in memory, parses it through the full pipeline
// (see epub.go), and renders a typeset reader — a title page, a chapter table of
// contents, and a scrollable chapter view with prev/next. No network, no files:
// it runs identically on desktop, web, and mobile, and shows off gophics' own
// text layout (wrapping, headings, long-form reading).
//
//	go run ./examples/epub
package main

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// book is parsed once at startup from the bundled sample epub.
var book = loadBook()

// Warm-paper palette.
var (
	paper  = paint.RGB(0.96, 0.94, 0.89)
	ink    = paint.RGB(0.17, 0.15, 0.13)
	muted  = paint.RGB(0.46, 0.42, 0.37)
	accent = paint.RGB(0.60, 0.33, 0.20)
	rule   = paint.RGB(0.84, 0.80, 0.72)
)

// maxColW keeps the text column at a comfortable measure on wide windows.
const maxColW = 680

type App struct{}

func (App) Build(ctx widget.Ctx) widget.Widget {
	// EPUB_CHAPTER deep-links straight to a chapter — for screenshots/thumbnails.
	home := widget.Widget(library{})
	if v := os.Getenv("EPUB_CHAPTER"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(book.Chapters) {
			home = reader{idx: i}
		}
	}
	return frame(widget.Navigator{Home: home})
}

// frame centres content and caps its width for readability, filling narrower
// screens. The paper background shows on either side.
func frame(child widget.Widget) widget.Widget {
	return widget.Fill{Color: paper, Child: widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		w := cs.Max.W
		if w > maxColW {
			w = maxColW
		}
		return widget.Flex{Axis: layout.Horizontal, MainAlign: layout.MainCenter, CrossAlign: layout.CrossStretch,
			Children: []widget.Widget{widget.Sized{W: w, Child: child}}}
	}}}
}

// --- library: title page + table of contents (Navigator Home) ---

type library struct{}

func (library) Build(ctx widget.Ctx) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	kids := []widget.Widget{
		widget.Padding{Insets: geom.Insets{Top: 56, Bottom: 8}, Child: widget.Text{S: book.Title, Font: "bold", Size: 30, Color: ink, Wrap: true}},
		widget.Padding{Insets: geom.Insets{Bottom: 40}, Child: widget.Text{S: "by " + book.Author, Size: 16, Color: muted}},
		widget.Text{S: "CONTENTS", Font: "bold", Size: 12, Color: accent},
		widget.Padding{Insets: geom.Insets{Top: 8}, Child: divider()},
	}
	for i, ch := range book.Chapters {
		idx := i
		kids = append(kids, tocRow(i+1, ch.Title, func() { nav.Push(reader{idx: idx}) }), divider())
	}
	return widget.Fill{Color: paper, Child: widget.Scroll{Child: widget.Padding{
		Insets: geom.InsetsSymmetric(28, 12),
		Child:  widget.Flex{CrossAlign: layout.CrossStretch, Children: kids},
	}}}
}

func tocRow(n int, title string, onTap func()) widget.Widget {
	return widget.Interactive{Handler: widget.Handler{OnTap: onTap}, Child: widget.Padding{
		Insets: geom.InsetsSymmetric(2, 15),
		Child: widget.Row(
			widget.Sized{W: 34, Child: widget.Text{S: fmt.Sprintf("%d", n), Size: 15, Color: accent}},
			widget.Expand(widget.Text{S: title, Size: 17, Color: ink, Wrap: true}),
		),
	}}
}

// --- reader: one chapter, scrollable, with prev/next ---

type reader struct{ idx int }

func (reader) CreateState() widget.State { return &readerState{} }

type readerState struct {
	widget.StateBase[reader]
	idx int
}

func (s *readerState) Init(widget.Ctx) { s.idx = s.W().idx }

func (s *readerState) go2(delta int) {
	n := s.idx + delta
	if n >= 0 && n < len(book.Chapters) {
		s.SetState(func() { s.idx = n })
	}
}

func (s *readerState) Build(ctx widget.Ctx) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	ch := book.Chapters[s.idx]

	top := widget.Padding{Insets: geom.InsetsSymmetric(20, 14), Child: widget.Row(
		widget.Interactive{Handler: widget.Handler{OnTap: nav.Pop}, Child: widget.Text{S: "‹  Contents", Size: 15, Color: accent}},
		widget.Spacer(),
		widget.Text{S: fmt.Sprintf("%d / %d", s.idx+1, len(book.Chapters)), Size: 13, Color: muted},
	)}

	var body []widget.Widget
	for _, b := range ch.Blocks {
		if b.Heading {
			body = append(body, widget.Padding{Insets: geom.Insets{Top: 10, Bottom: 14}, Child: widget.Text{S: b.Text, Font: "bold", Size: 25, Color: ink, Wrap: true}})
		} else {
			body = append(body, widget.Padding{Insets: geom.Insets{Bottom: 18}, Child: widget.Text{S: b.Text, Size: 17, Color: ink, Wrap: true}})
		}
	}
	// Key the scroll on the chapter so moving to another chapter starts at the top.
	page := widget.WithKey{Key: s.idx, Child: widget.Scroll{Child: widget.Padding{
		Insets: geom.Insets{Left: 28, Right: 28, Top: 8, Bottom: 40},
		Child:  widget.Flex{CrossAlign: layout.CrossStretch, Children: body},
	}}}

	bottom := widget.Padding{Insets: geom.InsetsSymmetric(20, 14), Child: widget.Row(
		navBtn("‹ Prev", s.idx > 0, func() { s.go2(-1) }),
		widget.Spacer(),
		navBtn("Next ›", s.idx < len(book.Chapters)-1, func() { s.go2(1) }),
	)}

	return widget.Fill{Color: paper, Child: widget.Flex{CrossAlign: layout.CrossStretch, Children: []widget.Widget{
		top, divider(), widget.Expand(page), divider(), bottom,
	}}}
}

func navBtn(label string, enabled bool, onTap func()) widget.Widget {
	col := muted
	if enabled {
		col = accent
	}
	t := widget.Text{S: label, Size: 15, Color: col}
	if !enabled {
		return t
	}
	return widget.Interactive{Handler: widget.Handler{OnTap: onTap}, Child: t}
}

func divider() widget.Widget { return widget.Sized{H: 1, Child: widget.Decorated{Color: rule}} }

func main() {
	if err := app.Run(App{}, app.Config{
		Title:        "Reader",
		Size:         geom.Size{W: 460, H: 720},
		Background:   paper,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}); err != nil {
		log.Fatal(err)
	}
}
