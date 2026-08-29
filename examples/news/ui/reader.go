package ui

import (
	"fmt"
	"time"

	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/examples/news/internal/store"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// readerPage shows one article in full: the extracted body, its pictures, at
// the reading size you chose.
//
// It carries only the article's ID, not the article, so the page is a plain
// serialisable value — which is what lets a development hot-restart put you
// back on the piece you were reading.
type readerPage struct {
	ItemID string
}

func (readerPage) CreateState() widget.State { return &readerState{} }

type readerState struct {
	widget.StateBase[readerPage]
	item   *store.Item
	blocks []block
	found  bool
	// builtDark records the scheme the blocks were parsed for. Span colours are
	// baked in at parse time, so switching to dark mode with a reader open left
	// the body in light-mode ink on a dark page.
	built     bool
	builtDark bool

	// lib is captured at mount because Dispose takes no context — see the
	// assertion below it.
	lib *library.Library

	opened   time.Time
	finished bool
	voted    int // -1, 0, +1
	scroll   widget.ScrollController
}

func (s *readerState) Init(ctx widget.Ctx) {
	lib := env(ctx).Lib
	s.lib = lib
	s.opened = time.Now()
	it, ok := lib.Item(s.W().ItemID)
	s.item, s.found = it, ok
	if !ok {
		return
	}
	// Opening counts immediately; finishing is decided when the page closes.
	lib.MarkRead(it, false)

	// Pull anything the prefetch missed — an article opened from a queue that
	// was refreshed on a bad connection.
	go lib.PrefetchItem(ctx.Context(), it)
}

// Dispose is where the reading signal is recorded. Whether an article was
// finished is not knowable while it is open, only when it is left: scrolled to
// the end, or dwelled on long enough that it must have been read.
//
// It takes no arguments, because widget.Disposer is `interface{ Dispose() }`.
// Written as Dispose(ctx) it compiles perfectly, satisfies nothing, and is
// never called — the strongest signal the ranking model can get, silently
// dropped. The assertion below is what makes that a build failure.
func (s *readerState) Dispose() {
	if !s.found || s.item == nil || s.lib == nil {
		return
	}
	if s.finished || s.fitOnScreen() || s.longEnough() {
		s.lib.MarkRead(s.item, true)
	}
}

// fitOnScreen covers the article short enough to need no scrolling at all.
// Without this it can never be finished: the end-reached signal only fires for
// content that scrolls, and the dwell fallback wants half its reading time,
// which for a two-hundred-word note is longer than reading it takes.
//
// A few seconds of dwell still has to pass, so that opening something and
// immediately going back is not mistaken for reading it.
func (s *readerState) fitOnScreen() bool {
	return s.scroll.MaxOffset() <= 0 && time.Since(s.opened) >= shortReadDwell
}

// shortReadDwell is how long an article that fits on one screen must be looked
// at before it counts as read.
var shortReadDwell = 6 * time.Second

// readerState must actually be a Disposer; see Dispose.
var _ widget.Disposer = (*readerState)(nil)

// longEnough is the dwell-time fallback for articles finished without the
// scroll reaching the very bottom — footnotes, comment sections and related-post
// blocks all sit below the last thing anyone reads. Half the estimated reading
// time is deliberately generous: the cost of a false positive is one wrong
// signal, and the cost of a false negative is never learning from a good read.
func (s *readerState) longEnough() bool {
	mins := readingMinutes(s.item)
	if mins <= 0 {
		return false
	}
	want := min(time.Duration(mins)*time.Minute/2, 4*time.Minute)
	return time.Since(s.opened) >= want
}

func (s *readerState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	if !s.found {
		return page(ctx, header(th, "Article", "", backButton(ctx)),
			centered(th, "Not found", "This article is no longer in the store."))
	}
	it := s.item

	// Parse once, and again only if the scheme flipped between light and dark.
	if !s.built || s.builtDark != th.Dark {
		s.blocks = parseArticle(it.ContentHTML, paletteOf(th))
		s.built, s.builtDark = true, th.Dark
	}

	size := th.Type.Body * env(ctx).Lib.Prefs.Scale()
	openURL := func(u string) { _ = ctx.OpenURL(u) }

	// header + lead image + body blocks + footer.
	const leading, trailing = 1, 1
	count := leading + len(s.blocks) + trailing

	body := widget.LazyList{
		Count:           count,
		EstimatedExtent: 90,
		Controller:      &s.scroll,
		Build: func(i int) widget.Widget {
			switch {
			case i == 0:
				return s.articleHead(th, it, size, openURL)
			case i == count-1:
				return s.articleFoot(ctx, th, it)
			default:
				return widget.Padding{Insets: geom.Insets{Left: 18, Right: 18},
					Child: buildBlock(th, s.blocks[i-leading], size, openURL)}
			}
		},
		OnEndReached: func() {
			// Reaching the bottom is the clearest evidence there is.
			if !s.finished {
				s.finished = true
			}
		},
	}

	return page(ctx, header(th, it.FeedName, ago(it.Published), backButton(ctx),
		headerAction(th, "A-", func() { s.nudgeText(ctx, -0.1) }),
		headerAction(th, "A+", func() { s.nudgeText(ctx, +0.1) }),
	), body)
}

// nudgeText changes the reading size in steps and rebuilds. The control is in
// the header rather than buried in settings because the moment you want it is
// while reading, not before.
func (s *readerState) nudgeText(ctx widget.Ctx, delta float64) {
	p := env(ctx).Lib.Prefs
	v := float64(p.Scale()) + delta
	v = max(0.8, min(2.0, v))
	p.SetScale(v)
	s.SetState(func() {})
}

// articleHead is the title block: headline, byline, and the lead picture.
func (s *readerState) articleHead(th theme.Theme, it *store.Item, size float32, openURL func(string)) widget.Widget {
	kids := []widget.Widget{
		widget.Text{S: it.Title, Font: "bold", Size: size * 1.6, Color: th.Text, Wrap: true},
		widget.Sized{H: 8},
	}

	meta := it.FeedName
	if it.Author != "" {
		meta += " · " + it.Author
	}
	meta += " · " + it.Published.Format("Jan 2, 2006")
	if m := readingMinutes(it); m > 0 {
		meta += fmt.Sprintf(" · %d min read", m)
	}
	kids = append(kids,
		widget.Text{S: meta, Size: size * 0.8, Color: th.Muted, Wrap: true},
		widget.Sized{H: 14},
	)

	// The lead image goes above the body, full width, as a publication would
	// run it — but only when the body does not open with a picture of its own.
	if it.LeadImage != "" && !bodyOpensWithImage(s.blocks) {
		kids = append(kids, Img{URL: it.LeadImage, MaxH: 420}, widget.Sized{H: 14})
	}

	if it.Source == store.SourceSummary {
		kids = append(kids, summaryNotice(th, it, size, openURL), widget.Sized{H: 12})
	}

	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStretch
	return widget.Padding{Insets: geom.Insets{Left: 18, Right: 18, Top: 16}, Child: col}
}

// bodyOpensWithImage reports whether the article's own markup starts with a
// picture, in which case the feed's lead image must not be shown above it.
//
// Comparing the two URLs is not enough and was the first thing tried: a
// publisher's feed enclosure and the same photograph inside the article come
// from the same CDN at different crops, so the addresses differ and the reader
// cheerfully showed the picture twice. Position is the reliable signal — a body
// that opens with an image is already leading with one.
func bodyOpensWithImage(blocks []block) bool {
	for i, b := range blocks {
		if i > 2 {
			return false
		}
		if b.kind == blockImage {
			return true
		}
	}
	return false
}

// summaryNotice explains an article that is only a teaser, and offers the way
// out: open the publisher's page. This is what a paywalled source looks like
// before a subscription is set up, and saying so is better than presenting two
// sentences as though they were the article.
func summaryNotice(th theme.Theme, it *store.Item, size float32, openURL func(string)) widget.Widget {
	msg := "This feed publishes a summary only."
	if it.ExtractError != "" {
		msg = "Only a summary could be retrieved."
	}
	col := widget.Column(
		widget.Text{S: msg, Size: size * 0.85, Color: th.Text, Wrap: true},
		widget.Sized{H: 6},
		widget.Rich{
			Spans:  []layout.RichSpan{{Text: "Open the full article →", Color: th.Primary, Link: it.Link, Underline: true}},
			Size:   size * 0.85,
			OnLink: openURL,
		},
	)
	col.CrossAlign = layout.CrossStart
	return widget.Decorated{Color: th.Surface, Radius: th.Radius,
		Child: widget.Padding{All: 12, Child: col}}
}

// articleFoot is what you reach at the end: the two buttons that teach the
// ranking model, and the link to the original.
func (s *readerState) articleFoot(ctx widget.Ctx, th theme.Theme, it *store.Item) widget.Widget {
	lib := env(ctx).Lib
	nav := ctx.MustOf[widget.Nav]()

	vote := func(up bool) {
		lib.Vote(it, up)
		s.SetState(func() {
			if up {
				s.voted = 1
			} else {
				s.voted = -1
			}
		})
	}

	upLabel, downLabel := "More like this", "Less like this"
	switch s.voted {
	case 1:
		upLabel = "More like this ●"
	case -1:
		downLabel = "Less like this ●"
	}

	buttons := widget.Row(
		widget.Expand(secondaryButton(th, downLabel, func() { vote(false) })),
		widget.Sized{W: 10},
		widget.Expand(secondaryButton(th, upLabel, func() { vote(true) })),
	)

	kids := []widget.Widget{
		divider(th),
		widget.Sized{H: 16},
		buttons,
		widget.Sized{H: 12},
		widget.Row(
			widget.Expand(secondaryButton(th, "Open original →", func() { _ = ctx.OpenURL(it.Link) })),
			widget.Sized{W: 10},
			widget.Expand(secondaryButton(th, "Next unread", func() {
				nav.Pop()
			})),
		),
		widget.Sized{H: 28},
	}
	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStretch
	return widget.Padding{Insets: geom.Insets{Left: 18, Right: 18, Top: 20}, Child: col}
}
