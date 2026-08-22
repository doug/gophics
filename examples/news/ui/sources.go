package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// sourcesTab lists what the reader is subscribed to, and is the way in to
// finding more.
//
// Two routes to a new source, because there are two ways people actually get
// them: from a list someone curated, and from a site they already read. The
// first is the built-in catalog; the second is pasting an address and letting
// the app work out the feed.
type sourcesTab struct{}

func (sourcesTab) CreateState() widget.State { return &sourcesState{} }

type sourcesState struct {
	widget.StateBase[sourcesTab]
	// status is each feed's last-poll outcome, read from the store once when
	// the screen appears. Reading it inside the row builder meant a file read
	// per row per frame.
	status map[string]feedStatus
}

// feedStatus is what the sources list says about a feed underneath its name.
type feedStatus struct {
	text string
	bad  bool // true for an error, which is coloured differently
}

func (s *sourcesState) Init(ctx widget.Ctx) { s.refreshStatus(ctx) }

func (s *sourcesState) refreshStatus(ctx widget.Ctx) {
	lib := env(ctx).Lib
	if lib.Store == nil {
		return
	}
	out := map[string]feedStatus{}
	for _, f := range lib.Subs.All() {
		st, err := lib.Store.LoadState(f.ID)
		if err != nil {
			continue
		}
		switch {
		case st.ConsecutiveErr > 2:
			out[f.ID] = feedStatus{"not responding", true}
		case st.LastError != "":
			out[f.ID] = feedStatus{"last check failed", true}
		}
	}
	s.status = out
}

func (s *sourcesState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	lib := env(ctx).Lib
	nav := widget.MustOf[widget.Nav](ctx)

	feeds := lib.Subs.All()
	sort.SliceStable(feeds, func(i, j int) bool {
		if feeds[i].Category != feeds[j].Category {
			return feeds[i].Category < feeds[j].Category
		}
		return strings.ToLower(feeds[i].Title) < strings.ToLower(feeds[j].Title)
	})

	head := header(th, "Sources", fmt.Sprintf("%d subscribed", len(feeds)), nil,
		headerAction(th, "Add", func() { nav.Push(addFeedPage{}) }))

	actions := widget.Padding{Insets: geom.InsetsSymmetric(14, 12),
		Child: widget.Row(
			widget.Expand(button(th, "Browse suggestions", func() { nav.Push(browsePage{}) })),
			widget.Sized{W: 10},
			widget.Expand(secondaryButton(th, "Add by address", func() { nav.Push(addFeedPage{}) })),
		),
	}

	var body widget.Widget
	if len(feeds) == 0 {
		body = centered(th, "No sources yet",
			"Browse the built-in suggestions, or paste the address of a site you already read.")
	} else {
		lastCat := ""
		body = widget.LazyList{
			Count:           len(feeds),
			EstimatedExtent: 64,
			Build: func(i int) widget.Widget {
				f := feeds[i]
				row := s.feedRow(ctx, th, nav, lib, f)
				// A category heading whenever the category changes. Cheap here
				// because the list is sorted by it.
				if i == 0 || feeds[i-1].Category != f.Category {
					lastCat = f.Category
					return colStretch(sectionHeading(th, lastCat), row)
				}
				return row
			},
		}
	}

	return tabScaffold(ctx, head, colStretch(actions, divider(th), widget.Expand(body)))
}

func (s *sourcesState) feedRow(ctx widget.Ctx, th theme.Theme, nav widget.Nav,
	lib *library.Library, f catalog.Feed) widget.Widget {

	status, statusColor := "", th.Muted
	if !f.IsEnabled() {
		status = "paused"
	} else if st, ok := s.status[f.ID]; ok {
		status = st.text
		if st.bad {
			statusColor = th.Danger
		}
	}

	sub := f.URL
	if status != "" {
		sub = status
	}
	col := widget.Column(
		widget.Text{S: displayTitle(f), Size: th.Type.Body, Color: th.Text, MaxLines: 1, Ellipsis: true},
		widget.Sized{H: 3},
		widget.Text{S: sub, Size: th.Type.Caption, Color: statusColor, MaxLines: 1, Ellipsis: true},
	)
	col.CrossAlign = layout.CrossStart

	row := widget.Row(
		widget.Expand(col),
		widget.Sized{W: 10},
		toggle(th, f.IsEnabled(), func() {
			lib.Subs.SetEnabled(f.ID, !f.IsEnabled())
			s.SetState(func() {})
		}),
	)
	row.CrossAlign = layout.CrossCenter

	return colStretch(
		theme.Tappable{
			OnTap:      func() { nav.Push(feedPage{FeedID: f.ID, URL: f.URL, Title: f.Title}) },
			Background: th.Bg,
			Pad:        geom.InsetsSymmetric(14, 11),
			Child:      row,
		},
		divider(th),
	)
}

