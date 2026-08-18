package ui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/examples/news/internal/rank"
	"github.com/doug/gophics/examples/news/internal/store"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// queueTab is the reading queue: every unread article from every source, in the
// order the ranking model thinks you want them.
//
// The design premise is that the scarce thing is attention, not articles. So a
// row carries exactly what is needed to decide whether to open it — headline,
// source, age, how long it will take, and a picture if there is one — and
// nothing else. Swiping a row away is as cheap as opening it, and both teach
// the model.
type queueTab struct{}

func (queueTab) CreateState() widget.State { return &queueState{} }

type queueState struct {
	widget.StateBase[queueTab]
	items    []rank.Scored
	cats     []library.CategoryCount
	category string
	loading  bool

	refreshing bool
	progress   string
	lastResult string
}

func (s *queueState) Init(ctx widget.Ctx) {
	s.loading = true
	s.reload(ctx)
	// A queue that is a week stale on opening is the most common way a reader
	// disappoints, so poll on launch unless asked not to.
	if lib := env(ctx).Lib; lib.Prefs != nil && lib.Prefs.RefreshOnLaunch() {
		if _, last, _ := lib.Refreshing(); time.Since(last) > 15*time.Minute {
			s.refresh(ctx)
		}
	}
}

// reload rebuilds the queue from what is already on disk. It is cheap — no
// network — so it runs after every action that could change the order.
func (s *queueState) reload(ctx widget.Ctx) {
	lib := env(ctx).Lib
	cat := s.category
	post := ctx.Post()
	go func() {
		items := lib.Queue(library.QueueOptions{Category: cat})
		cats := lib.Categories()
		post(func() { s.SetState(func() { s.items, s.cats, s.loading = items, cats, false }) })
	}()
}

// refresh polls the network, reporting progress into the header as it goes.
func (s *queueState) refresh(ctx widget.Ctx) {
	lib := env(ctx).Lib
	post := ctx.Post()
	s.refreshing, s.progress, s.lastResult = true, "checking feeds…", ""
	// Read the selected category here, on the UI goroutine. The background
	// work below must not touch widget state directly.
	cat := s.category

	go func() {
		res := lib.Refresh(context.Background(), func(p library.Progress) {
			post(func() {
				s.SetState(func() {
					if p.Phase == "images" {
						s.progress = fmt.Sprintf("saving pictures %d/%d", p.Done, p.Total)
					} else {
						s.progress = fmt.Sprintf("%d/%d feeds", p.Done, p.Total)
					}
				})
			})
		})
		items := lib.Queue(library.QueueOptions{Category: cat})
		cats := lib.Categories()
		post(func() {
			s.SetState(func() {
				s.items, s.cats, s.loading, s.refreshing = items, cats, false, false
				s.progress = ""
				s.lastResult = summarise(res)
			})
		})
	}()
}

// summarise is the one line shown after a refresh. It reports failures, because
// a feed that has been broken for a month should not be a silent absence.
func summarise(r library.RefreshResult) string {
	switch {
	case r.Skipped:
		return "" // a poll was already running; leave the header alone
	case r.Feeds == 0:
		return "No sources yet — add some under Sources."
	case r.NewItems == 0 && r.Failed == 0:
		return "Up to date."
	case r.Failed > 0:
		return fmt.Sprintf("%d new · %d source(s) failed", r.NewItems, r.Failed)
	default:
		return fmt.Sprintf("%d new article(s)", r.NewItems)
	}
}

func (s *queueState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	lib := env(ctx).Lib

	subtitle := s.progress
	if subtitle == "" {
		subtitle = s.lastResult
	}
	if subtitle == "" && len(s.items) > 0 {
		subtitle = fmt.Sprintf("%d unread", len(s.items))
	}

	head := header(th, "Read", subtitle, nil,
		headerAction(th, "Refresh", func() {
			if !s.refreshing {
				s.SetState(func() { s.refresh(ctx) })
			}
		}),
	)

	var body widget.Widget
	switch {
	case s.loading:
		body = centered(th, "Loading…", "")
	case len(s.items) == 0 && s.category != "":
		body = centered(th, "Nothing here", "No unread articles in "+s.category+".")
	case len(s.items) == 0:
		body = centered(th, "You are all caught up",
			"Pull down to check your sources for something new, or add more under Sources.")
	default:
		body = s.list(ctx, th, lib)
	}

	return tabScaffold(ctx, head, colStretch(
		s.filterRow(ctx, th, lib),
		widget.Expand(body),
	))
}

// filterRow is the category chips. It only appears once there is more than one
// category to choose between — a filter with one option is furniture.
func (s *queueState) filterRow(ctx widget.Ctx, th theme.Theme, lib *library.Library) widget.Widget {
	// The counts come from the cached snapshot taken alongside the queue.
	// Recomputing them here would query the article store on every frame, which
	// is disk I/O inside the frame loop.
	cats := s.cats
	if len(cats) < 2 {
		return widget.Sized{}
	}
	kids := []widget.Widget{
		chip(th, "All", s.category == "", func() {
			s.SetState(func() { s.category = "" })
			s.reload(ctx)
		}),
	}
	for _, c := range cats {
		label := c.Name
		if c.Unread > 0 {
			label = fmt.Sprintf("%s %d", c.Name, c.Unread)
		}
		kids = append(kids, chip(th, label, s.category == c.Name, func() {
			s.SetState(func() { s.category = c.Name })
			s.reload(ctx)
		}))
	}
	return colStretch(
		widget.Padding{Insets: geom.InsetsSymmetric(12, 8),
			Child: chipBar(kids...)},
		divider(th),
	)
}

