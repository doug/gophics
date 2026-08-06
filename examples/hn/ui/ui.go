// Command hn is a HackerNews client: the M-HN driving application
// (PLAN.md §7.1). Lazy story feed with fling scrolling, Navigator-driven
// pages with slide transitions, rich comments with tappable links, async
// loading over the real Firebase API — on desktop and web from one
// codebase.
package ui

import (
	"fmt"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// colBg is the light-theme background. It is used before a widget context
// exists (app.Config.Background, via Background()) and by tests. Inside the
// tree every color comes from theme.Of(ctx), so the whole app follows the
// platform light/dark scheme automatically.
var colBg = theme.Light().Bg

// commentStyle is the light-theme span palette — used by tests and as a
// default. commentRow builds a themed palette per frame from theme.Of(ctx).
var commentStyle = spanStyle{
	Text: theme.Light().Text,
	Link: theme.Light().Primary,
	Emph: theme.Light().Muted,
}

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

func (h HN) Build(ctx widget.Ctx) widget.Widget {
	// Resolve the theme from the platform color scheme and provide it to the
	// tree, so every page below reads colors with theme.Of(ctx) and the whole
	// app follows light/dark automatically.
	th := theme.Auto(ctx)
	// Provide the API down the tree instead of threading it through every page.
	// Pages then carry only data (which story), so they're plain serializable
	// values — which is what lets `gophics dev` restore your navigation on a
	// hot-restart (see threadPage's registration below).
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Provide[API]{
			Value: h.API,
			Child: widget.Fill{Color: th.Bg, Child: widget.Navigator{Home: feedPage{N: h.pageSize()}}},
		},
	}
}

func init() {
	// Register the pushable page(s) so the Navigator's stack survives a
	// state-preserving hot-restart.
	widget.RegisterSnapshotType[threadPage]()
}

func header(th theme.Theme, title string, lead widget.Widget) widget.Widget {
	if lead == nil {
		lead = widget.Text{S: "Y", Size: th.Type.Heading, Color: th.OnPrimary}
	}
	return widget.Padding{Insets: geom.InsetsSymmetric(12, 10),
		Child: widget.Row(
			lead,
			widget.Sized{W: 10},
			widget.Expand(widget.Text{S: title, Font: "bold", Size: th.Type.Heading, Color: th.OnPrimary}),
		),
	}
}

func backButton(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	nav := widget.MustOf[widget.Nav](ctx)
	return widget.Interactive{
		Handler: widget.Handler{OnTap: nav.Pop},
		Child: widget.Padding{Insets: geom.InsetsSymmetric(6, 4),
			Child: widget.Text{S: "‹ Back", Size: th.Type.Body, Color: th.OnPrimary}},
	}
}

func page(ctx widget.Ctx, headerW, body widget.Widget) widget.Widget {
	th := theme.Of(ctx)
	// Pad by the platform safe areas (status bar / notch / keyboard); the
	// header bar's color extends behind the top inset.
	in := ctx.SafeInsets()
	col := widget.Column(
		widget.Decorated{Color: th.Primary, Child: widget.Padding{
			Insets: geom.Insets{Top: in.Top, Left: in.Left, Right: in.Right},
			Child:  headerW,
		}},
		widget.Expand(widget.Padding{
			Insets: geom.Insets{Left: in.Left, Right: in.Right, Bottom: in.Bottom},
			Child:  widget.SelectionArea{Child: body},
		}),
	)
	col.CrossAlign = layout.CrossStretch
	// Pages carry their own opaque background so slide transitions cover
	// the page beneath.
	content := widget.Decorated{Color: th.Bg, Child: col}

	// Responsive: on a wide viewport (desktop browser, large window, wide
	// terminal) center a comfortable reading column with gutters; on a narrow
	// one (phone, mobile web) fill the width unchanged.
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		const maxW = 720
		if !cs.BoundedW() || cs.Max.W <= maxW+96 {
			return content
		}
		row := widget.Row(
			widget.Expand(widget.Sized{}),
			widget.Sized{W: maxW, Child: content},
			widget.Expand(widget.Sized{}),
		)
		row.CrossAlign = layout.CrossStretch
		// Border reads as a subtle neutral frame around the reading column.
		return widget.Decorated{Color: th.Border, Child: row}
	}}
}

// feedPage lists top stories. It carries only data; the API comes from context.
type feedPage struct {
	N int
}

func (f feedPage) CreateState() widget.State { return &feedState{} }

type feedState struct {
	widget.StateBase[feedPage]
	stories    []Item
	loading    bool
	refreshing bool
	err        string
}

// stateHook lets tests observe the mounted feed state.
var stateHook func(*feedState)

func (s *feedState) Init(ctx widget.Ctx) {
	if stateHook != nil {
		stateHook(s)
	}
	s.loading = true
	s.fetch(ctx)
}