func displayTitle(f catalog.Feed) string {
	if f.Title != "" {
		return f.Title
	}
	return library.FeedDomain(f.URL)
}

func sectionHeading(th theme.Theme, s string) widget.Widget {
	return widget.Decorated{Color: th.Surface, Child: widget.Padding{
		Insets: geom.InsetsSymmetric(14, 6),
		Child: widget.Text{S: strings.ToUpper(s), Font: "bold",
			Size: th.Type.Caption, Color: th.Muted},
	}}
}

// toggle is a compact on/off control. The framework has no switch widget, so
// this is a tappable pill that reads unambiguously in both states.
func toggle(th theme.Theme, on bool, onTap func()) widget.Widget {
	label, fg, bg := "Off", th.Muted, th.Surface
	if on {
		label, fg, bg = "On", th.OnPrimary, th.Primary
	}
	return theme.Tappable{
		OnTap:      onTap,
		Background: bg,
		Radius:     12,
		Pad:        geom.InsetsSymmetric(14, 6),
		Child:      widget.Text{S: label, Size: th.Type.Caption, Color: fg},
	}
}

// browsePage is the built-in catalog: verified sources grouped by category,
// with what each one is for. This is the "I don't know what to subscribe to"
// answer, and it is why the app ships a catalog at all.
type browsePage struct{}

func (browsePage) CreateState() widget.State { return &browseState{} }

type browseState struct {
	widget.StateBase[browsePage]
	category string
}

func (s *browseState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	lib := env(ctx).Lib
	nav := widget.MustOf[widget.Nav](ctx)

	cat, err := library.Suggestions()
	if err != nil {
		return page(ctx, header(th, "Suggestions", "", backButton(ctx)),
			centered(th, "Catalog unavailable", err.Error()))
	}

	// Copy before sorting. cat.Feeds is the backing array of the catalog cached
	// process-wide by library.Suggestions, so sorting it in place would reorder
	// shared state permanently — and race with any background goroutine reading
	// it, which Library.Meta does on every refresh.
	var feeds []catalog.Feed
	for _, x := range cat.Feeds {
		if s.category == "" || x.Category == s.category {
			feeds = append(feeds, x)
		}
	}
	sort.SliceStable(feeds, func(i, j int) bool {
		if feeds[i].Priority != feeds[j].Priority {
			return feeds[i].Priority > feeds[j].Priority // must-read first
		}
		return strings.ToLower(feeds[i].Title) < strings.ToLower(feeds[j].Title)
	})

	chips := []widget.Widget{
		chip(th, "All", s.category == "", func() { s.SetState(func() { s.category = "" }) }),
	}
	for _, c := range cat.Categories() {
		chips = append(chips, chip(th, c, s.category == c, func() {
			s.SetState(func() { s.category = c })
		}))
	}
	filter := colStretch(
		widget.Padding{Insets: geom.InsetsSymmetric(12, 8),
			Child: chipBar(chips...)},
		divider(th),
	)

	list := widget.LazyList{
		Count:           len(feeds),
		EstimatedExtent: 86,
		Build: func(i int) widget.Widget {
			return s.suggestionRow(ctx, th, nav, lib, feeds[i])
		},
	}

	return page(ctx, header(th, "Suggestions",
		fmt.Sprintf("%d sources", len(feeds)), backButton(ctx)),
		colStretch(filter, widget.Expand(list)))
}