func (s *queueState) list(ctx widget.Ctx, th theme.Theme, lib *library.Library) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	return widget.LazyList{
		Count:           len(s.items),
		EstimatedExtent: 116,
		Refreshing:      s.refreshing,
		OnRefresh: func() {
			s.SetState(func() { s.refresh(ctx) })
		},
		Build: func(i int) widget.Widget {
			if i >= len(s.items) {
				return widget.Sized{}
			}
			sc := s.items[i]
			// Building a row is the moment the article was actually on screen.
			// Enough of these without an open is what the model reads as a skip.
			lib.Impression(sc.Item)
			return widget.WithKey{Key: sc.Item.ID, Child: widget.Dismissible{
				Direction: widget.DismissLeft,
				Background: widget.Decorated{Color: th.Muted, Child: widget.Padding{
					Insets: geom.InsetsSymmetric(20, 0),
					Child: widget.Row(widget.Expand(widget.Sized{}),
						widget.Text{S: "Not interested", Size: th.Type.Label, Color: th.Bg}),
				}},
				OnDismissed: func() {
					lib.Dismiss(sc.Item)
					// Remove by identity, not by the index this row was built
					// with: a refresh landing mid-swipe reorders the queue, and
					// the index would then delete somebody else.
					s.SetState(func() { s.items = removeItem(s.items, sc.Item.ID) })
				},
				Child: s.row(ctx, th, nav, sc),
			}}
		},
	}
}

func (s *queueState) row(ctx widget.Ctx, th theme.Theme, nav widget.Nav, sc rank.Scored) widget.Widget {
	it := sc.Item

	title := widget.Text{S: it.Title, Font: "bold", Size: th.Type.Heading, Color: th.Text,
		Wrap: true, MaxLines: 3, Ellipsis: true}

	metaText := fmt.Sprintf("%s · %s", it.FeedName, ago(it.Published))
	if m := readingMinutes(it); m > 0 {
		metaText += fmt.Sprintf(" · %d min", m)
	}
	if it.Source == store.SourceSummary {
		// Be honest that this one is a headline, not an article: it changes
		// whether it is worth opening away from wifi.
		metaText += " · summary only"
	}

	left := []widget.Widget{title}
	if sum := strings.TrimSpace(it.Summary); sum != "" {
		left = append(left, widget.Sized{H: 4},
			widget.Text{S: sum, Size: th.Type.Label, Color: th.Muted,
				Wrap: true, MaxLines: 2, Ellipsis: true})
	}
	left = append(left, widget.Sized{H: 5},
		widget.Text{S: metaText, Size: th.Type.Caption, Color: th.Muted,
			MaxLines: 1, Ellipsis: true})
	col := widget.Column(left...)
	col.CrossAlign = layout.CrossStart

	kids := []widget.Widget{
		widget.Sized{W: 4, Child: scoreBar(th, sc.Score)},
		widget.Sized{W: 10},
		widget.Expand(col),
	}
	if it.LeadImage != "" {
		kids = append(kids, widget.Sized{W: 12}, Img{URL: it.LeadImage, W: 92, H: 68})
	}
	row := widget.Row(kids...)
	row.CrossAlign = layout.CrossStart

	return colStretch(
		theme.Tappable{
			// The reader records the open itself, on mount. Doing it here as
			// well counted every open twice.
			OnTap: func() { nav.Push(readerPage{ItemID: it.ID}) },
			// Holding a row asks why it is where it is. Ranking that cannot be
			// interrogated is ranking you end up fighting.
			OnLongPress: func() { nav.Push(whyPage{ItemID: it.ID}) },
			Background:  th.Bg,
			Pad:         geom.InsetsSymmetric(12, 12),
			Child:       row,
		},
		divider(th),
	)
}

// scoreBar is the likelihood, drawn as a short vertical bar down the left of
// the row. A number would invite arguing with a probability; a bar reads as
// "the app is fairly sure about this one" at a glance and takes no space.
func scoreBar(th theme.Theme, score float64) widget.Widget {
	// Map 0.35..0.85 onto the visible range: real scores cluster in the middle,
	// and a bar that never moves tells you nothing.
	f := (score - 0.35) / 0.5
	f = max(0, min(1, f))

	col := th.Muted
	switch {
	case f > 0.66:
		col = th.Primary
	case f > 0.33:
		col = th.Muted
	default:
		col = th.Border
	}
	return widget.Column(
		widget.Expand(widget.Sized{}),
		widget.Decorated{Color: col, Radius: 2,
			Child: widget.Sized{W: 4, H: float32(12 + 44*f)}},
		widget.Expand(widget.Sized{}),
	)
}

// readingMinutes at 220 words a minute.
func readingMinutes(it *store.Item) int {
	if it.WordCount <= 0 {
		return 0
	}
	return max(1, (it.WordCount+219)/220)
}

// ago is a compact relative time. Precision beyond the hour is noise in a queue.
func ago(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

func removeItem(items []rank.Scored, id string) []rank.Scored {
	out := make([]rank.Scored, 0, len(items))
	for _, sc := range items {
		if sc.Item.ID != id {
			out = append(out, sc)
		}
	}
	return out
}