// fetch loads the top stories on a background goroutine and swaps them in.
// Used for the initial load and for pull-to-refresh.
func (s *feedState) fetch(ctx widget.Ctx) {
	post := ctx.Post()
	api := widget.MustOf[API](ctx)
	n := s.W().N
	go func() {
		ids, err := api.TopStories()
		if err != nil {
			post(func() { s.SetState(func() { s.loading, s.refreshing, s.err = false, false, err.Error() }) })
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
		post(func() { s.SetState(func() { s.loading, s.refreshing, s.stories = false, false, stories }) })
	}()
}

func (s *feedState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	var body widget.Widget
	switch {
	case s.loading:
		body = widget.Center(widget.Text{S: "loading…", Size: th.Type.Body, Color: th.Muted})
	case s.err != "":
		body = widget.Center(widget.Text{S: s.err, Wrap: true, Size: th.Type.Body, Color: th.Muted})
	default:
		nav := widget.MustOf[widget.Nav](ctx)
		body = widget.LazyList{
			Count:           len(s.stories),
			EstimatedExtent: 66,
			Build:           func(i int) widget.Widget { return s.storyRow(th, nav, i) },
			Refreshing:      s.refreshing,
			OnRefresh: func() {
				s.SetState(func() { s.refreshing = true })
				s.fetch(ctx)
			},
		}
	}
	return page(ctx, header(th, "Hacker News", nil), body)
}

func (s *feedState) storyRow(th theme.Theme, nav widget.Nav, i int) widget.Widget {
	st := s.stories[i]
	meta := fmt.Sprintf("%d points · %s · %d comments", st.Score, st.By, st.Descendants)
	if d := domain(st.URL); d != "" {
		meta = d + " · " + meta
	}
	title := widget.Column(
		widget.Text{S: st.Title, Font: "bold", Size: th.Type.Heading, Color: th.Text, Wrap: true},
		widget.Sized{H: 4},
		widget.Text{S: meta, Size: th.Type.Caption, Color: th.Muted},
	)
	title.CrossAlign = layout.CrossStart
	row := widget.Row(
		widget.Sized{W: 34, Child: widget.Text{S: fmt.Sprintf("%d.", i+1), Size: th.Type.Label, Color: th.Muted}},
		widget.Expand(title),
	)
	row.CrossAlign = layout.CrossStart
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() { nav.Push(threadPage{Story: st}) }},
		Child: widget.Decorated{Color: th.Surface,
			Child: widget.Padding{Insets: geom.InsetsSymmetric(12, 10), Child: row}},
	}
}

// threadPage shows one story's comments. It carries only the story (plain
// serializable data); the API comes from context. That makes it registerable
// so a hot-restart can rebuild it and land you back on the same thread.
type threadPage struct {
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
	api := widget.MustOf[API](ctx)
	story := s.W().Story
	go func() {
		comments := loadComments(api, story, 80)
		post(func() { s.SetState(func() { s.comments, s.loading = comments, false }) })
	}()
}

func (s *threadState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	st := s.W().Story
	var body widget.Widget
	if s.loading {
		body = widget.Center(widget.Text{S: "loading comments…", Size: th.Type.Body, Color: th.Muted})
	} else {
		n := len(s.comments)
		openURL := func(u string) { _ = ctx.OpenURL(u) }
		body = widget.LazyList{
			Count:           n + 1,
			EstimatedExtent: 90,
			Build: func(i int) widget.Widget {
				if i == 0 {
					return storyHeaderCell(th, st, openURL)
				}
				return commentRow(th, s.comments[i-1], openURL)
			},
		}
	}
	return page(ctx, header(th, st.Title, backButton(ctx)), body)
}

func storyHeaderCell(th theme.Theme, st Item, openURL func(string)) widget.Widget {
	meta := fmt.Sprintf("%d points by %s · %d comments", st.Score, st.By, st.Descendants)
	kids := []widget.Widget{
		widget.Text{S: st.Title, Font: "bold", Size: th.Type.Heading, Color: th.Text, Wrap: true},
		widget.Sized{H: 6},
		widget.Text{S: meta, Size: th.Type.Caption, Color: th.Muted},
	}
	if st.URL != "" {
		kids = append(kids, widget.Sized{H: 6}, widget.Rich{
			Spans:  []layout.RichSpan{{Text: domain(st.URL) + " ↗", Color: th.Primary, Link: st.URL}},
			Size:   th.Type.Label,
			OnLink: openURL,
		})
	}
	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStart
	return widget.Decorated{Color: th.Surface,
		Child: widget.Padding{All: 14, Child: col}}
}

func commentRow(th theme.Theme, c Comment, openURL func(string)) widget.Widget {
	style := spanStyle{Text: th.Text, Link: th.Primary, Emph: th.Muted}
	body := widget.Column(
		widget.Text{S: c.By, Size: th.Type.Label, Color: th.Primary},
		widget.Sized{H: 4},
		widget.Rich{Spans: parseSpans(c.Text, style), Size: th.Type.Body, OnLink: openURL},
	)
	body.CrossAlign = layout.CrossStart
	return widget.Padding{
		Insets: geom.Insets{Top: 6, Left: 12 + float32(c.Depth)*16, Right: 12},
		Child: widget.Decorated{Color: th.Surface,
			Child: widget.Padding{All: 10, Child: body}},
	}
}

// Root returns the HN app widget over the live API.
func Root() widget.Widget { return HN{API: newLiveAPI()} }

// Background returns the app background color.
func Background() paint.Color { return colBg }
