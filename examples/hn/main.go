// Command hn is a HackerNews client: the M-HN driving application
// (PLAN.md §7.1). Lazy story feed with fling scrolling, Navigator-driven
// pages with slide transitions, rich comments with tappable links, async
// loading over the real Firebase API — on desktop and web from one
// codebase.
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

var (
	colBg     = paint.RGB(0.96, 0.96, 0.94)
	colBar    = paint.RGB(1.00, 0.40, 0.00) // HN orange
	colCard   = paint.RGB(1, 1, 1)
	colTitle  = paint.RGB(0.10, 0.10, 0.10)
	colMeta   = paint.RGB(0.51, 0.51, 0.49)
	colOnBar  = paint.RGB(1, 1, 1)
	colAccent = paint.RGB(1.00, 0.40, 0.00)
	colLink   = paint.RGB(0.05, 0.35, 0.75)

	commentStyle = spanStyle{Text: colTitle, Link: colLink, Emph: colMeta}
)

// HN is the root widget: the Navigator over the feed.
type HN struct {
	API      API
	PageSize int // stories to load (0 → 60)
}

func (h HN) pageSize() int {
	if h.PageSize == 0 {
		return 60
	}
	return h.PageSize
}

func (h HN) Build(widget.Ctx) widget.Widget {
	return widget.Navigator{Home: feedPage{API: h.API, N: h.pageSize()}}
}

func header(title string, lead widget.Widget) widget.Widget {
	if lead == nil {
		lead = widget.Text{S: "Y", Size: 15, Color: colOnBar}
	}
	return widget.Decorated{
		Color: colBar,
		Child: widget.Padding{Insets: geom.InsetsSymmetric(12, 10),
			Child: widget.Row(
				lead,
				widget.Sized{W: 10},
				widget.Expand(widget.Text{S: title, Size: 15, Color: colOnBar}),
			)},
	}
}

func backButton(ctx widget.Ctx) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	return widget.Interactive{
		Handler: widget.Handler{OnTap: nav.Pop},
		Child: widget.Padding{Insets: geom.InsetsSymmetric(6, 4),
			Child: widget.Text{S: "‹ Back", Size: 15, Color: colOnBar}},
	}
}

func page(headerW, body widget.Widget) widget.Widget {
	col := widget.Column(headerW, widget.Expand(body))
	col.CrossAlign = layout.CrossStretch
	// Pages carry their own opaque background so slide transitions cover
	// the page beneath.
	return widget.Decorated{Color: colBg, Child: col}
}

// feedPage lists top stories.
type feedPage struct {
	API API
	N   int
}

func (f feedPage) CreateState() widget.State { return &feedState{} }

type feedState struct {
	widget.StateBase[feedPage]
	stories []Item
	loading bool
	err     string
}

// stateHook lets tests observe the mounted feed state.
var stateHook func(*feedState)

func (s *feedState) Init(ctx widget.Ctx) {
	if stateHook != nil {
		stateHook(s)
	}
	s.loading = true
	post := ctx.Post()
	f := s.W()
	go func() {
		ids, err := f.API.TopStories()
		if err != nil {
			post(func() { s.SetState(func() { s.loading, s.err = false, err.Error() }) })
			return
		}
		if len(ids) > f.N {
			ids = ids[:f.N]
		}
		stories := make([]Item, 0, len(ids))
		for _, id := range ids {
			if it, err := f.API.Item(id); err == nil && it.Title != "" {
				stories = append(stories, it)
			}
		}
		post(func() { s.SetState(func() { s.loading, s.stories = false, stories }) })
	}()
}

func (s *feedState) Build(ctx widget.Ctx) widget.Widget {
	var body widget.Widget
	switch {
	case s.loading:
		body = widget.Center(widget.Text{S: "loading…", Color: colMeta})
	case s.err != "":
		body = widget.Center(widget.Text{S: s.err, Wrap: true, Color: colMeta})
	default:
		nav := widget.MustOf[widget.Nav](ctx)
		body = widget.LazyList{
			Count:           len(s.stories),
			EstimatedExtent: 66,
			Build:           func(i int) widget.Widget { return s.storyRow(nav, i) },
		}
	}
	return page(header("Hacker News", nil), body)
}

