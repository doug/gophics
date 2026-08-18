package ui

import (
	"fmt"
	"math"
	"time"

	"github.com/doug/gophics/examples/news/internal/store"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// whyPage explains one article's position in the queue.
//
// A ranking you cannot interrogate is a ranking you end up fighting. The model
// is small enough to explain honestly — every score is a sum of named terms —
// so this shows the actual arithmetic rather than a reassuring paraphrase, and
// gives you the two buttons that change it.
type whyPage struct {
	ItemID string
}

func (whyPage) CreateState() widget.State { return &whyState{} }

type whyState struct {
	widget.StateBase[whyPage]
	item  *store.Item
	found bool
}

// Init resolves the article once. Doing it in Build meant a lookup per frame,
// and a lookup that misses the queue's index walks every file in the store.
func (s *whyState) Init(ctx widget.Ctx) {
	s.item, s.found = env(ctx).Lib.Item(s.W().ItemID)
}

func (s *whyState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	lib := env(ctx).Lib
	nav := widget.MustOf[widget.Nav](ctx)

	it, ok := s.item, s.found
	if !ok {
		return page(ctx, header(th, "Why this?", "", backButton(ctx)),
			centered(th, "Not found", "This article is no longer in the store."))
	}

	score := lib.Rank.Score(it, lib.Meta, time.Now())
	terms := lib.Rank.Explain(it, lib.Meta, time.Now())
	trained := lib.Rank.Trained()

	kids := []widget.Widget{
		card(th, colStart(
			widget.Text{S: it.Title, Font: "bold", Size: th.Type.Heading, Color: th.Text, Wrap: true},
			widget.Sized{H: 10},
			widget.Text{S: fmt.Sprintf("Ranked %d%% likely to be worth your time", int(math.Round(score*100))),
				Size: th.Type.Body, Color: th.Primary},
			widget.Sized{H: 4},
			widget.Text{S: confidenceNote(trained), Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		)),
	}

	if len(terms) == 0 {
		kids = append(kids, card(th, widget.Text{
			S:    "Nothing has pushed this article either way yet — it is sitting at the default.",
			Size: th.Type.Body, Color: th.Muted, Wrap: true}))
	} else {
		rows := []widget.Widget{
			widget.Text{S: "WHAT MOVED IT", Font: "bold", Size: th.Type.Caption, Color: th.Muted},
			widget.Sized{H: 4},
		}
		for _, t := range terms {
			rows = append(rows, widget.Sized{H: 10}, termRow(th, t.Label, t.Detail, t.Weight))
		}
		kids = append(kids, card(th, colStart(rows...)))
	}

	kids = append(kids, card(th, colStart(
		widget.Text{S: "Teach it", Font: "bold", Size: th.Type.Body, Color: th.Text},
		widget.Sized{H: 6},
		widget.Text{S: "These count for much more than a tap or a scroll, and apply to the source, the topic and the author together.",
			Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 12},
		widget.Row(
			widget.Expand(secondaryButton(th, "Less like this", func() {
				lib.Vote(it, false)
				nav.Pop()
			})),
			widget.Sized{W: 10},
			widget.Expand(secondaryButton(th, "More like this", func() {
				lib.Vote(it, true)
				nav.Pop()
			})),
		),
	)))

	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStretch
	return page(ctx, header(th, "Why this?", it.FeedName, backButton(ctx)),
		widget.Scroll{Child: col})
}

// confidenceNote is honest about how much of the ranking is still the catalog's
// opinion rather than the reader's own behaviour.
func confidenceNote(trained float64) string {
	switch {
	case trained < 10:
		return "Mostly the source ratings so far — the app has barely seen you read anything yet."
	case trained < 60:
		return "Part source ratings, part what you have read so far."
	default:
		return "Mostly learned from what you have read."
	}
}

// termRow draws one contribution as a label and a bar either side of centre, so
// what pushed an article up and what pushed it down are distinguishable at a
// glance rather than by reading signs.
func termRow(th theme.Theme, label, detail string, weight float64) widget.Widget {
	const fullScale = 2.5 // the clamp ceiling used by the model
	f := math.Abs(weight) / fullScale
	f = math.Min(1, f)
	width := float32(6 + 90*f)

	color := th.Success
	if weight < 0 {
		color = th.Danger
	}
	bar := widget.Decorated{Color: color, Radius: 2, Child: widget.Sized{W: width, H: 6}}

	// Negative bars grow leftward from the centre, positive ones rightward, so
	// the two directions are told apart by shape rather than by reading a sign.
	var track widget.Widget
	if weight < 0 {
		track = widget.Row(
			widget.Sized{W: 100, Child: widget.Row(widget.Expand(widget.Sized{}), bar)},
			widget.Sized{W: 100},
		)
	} else {
		track = widget.Row(
			widget.Sized{W: 100},
			widget.Sized{W: 100, Child: widget.Row(bar, widget.Expand(widget.Sized{}))},
		)
	}

	text := label
	if detail != "" {
		text += " — " + detail
	}
	col := colStart(
		widget.Text{S: text, Size: th.Type.Label, Color: th.Text, Wrap: true, MaxLines: 2, Ellipsis: true},
		widget.Sized{H: 5},
		track,
	)
	return col
}

// colStart is a column aligned to the leading edge, which is what almost every
// stack of text in this app wants.
func colStart(kids ...widget.Widget) widget.Widget {
	c := widget.Column(kids...)
	c.CrossAlign = layout.CrossStart
	return c
}

// colStretch is a column whose children span the full width.
//
// This is not a stylistic preference. widget.Column defaults to CrossCenter,
// which gives each child its intrinsic cross-axis size — and a LazyList has no
// intrinsic width, so a list built into a plain Column lays out at zero width
// and renders nothing at all while the chrome around it looks perfect. Anything
// holding a list, a divider, or a full-bleed row wants this.
func colStretch(kids ...widget.Widget) widget.Widget {
	c := widget.Column(kids...)
	c.CrossAlign = layout.CrossStretch
	return c
}
