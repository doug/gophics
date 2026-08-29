package ui

import (
	"fmt"
	"strings"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// feedPage is one source: what it publishes, and every setting that governs how
// the reader treats it.
//
// The same page serves a subscribed feed and a suggestion being considered,
// because they are the same question asked at different times — "what is this,
// and do I want it?" — and splitting them would mean two screens that drift.
type feedPage struct {
	FeedID string
	URL    string
	Title  string
}

func (feedPage) CreateState() widget.State { return &feedState{} }

type feedState struct {
	widget.StateBase[feedPage]
	preview library.Candidate
	loading bool
	err     string
}

func (s *feedState) Init(ctx widget.Ctx) {
	s.loading = true
	post := ctx.Post()
	url := s.W().URL
	lctx := ctx.Context()
	go func() {
		c, err := library.Preview(lctx, url)
		post(func() {
			s.SetState(func() {
				s.loading = false
				if err != nil {
					s.err = err.Error()
					return
				}
				s.preview = c
			})
		})
	}()
}

func (s *feedState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	lib := env(ctx).Lib
	nav := ctx.MustOf[widget.Nav]()

	f, subscribed := lib.Subs.ByID(s.W().FeedID)
	if !subscribed {
		// Not subscribed: fall back to the catalog entry, or synthesise one
		// from what discovery found.
		if cat, err := library.Suggestions(); err == nil {
			if cf, ok := cat.ByID(s.W().FeedID); ok {
				f = cf
			}
		}
		if f.ID == "" {
			f = s.preview.FeedFor("unsorted")
		}
	}

	title := f.Title
	if title == "" {
		title = s.W().Title
	}

	kids := []widget.Widget{
		s.summaryCard(th, f, subscribed),
		s.actions(ctx, th, lib, nav, f, subscribed),
	}
	if subscribed {
		kids = append(kids, s.settingsCard(ctx, th, lib, f))
	}
	kids = append(kids, s.recentCard(th))

	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStretch

	return page(ctx, header(th, title, library.FeedDomain(f.URL), backButton(ctx)),
		widget.Scroll{Child: col})
}

func (s *feedState) summaryCard(th theme.Theme, f catalog.Feed, subscribed bool) widget.Widget {
	lines := []widget.Widget{}
	if f.Notes != "" {
		lines = append(lines,
			widget.Text{S: f.Notes, Size: th.Type.Body, Color: th.Text, Wrap: true},
			widget.Sized{H: 10})
	}
	lines = append(lines,
		widget.Text{S: describeFeed(f), Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 6},
		widget.Text{S: f.URL, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
	)
	if !subscribed {
		lines = append(lines, widget.Sized{H: 6},
			widget.Text{S: "Not subscribed", Size: th.Type.Caption, Color: th.Muted})
	}
	col := widget.Column(lines...)
	col.CrossAlign = layout.CrossStart
	return card(th, col)
}

func (s *feedState) actions(ctx widget.Ctx, th theme.Theme, lib *library.Library,
	nav widget.Nav, f catalog.Feed, subscribed bool) widget.Widget {

	if !subscribed {
		return widget.Padding{Insets: geom.InsetsSymmetric(14, 4),
			Child: button(th, "Subscribe", func() {
				lib.Subs.Add(f)
				s.SetState(func() {})
			})}
	}
	return widget.Padding{Insets: geom.InsetsSymmetric(14, 4), Child: widget.Row(
		widget.Expand(secondaryButton(th, enabledLabel(f), func() {
			lib.Subs.SetEnabled(f.ID, !f.IsEnabled())
			s.SetState(func() {})
		})),
		widget.Sized{W: 10},
		widget.Expand(secondaryButton(th, "Unsubscribe", func() {
			lib.Subs.Remove(f.ID)
			nav.Pop()
		})),
	)}
}

func enabledLabel(f catalog.Feed) string {
	if f.IsEnabled() {
		return "Pause polling"
	}
	return "Resume polling"
}

// settingsCard holds the per-feed settings worth exposing: where it files, how
// much it is trusted, and — for a gated publisher — the subscription.
func (s *feedState) settingsCard(ctx widget.Ctx, th theme.Theme, lib *library.Library,
	f catalog.Feed) widget.Widget {

	nav := ctx.MustOf[widget.Nav]()

	// Category
	cats := lib.Subs.Categories()
	catChips := make([]widget.Widget, 0, len(cats))
	for _, c := range cats {
		catChips = append(catChips, chip(th, c, f.Category == c, func() {
			f.Category = c
			lib.Subs.Update(f)
			s.SetState(func() {})
		}))
	}

	// Priority: the editorial weight the ranking model starts from.
	prios := []struct {
		label string
		v     catalog.Priority
	}{
		{"Filler", catalog.Filler},
		{"Normal", catalog.Normal},
		{"Must-read", catalog.MustRead},
	}
	prioChips := make([]widget.Widget, 0, len(prios))
	for _, p := range prios {
		cur := f.Priority
		if cur == 0 {
			cur = catalog.Normal
		}
		prioChips = append(prioChips, chip(th, p.label, cur == p.v, func() {
			f.Priority = p.v
			lib.Subs.Update(f)
			s.SetState(func() {})
		}))
	}

	kids := []widget.Widget{
		widget.Text{S: "FILE UNDER", Font: "bold", Size: th.Type.Caption, Color: th.Muted},
		widget.Sized{H: 8},
		chipBar(catChips...),
		widget.Sized{H: 16},
		widget.Text{S: "HOW MUCH TO TRUST IT", Font: "bold", Size: th.Type.Caption, Color: th.Muted},
		widget.Sized{H: 6},
		widget.Text{S: "The starting point for ranking, before the app learns from what you read.",
			Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 8},
		chipBar(prioChips...),
	}

	// Subscription, for the feeds that need one.
	if f.Fulltext == catalog.Teaser && f.ShouldExtract() {
		st := library.Cookies(f.URL)
		note := "This publisher gates its articles. Sign in to read them here."
		if st.Healthy() {
			note = fmt.Sprintf("Signed in — %d cookies saved for %s.", st.Count, st.Domain)
		} else if st.Present {
			note = "The saved session has expired. Sign in again."
		}
		kids = append(kids,
			widget.Sized{H: 18},
			widget.Text{S: "SUBSCRIPTION", Font: "bold", Size: th.Type.Caption, Color: th.Muted},
			widget.Sized{H: 6},
			widget.Text{S: note, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
			widget.Sized{H: 10},
			secondaryButton(th, signInLabel(st), func() {
				nav.Push(signInPage{FeedID: f.ID, URL: f.URL, Title: f.Title})
			}),
		)
	}

	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStart
	return card(th, col)
}

func signInLabel(st library.CookieStatus) string {
	if st.Present {
		return "Manage subscription"
	}
	return "Sign in to this publisher"
}

// recentCard is what the source published lately — the only reliable way to
// judge whether it is worth the space in a queue.
func (s *feedState) recentCard(th theme.Theme) widget.Widget {
	switch {
	case s.loading:
		return card(th, widget.Text{S: "Loading recent articles…", Size: th.Type.Body, Color: th.Muted})
	case s.err != "":
		return card(th, widget.Column(
			widget.Text{S: "Could not read this feed", Font: "bold", Size: th.Type.Body, Color: th.Text},
			widget.Sized{H: 6},
			widget.Text{S: s.err, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		))
	case len(s.preview.Items) == 0:
		return card(th, widget.Text{S: "This feed is currently empty.", Size: th.Type.Body, Color: th.Muted})
	}

	kids := []widget.Widget{
		widget.Text{S: "RECENTLY", Font: "bold", Size: th.Type.Caption, Color: th.Muted},
	}
	for _, it := range s.preview.Items {
		meta := it.Published
		if it.Words > 0 {
			meta = strings.TrimSpace(meta + fmt.Sprintf(" · %d words", it.Words))
		}
		kids = append(kids, widget.Sized{H: 12}, widget.Column(
			widget.Text{S: it.Title, Size: th.Type.Body, Color: th.Text, Wrap: true,
				MaxLines: 2, Ellipsis: true},
			widget.Sized{H: 3},
			widget.Text{S: meta, Size: th.Type.Caption, Color: th.Muted},
		))
	}
	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStart
	return card(th, col)
}

// card is the standard inset panel used across the settings-ish screens.
func card(th theme.Theme, child widget.Widget) widget.Widget {
	return widget.Padding{Insets: geom.InsetsSymmetric(14, 8),
		Child: widget.Decorated{Color: th.Surface, Radius: th.Radius,
			Child: widget.Padding{All: 14, Child: child}}}
}
