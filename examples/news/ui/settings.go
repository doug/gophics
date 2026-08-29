package ui

import (
	"fmt"

	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// settingsTab holds the few genuine preferences, the subscriptions to gated
// publishers, and the two destructive buttons that need to exist somewhere:
// clearing the picture cache and forgetting what the ranking model learned.
type settingsTab struct{}

func (settingsTab) CreateState() widget.State { return &settingsState{} }

type settingsState struct {
	widget.StateBase[settingsTab]
	confirmReset bool
	cacheNote    string

	// Snapshots taken when the screen appears. Both walk the filesystem — the
	// picture cache directory, and one stat per gated publisher — which must
	// not happen inside the frame loop.
	cacheBytes int64
	cookies    map[string]library.CookieStatus
}

func (s *settingsState) Init(ctx widget.Ctx) { s.snapshot(ctx) }

func (s *settingsState) snapshot(ctx widget.Ctx) {
	lib := env(ctx).Lib
	s.cacheBytes = lib.Images.Size()
	s.cookies = map[string]library.CookieStatus{}
	for _, f := range lib.PaywalledFeeds() {
		s.cookies[f.ID] = library.Cookies(f.URL)
	}
}

func (s *settingsState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	lib := env(ctx).Lib
	nav := ctx.MustOf[widget.Nav]()
	prefs := lib.Prefs

	kids := []widget.Widget{
		s.readingCard(th, prefs),
		s.paywallCard(th, lib, nav),
		s.offlineCard(th, lib, prefs),
		s.rankingCard(th, lib),
		s.aboutCard(th, lib),
	}
	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStretch

	return tabScaffold(ctx, header(th, "Settings", "", nil), widget.Scroll{Child: col})
}

func (s *settingsState) readingCard(th theme.Theme, prefs *library.Settings) widget.Widget {
	scale := prefs.Scale()
	sample := widget.Text{
		S:     "The quick brown fox jumps over the lazy dog.",
		Size:  th.Type.Body * scale,
		Color: th.Text, Wrap: true,
	}
	set := func(v float64) {
		prefs.SetScale(v)
		s.SetState(func() {})
	}
	return card(th, colStart(
		widget.Text{S: "Reading size", Font: "bold", Size: th.Type.Body, Color: th.Text},
		widget.Sized{H: 10},
		sample,
		widget.Sized{H: 12},
		widget.Row(
			widget.Expand(secondaryButton(th, "Smaller", func() {
				set(max(0.8, float64(scale)-0.1))
			})),
			widget.Sized{W: 10},
			widget.Expand(secondaryButton(th, "Larger", func() {
				set(min(2.0, float64(scale)+0.1))
			})),
		),
		widget.Sized{H: 8},
		widget.Text{S: fmt.Sprintf("Currently %d%%. The same control sits in the reader's title bar.",
			int(scale*100)), Size: th.Type.Caption, Color: th.Muted, Wrap: true},
	))
}

// paywallCard lists the subscribed feeds that are gated, with whether each has
// a working session. Anything paid for should be visible in one place.
func (s *settingsState) paywallCard(th theme.Theme, lib *library.Library, nav widget.Nav) widget.Widget {
	feeds := lib.PaywalledFeeds()
	if len(feeds) == 0 {
		return widget.Sized{}
	}
	kids := []widget.Widget{
		widget.Text{S: "Subscriptions", Font: "bold", Size: th.Type.Body, Color: th.Text},
		widget.Sized{H: 6},
		widget.Text{S: "Sources that publish a teaser and gate the article. Sign in and the reader fetches the full text.",
			Size: th.Type.Caption, Color: th.Muted, Wrap: true},
	}
	for _, f := range feeds {
		st := s.cookies[f.ID]
		status, color := "not signed in", th.Muted
		if st.Healthy() {
			status, color = "signed in", th.Success
		} else if st.Present {
			status, color = "session expired", th.Warning
		}
		kids = append(kids, widget.Sized{H: 12}, theme.Tappable{
			OnTap:  func() { nav.Push(signInPage{FeedID: f.ID, URL: f.URL, Title: f.Title}) },
			Radius: th.Radius,
			Child: widget.Row(
				widget.Expand(colStart(
					widget.Text{S: displayTitle(f), Size: th.Type.Body, Color: th.Text,
						MaxLines: 1, Ellipsis: true},
					widget.Sized{H: 3},
					widget.Text{S: status, Size: th.Type.Caption, Color: color},
				)),
				widget.Text{S: "›", Size: th.Type.Heading, Color: th.Muted},
			),
		})
	}
	return card(th, colStart(kids...))
}

func (s *settingsState) offlineCard(th theme.Theme, lib *library.Library, prefs *library.Settings) widget.Widget {
	note := s.cacheNote
	if note == "" {
		note = fmt.Sprintf("%s of pictures saved for offline reading.", humanBytes(s.cacheBytes))
	}
	return card(th, colStart(
		widget.Text{S: "Offline", Font: "bold", Size: th.Type.Body, Color: th.Text},
		widget.Sized{H: 10},
		settingRow(th, "Download pictures on refresh",
			"Articles read the same with no signal. Costs data on each refresh.",
			prefs.Prefetch(), func() {
				prefs.SetPrefetch(!prefs.Prefetch())
				s.SetState(func() {})
			}),
		widget.Sized{H: 14},
		settingRow(th, "Check for news when opened",
			"Polls your sources when the app comes to the front, if it has been a while.",
			prefs.RefreshOnLaunch(), func() {
				prefs.SetRefreshOnLaunch(!prefs.RefreshOnLaunch())
				s.SetState(func() {})
			}),
		widget.Sized{H: 16},
		widget.Text{S: note, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 10},
		secondaryButton(th, "Clear picture cache", func() {
			lib.Images.Clear()
			s.SetState(func() {
				s.cacheBytes = 0
				s.cacheNote = "Cleared. Pictures download again on the next refresh."
			})
		}),
	))
}

func (s *settingsState) rankingCard(th theme.Theme, lib *library.Library) widget.Widget {
	trained := lib.Rank.Trained()
	kids := []widget.Widget{
		widget.Text{S: "Ranking", Font: "bold", Size: th.Type.Body, Color: th.Text},
		widget.Sized{H: 6},
		widget.Text{S: confidenceNote(trained), Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 6},
		widget.Text{S: "Everything it knows stays on this device. Hold any article in the queue to see why it is ranked where it is.",
			Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 12},
	}
	if s.confirmReset {
		kids = append(kids,
			widget.Text{S: "This forgets every source, topic and author preference it has learned. It cannot be undone.",
				Size: th.Type.Caption, Color: th.Danger, Wrap: true},
			widget.Sized{H: 10},
			widget.Row(
				widget.Expand(secondaryButton(th, "Cancel", func() {
					s.SetState(func() { s.confirmReset = false })
				})),
				widget.Sized{W: 10},
				widget.Expand(button(th, "Forget it all", func() {
					lib.Rank.Reset()
					s.SetState(func() { s.confirmReset = false })
				})),
			))
	} else {
		kids = append(kids, secondaryButton(th, "Start ranking over", func() {
			s.SetState(func() { s.confirmReset = true })
		}))
	}
	return card(th, colStart(kids...))
}

func (s *settingsState) aboutCard(th theme.Theme, lib *library.Library) widget.Widget {
	_, last, lastErr := lib.Refreshing()
	line := "Never refreshed."
	if !last.IsZero() {
		line = "Last refreshed " + ago(last) + " ago."
	}
	kids := []widget.Widget{
		widget.Text{S: line, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 4},
		widget.Text{S: fmt.Sprintf("%d sources subscribed.", len(lib.Subs.All())),
			Size: th.Type.Caption, Color: th.Muted},
		widget.Sized{H: 4},
		widget.Text{S: "Data is stored at " + library.DataDir(),
			Size: th.Type.Caption, Color: th.Muted, Wrap: true},
	}
	if lastErr != "" {
		kids = append(kids, widget.Sized{H: 6},
			widget.Text{S: "Last error: " + lastErr, Size: th.Type.Caption, Color: th.Warning, Wrap: true})
	}
	return card(th, colStart(kids...))
}

// settingRow is a labelled switch with the explanation underneath, because a
// toggle whose consequence is not stated is a toggle nobody touches.
func settingRow(th theme.Theme, title, detail string, on bool, onTap func()) widget.Widget {
	row := widget.Row(
		widget.Expand(colStart(
			widget.Text{S: title, Size: th.Type.Body, Color: th.Text, Wrap: true},
			widget.Sized{H: 3},
			widget.Text{S: detail, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		)),
		widget.Sized{W: 12},
		toggle(th, on, onTap),
	)
	row.CrossAlign = layout.CrossCenter
	return row
}

func humanBytes(n int64) string {
	switch {
	case n < 1<<10:
		return fmt.Sprintf("%d B", n)
	case n < 1<<20:
		return fmt.Sprintf("%.0f KB", float64(n)/(1<<10))
	case n < 1<<30:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	default:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	}
}
