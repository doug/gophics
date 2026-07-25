// Command hn is a HackerNews client: the M-HN driving application
// (PLAN.md §7.1). Lazy story feed with fling scrolling, tap-through to
// comment threads, async loading over the real Firebase API — on desktop
// and web from one codebase.
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
)

// HN is the root widget.
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

func (HN) CreateState() widget.State { return &hnState{} }

type hnState struct {
	widget.StateBase[HN]
	post func(func())

	stories []Item
	loading bool
	err     string

	// non-nil: viewing a story's comments
	open     *Item
	comments []Comment
	loadingC bool
}

// stateHook lets tests observe the mounted state.
var stateHook func(*hnState)

func (s *hnState) Init(ctx widget.Ctx) {
	if stateHook != nil {
		stateHook(s)
	}
	s.post = ctx.Post()
	s.loading = true
	api, n := s.W().API, s.W().pageSize()
	go func() {
		ids, err := api.TopStories()
		if err != nil {
			s.post(func() { s.SetState(func() { s.loading, s.err = false, err.Error() }) })
			return
		}
		if len(ids) > n {
			ids = ids[:n]
		}
		stories := make([]Item, 0, len(ids))
		for _, id := range ids {
			if it, err := api.Item(id); err == nil && it.Title != "" {
				stories = append(stories, it)
			}
		}
		s.post(func() { s.SetState(func() { s.loading, s.stories = false, stories }) })
	}()
}

func (s *hnState) openStory(st Item) {
	s.SetState(func() { s.open, s.comments, s.loadingC = &st, nil, true })
	api := s.W().API
	go func() {
		comments := loadComments(api, st, 80)
		s.post(func() { s.SetState(func() { s.comments, s.loadingC = comments, false }) })
	}()
}

func (s *hnState) back() {
	s.SetState(func() { s.open, s.comments = nil, nil })
}

func (s *hnState) Build(widget.Ctx) widget.Widget {
	body := s.feed()
	title := "Hacker News"
	showBack := false
	if s.open != nil {
		body = s.thread()
		title = s.open.Title
		showBack = true
	}
	col := widget.Column(
		s.header(title, showBack),
		widget.Expand(body),
	)
	col.CrossAlign = layout.CrossStretch
	return col
}

func (s *hnState) header(title string, back bool) widget.Widget {
	var lead widget.Widget = widget.Text{S: "Y", Size: 15, Color: colOnBar}
	if back {
		lead = widget.Interactive{
			Handler: widget.Handler{OnTap: s.back},
			Child:   widget.Padding{Insets: geom.InsetsSymmetric(6, 4), Child: widget.Text{S: "‹ Back", Size: 15, Color: colOnBar}},
		}
	}
	return widget.Semantics{Label: "header", Child: widget.Decorated{
		Color: colBar,
		Child: widget.Padding{Insets: geom.InsetsSymmetric(12, 10),
			Child: widget.Row(
				lead,
				widget.Sized{W: 10},
				widget.Expand(widget.Text{S: title, Size: 15, Color: colOnBar}),
			)},
	}}
}

func (s *hnState) feed() widget.Widget {
	if s.loading {
		return widget.Center(widget.Text{S: "loading…", Color: colMeta})
	}
	if s.err != "" {
		return widget.Center(widget.Text{S: s.err, Wrap: true, Color: colMeta})
	}
	return widget.LazyList{
		Count:           len(s.stories),
		EstimatedExtent: 66,
		Build:           func(i int) widget.Widget { return s.storyRow(i) },
	}
}

func (s *hnState) storyRow(i int) widget.Widget {
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
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() { s.openStory(st) }},
		Child: widget.Decorated{Color: colCard,
			Child: widget.Padding{Insets: geom.InsetsSymmetric(12, 10), Child: row}},
	}
}

func (s *hnState) thread() widget.Widget {
	if s.loadingC {
		return widget.Center(widget.Text{S: "loading comments…", Color: colMeta})
	}
	st := *s.open
	n := len(s.comments)
	return widget.LazyList{
		Count:           n + 1, // story header cell + comments
		EstimatedExtent: 90,
		Build: func(i int) widget.Widget {
			if i == 0 {
				return s.storyHeaderCell(st)
			}
			return s.commentRow(s.comments[i-1])
		},
	}
}

func (s *hnState) storyHeaderCell(st Item) widget.Widget {
	meta := fmt.Sprintf("%d points by %s · %d comments", st.Score, st.By, st.Descendants)
	col := widget.Column(
		widget.Text{S: st.Title, Size: 17, Color: colTitle, Wrap: true},
		widget.Sized{H: 6},
		widget.Text{S: meta, Size: 12, Color: colMeta},
	)
	col.CrossAlign = layout.CrossStart
	return widget.Decorated{Color: colCard,
		Child: widget.Padding{All: 14, Child: col}}
}

func (s *hnState) commentRow(c Comment) widget.Widget {
	body := widget.Column(
		widget.Text{S: c.By, Size: 12, Color: colAccent},
		widget.Sized{H: 4},
		widget.Text{S: plainText(c.Text), Size: 13, Color: colTitle, Wrap: true},
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