func (s *browseState) suggestionRow(ctx widget.Ctx, th theme.Theme, nav widget.Nav,
	lib *library.Library, f catalog.Feed) widget.Widget {

	subscribed := lib.Subs.Has(f.ID)

	// The catalog's own note is the most useful thing on the row: it says what
	// the source is for, which a title rarely does.
	note := f.Notes
	if note == "" {
		note = describeFeed(f)
	}

	col := widget.Column(
		widget.Text{S: f.Title, Font: "bold", Size: th.Type.Body, Color: th.Text,
			MaxLines: 1, Ellipsis: true},
		widget.Sized{H: 3},
		widget.Text{S: note, Size: th.Type.Caption, Color: th.Muted, Wrap: true,
			MaxLines: 2, Ellipsis: true},
		widget.Sized{H: 5},
		widget.Text{S: describeFeed(f), Size: th.Type.Caption, Color: th.Muted},
	)
	col.CrossAlign = layout.CrossStart

	action := button(th, "Add", func() {
		lib.Subs.Add(f)
		s.SetState(func() {})
	})
	if subscribed {
		action = secondaryButton(th, "Added", func() {
			lib.Subs.Remove(f.ID)
			s.SetState(func() {})
		})
	}

	row := widget.Row(widget.Expand(col), widget.Sized{W: 12}, action)
	row.CrossAlign = layout.CrossCenter

	return colStretch(
		theme.Tappable{
			OnTap:      func() { nav.Push(feedPage{FeedID: f.ID, URL: f.URL, Title: f.Title}) },
			Background: th.Bg,
			Pad:        geom.InsetsSymmetric(14, 12),
			Child:      row,
		},
		divider(th),
	)
}

// describeFeed says what a source delivers, in the catalog's own terms.
func describeFeed(f catalog.Feed) string {
	var parts []string
	if f.Priority == catalog.MustRead {
		parts = append(parts, "must-read")
	}
	if f.Kind != "" {
		parts = append(parts, string(f.Kind))
	}
	switch f.Fulltext {
	case catalog.FullText:
		parts = append(parts, "full text")
	case catalog.Partial:
		parts = append(parts, "partial text")
	case catalog.Teaser:
		parts = append(parts, "teaser — fetched from the site")
	}
	if len(parts) == 0 {
		return f.Category
	}
	return f.Category + " · " + strings.Join(parts, " · ")
}

// addFeedPage takes a website address and finds its feeds.
type addFeedPage struct{}

func (addFeedPage) CreateState() widget.State { return &addFeedState{} }

type addFeedState struct {
	widget.StateBase[addFeedPage]
	input      string
	searching  bool
	err        string
	candidates []library.Candidate
	category   string
}

func (s *addFeedState) Init(widget.Ctx) { s.category = "unsorted" }

func (s *addFeedState) search(ctx widget.Ctx) {
	if strings.TrimSpace(s.input) == "" {
		return
	}
	s.searching, s.err, s.candidates = true, "", nil
	post := ctx.Post()
	in := s.input
	lctx := ctx.Context()
	go func() {
		cands, err := library.Discover(lctx, in)
		post(func() {
			s.SetState(func() {
				s.searching = false
				if err != nil {
					s.err = err.Error()
					return
				}
				s.candidates = cands
			})
		})
	}()
}

func (s *addFeedState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	lib := env(ctx).Lib

	field := widget.Decorated{Color: th.Surface, Radius: th.Radius,
		Child: widget.Padding{All: 12, Child: widget.TextField{
			Value:            s.input,
			Placeholder:      "example.com, or a feed address",
			Size:             th.Type.Body,
			TextColor:        th.Text,
			PlaceholderColor: th.Muted,
			CaretColor:       th.Primary,
			SelectionColor:   th.Selection,
			OnChange:         func(v string) { s.SetState(func() { s.input = v }) },
			OnSubmit:         func(string) { s.SetState(func() { s.search(ctx) }) },
		}},
	}

	head := colStretch(
		widget.Padding{Insets: geom.InsetsSymmetric(16, 14), Child: colStretch(
			widget.Text{S: "Paste the address of a site you read. The app will find its feed.",
				Size: th.Type.Label, Color: th.Muted, Wrap: true},
			widget.Sized{H: 10},
			field,
			widget.Sized{H: 10},
			button(th, "Find feeds", func() { s.SetState(func() { s.search(ctx) }) }),
		)},
		divider(th),
	)

	var body widget.Widget
	switch {
	case s.searching:
		body = centered(th, "Looking…", "Fetching the page and reading what it publishes.")
	case s.err != "":
		body = centered(th, "No feed found", s.err)
	case len(s.candidates) > 0:
		body = widget.Scroll{Child: s.results(ctx, th, lib)}
	default:
		body = widget.Sized{}
	}

	return page(ctx, header(th, "Add a source", "", backButton(ctx)),
		widget.Column(head, widget.Expand(body)))
}

