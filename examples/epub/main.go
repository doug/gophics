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
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// book is parsed once at startup from the bundled sample epub.
var book = loadBook()

// BG is the window background used at Start, before a widget context exists (the
// mobile bind / app.Config passes it as Config.Background). Inside the tree every
// color comes from theme.Of(ctx), so the reader follows the platform light/dark
// scheme for free — the light theme's warm off-white doubles as the reading paper.
var BG = theme.Light().Bg

// maxColW keeps the text column at a comfortable measure on wide windows.
const maxColW = 680

type App struct{}

func (App) Build(ctx widget.Ctx) widget.Widget {
	// Resolve the theme from the platform color scheme and provide it to the tree,
	// so every page below reads colors with theme.Of(ctx) and the whole reader
	// follows light/dark automatically.
	th := theme.Auto(ctx)
	// EPUB_CHAPTER deep-links straight to a chapter — for screenshots/thumbnails.
	home := widget.Widget(library{})
	if v := os.Getenv("EPUB_CHAPTER"); v != "" {
		if i, err := strconv.Atoi(v); err == nil && i >= 0 && i < len(book.Chapters) {
			home = reader{idx: i}
		}
	}
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Fill{Color: th.Bg, Child: frame(widget.Navigator{Home: home})},
	}
}

// frame centres content and caps its width for readability, filling narrower
// screens. The reading background (provided by the root Fill) shows on either side.
func frame(child widget.Widget) widget.Widget {
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		w := cs.Max.W
		if w > maxColW {
			w = maxColW
		}
		return widget.Flex{Axis: layout.Horizontal, MainAlign: layout.MainCenter, CrossAlign: layout.CrossStretch,
			Children: []widget.Widget{widget.Sized{W: w, Child: child}}}
	}}
}

// --- library: title page + table of contents (Navigator Home) ---

type library struct{}

func (library) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	nav := widget.MustOf[widget.Nav](ctx)
	kids := []widget.Widget{
		widget.Padding{Insets: geom.Insets{Top: 56, Bottom: 8}, Child: widget.Text{S: book.Title, Font: theme.FontBold, Size: th.Type.Display, Color: th.Text, Wrap: true}},
		widget.Padding{Insets: geom.Insets{Bottom: 40}, Child: widget.Text{S: "by " + book.Author, Size: th.Type.Body, Color: th.Muted}},
		widget.Text{S: "CONTENTS", Font: theme.FontBold, Size: th.Type.Label, Color: th.Primary},
		widget.Padding{Insets: geom.Insets{Top: 8}, Child: divider(th)},
	}
	for i, ch := range book.Chapters {
		idx := i
		kids = append(kids, tocRow(th, i+1, ch.Title, func() { nav.Push(reader{idx: idx}) }), divider(th))
	}
	return widget.Fill{Color: th.Bg, Child: widget.Scroll{Child: widget.Padding{
		Insets: geom.InsetsSymmetric(28, 12),
		Child:  widget.Flex{CrossAlign: layout.CrossStretch, Children: kids},
	}}}
}

func tocRow(th theme.Theme, n int, title string, onTap func()) widget.Widget {
	return widget.Interactive{Handler: widget.Handler{OnTap: onTap}, Child: widget.Padding{
		Insets: geom.InsetsSymmetric(2, 15),
		Child: widget.Row(
			widget.Sized{W: 34, Child: widget.Text{S: fmt.Sprintf("%d", n), Size: th.Type.Body, Color: th.Primary}},
			widget.Expand(widget.Text{S: title, Size: th.Type.Heading, Color: th.Text, Wrap: true}),
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
	th := theme.Of(ctx)
	nav := widget.MustOf[widget.Nav](ctx)
	ch := book.Chapters[s.idx]

	top := widget.Padding{Insets: geom.InsetsSymmetric(20, 14), Child: widget.Row(
		widget.Interactive{Handler: widget.Handler{OnTap: nav.Pop}, Child: widget.Text{S: "‹  Contents", Size: th.Type.Body, Color: th.Primary}},
		widget.Spacer(),
		widget.Text{S: fmt.Sprintf("%d / %d", s.idx+1, len(book.Chapters)), Size: th.Type.Label, Color: th.Muted},
	)}

	var body []widget.Widget
	for _, b := range ch.Blocks {
		if b.Heading {
			body = append(body, widget.Padding{Insets: geom.Insets{Top: 10, Bottom: 14}, Child: widget.Text{S: b.Text, Font: theme.FontBold, Size: th.Type.Title, Color: th.Text, Wrap: true}})
		} else {
			body = append(body, widget.Padding{Insets: geom.Insets{Bottom: 18}, Child: widget.Text{S: b.Text, Size: th.Type.Body, Color: th.Text, Wrap: true}})
		}
	}
	// Key the scroll on the chapter so moving to another chapter starts at the top.
	page := widget.WithKey{Key: s.idx, Child: widget.Scroll{Child: widget.Padding{
		Insets: geom.Insets{Left: 28, Right: 28, Top: 8, Bottom: 40},
		Child:  widget.Flex{CrossAlign: layout.CrossStretch, Children: body},
	}}}

	bottom := widget.Padding{Insets: geom.InsetsSymmetric(20, 14), Child: widget.Row(
		navBtn(th, "‹ Prev", s.idx > 0, func() { s.go2(-1) }),
		widget.Spacer(),
		navBtn(th, "Next ›", s.idx < len(book.Chapters)-1, func() { s.go2(1) }),
	)}

	return widget.Fill{Color: th.Bg, Child: widget.Flex{CrossAlign: layout.CrossStretch, Children: []widget.Widget{
		top, divider(th), widget.Expand(page), divider(th), bottom,
	}}}
}

func navBtn(th theme.Theme, label string, enabled bool, onTap func()) widget.Widget {
	col := th.Muted
	if enabled {
		col = th.Primary
	}
	t := widget.Text{S: label, Size: th.Type.Body, Color: col}
	if !enabled {
		return t
	}
	return widget.Interactive{Handler: widget.Handler{OnTap: onTap}, Child: t}
}

func divider(th theme.Theme) widget.Widget {
	return widget.Sized{H: 1, Child: widget.Decorated{Color: th.Border}}
}

func main() {
	if err := app.Run(App{}, app.Config{
		Title:        "Reader",
		Size:         geom.Size{W: 460, H: 720},
		Background:   BG,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}); err != nil {
		log.Fatal(err)
	}
}