func (s *feedState) storyRow(nav widget.Nav, i int) widget.Widget {
	st := s.stories[i]
	meta := fmt.Sprintf("%d points · %s · %d comments", st.Score, st.By, st.Descendants)
	if d := domain(st.URL); d != "" {
		meta = d + " · " + meta
	}
	title := widget.Column(
		widget.Text{S: st.Title, Size: 15, Color: colTitle, Wrap: true},
		widget.Sized{H: 4},
		widget.Text{S: meta, Size: 12, Color: colMeta},
	)
	title.CrossAlign = layout.CrossStart
	row := widget.Row(
		widget.Sized{W: 34, Child: widget.Text{S: fmt.Sprintf("%d.", i + 1), Size: 13, Color: colMeta}},
		widget.Expand(title),
	)
	row.CrossAlign = layout.CrossStart
	api := s.W().API
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() { nav.Push(threadPage{API: api, Story: st}) }},
		Child: widget.Decorated{Color: colCard,
			Child: widget.Padding{Insets: geom.InsetsSymmetric(12, 10), Child: row}},
	}
}

// threadPage shows one story's comments.
type threadPage struct {
	API   API
	Story Item
}

func (t threadPage) CreateState() widget.State { return &threadState{} }

type threadState struct {
	widget.StateBase[threadPage]
	comments []Comment
	loading  bool
}

func (s *threadState) Init(ctx widget.Ctx) {
	s.loading = true
	post := ctx.Post()
	t := s.W()
	go func() {
		comments := loadComments(t.API, t.Story, 80)
		post(func() { s.SetState(func() { s.comments, s.loading = comments, false }) })
	}()
}

func (s *threadState) Build(ctx widget.Ctx) widget.Widget {
	st := s.W().Story
	var body widget.Widget
	if s.loading {
		body = widget.Center(widget.Text{S: "loading comments…", Color: colMeta})
	} else {
		n := len(s.comments)
		openURL := func(u string) { _ = ctx.OpenURL(u) }
		body = widget.LazyList{
			Count:           n + 1,
			EstimatedExtent: 90,
			Build: func(i int) widget.Widget {
				if i == 0 {
					return storyHeaderCell(st, openURL)
				}
				return commentRow(s.comments[i-1], openURL)
			},
		}
	}
	return page(header(st.Title, backButton(ctx)), body)
}

func storyHeaderCell(st Item, openURL func(string)) widget.Widget {
	meta := fmt.Sprintf("%d points by %s · %d comments", st.Score, st.By, st.Descendants)
	kids := []widget.Widget{
		widget.Text{S: st.Title, Size: 17, Color: colTitle, Wrap: true},
		widget.Sized{H: 6},
		widget.Text{S: meta, Size: 12, Color: colMeta},
	}
	if st.URL != "" {
		kids = append(kids, widget.Sized{H: 6}, widget.Rich{
			Spans:  []layout.RichSpan{{Text: domain(st.URL) + " ↗", Color: colLink, Link: st.URL}},
			Size:   13,
			OnLink: openURL,
		})
	}
	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStart
	return widget.Decorated{Color: colCard,
		Child: widget.Padding{All: 14, Child: col}}
}

func commentRow(c Comment, openURL func(string)) widget.Widget {
	body := widget.Column(
		widget.Text{S: c.By, Size: 12, Color: colAccent},
		widget.Sized{H: 4},
		widget.Rich{Spans: parseSpans(c.Text, commentStyle), Size: 13, OnLink: openURL},
	)
	body.CrossAlign = layout.CrossStart
	return widget.Padding{
		Insets: geom.Insets{Top: 6, Left: 12 + float32(c.Depth)*16, Right: 12},
		Child: widget.Decorated{Color: colCard,
			Child: widget.Padding{All: 10, Child: body}},
	}
}

func main() {
	err := app.Run(HN{API: newLiveAPI()}, app.Config{
		Title:      "gossamer · hn",
		Size:       geom.Size{W: 480, H: 720},
		Background: colBg,
		Font:       goregular.TTF,
	})
	if err != nil {
		log.Fatal(err)
	}
}