func (s *addFeedState) results(ctx widget.Ctx, th theme.Theme, lib *library.Library) widget.Widget {
	kids := []widget.Widget{s.categoryPicker(th, lib)}
	for _, c := range s.candidates {
		kids = append(kids, s.candidateCard(th, lib, c))
	}
	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStretch
	return col
}

// categoryPicker chooses where a new source files. Categories come from what is
// already subscribed, so the list stays the user's own vocabulary.
func (s *addFeedState) categoryPicker(th theme.Theme, lib *library.Library) widget.Widget {
	cats := lib.Subs.Categories()
	if len(cats) == 0 {
		cats = []string{"unsorted"}
	}
	chips := make([]widget.Widget, 0, len(cats))
	for _, c := range cats {
		chips = append(chips, chip(th, c, s.category == c, func() {
			s.SetState(func() { s.category = c })
		}))
	}
	return widget.Padding{Insets: geom.InsetsSymmetric(14, 10), Child: widget.Column(
		widget.Text{S: "File under", Size: th.Type.Caption, Color: th.Muted},
		widget.Sized{H: 8},
		chipBar(chips...),
	)}
}

// candidateCard previews one discovered feed: what it is, what it ships, and
// the last few things it published. Subscribing to something sight unseen is
// how a queue fills with noise.
func (s *addFeedState) candidateCard(th theme.Theme, lib *library.Library, c library.Candidate) widget.Widget {
	title := c.Title
	if title == "" {
		title = library.FeedDomain(c.URL)
	}

	kids := []widget.Widget{
		widget.Text{S: title, Font: "bold", Size: th.Type.Heading, Color: th.Text, Wrap: true},
		widget.Sized{H: 4},
		widget.Text{S: c.URL, Size: th.Type.Caption, Color: th.Muted, MaxLines: 1, Ellipsis: true},
	}
	if c.Err != "" {
		kids = append(kids, widget.Sized{H: 6},
			widget.Text{S: c.Err, Size: th.Type.Caption, Color: th.Danger})
	} else {
		kids = append(kids, widget.Sized{H: 8},
			widget.Text{S: fulltextNote(c.Fulltext), Size: th.Type.Caption, Color: th.Muted, Wrap: true})
		for _, it := range c.Items {
			kids = append(kids, widget.Sized{H: 8}, widget.Column(
				widget.Text{S: "· " + it.Title, Size: th.Type.Label, Color: th.Text,
					Wrap: true, MaxLines: 2, Ellipsis: true},
			))
		}
		kids = append(kids, widget.Sized{H: 14})
		if lib.Subs.Has(c.FeedFor(s.category).ID) {
			kids = append(kids, secondaryButton(th, "Already subscribed", func() {}))
		} else {
			kids = append(kids, button(th, "Subscribe", func() {
				lib.Subs.Add(c.FeedFor(s.category))
				s.SetState(func() {})
			}))
		}
	}

	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStart
	return widget.Padding{Insets: geom.InsetsSymmetric(14, 8),
		Child: widget.Decorated{Color: th.Surface, Radius: th.Radius,
			Child: widget.Padding{All: 14, Child: col}}}
}

func fulltextNote(f catalog.Fulltext) string {
	switch f {
	case catalog.FullText:
		return "Ships full article text — nothing extra to download."
	case catalog.Partial:
		return "Ships part of each article; the rest is fetched from the site."
	case catalog.Teaser:
		return "Ships headlines only; articles are fetched from the site."
	default:
		return ""
	}
}
