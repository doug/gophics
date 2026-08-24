package ui

// Navigator with Hero shared-element transitions.
//
// This is the one section that is a full page of its own rather than a
// scrolling demo list, and it earns that: a push transition cannot be shown
// inside a list, because the transition *is* the page changing. The Hero swatch
// flies from the row into the detail header, which is the whole point.
//
// Pull-to-refresh and swipe-to-dismiss used to be tangled into this same feed;
// they are their own sections now (sections_gestures.go), so each demo has one
// subject. The procedural-image helpers stay here as content, not chrome — the
// gesture sections share them.

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// --- Data (procedural, deterministic) ---------------------------------------

type card struct {
	id            int
	title, author string
	img           image.Image // a real decoded image (procedurally generated)
	likes         int
}

var titles = []string{
	"Aurora over the fjords", "Concrete and light", "Tidepools at dawn",
	"The last analog radio", "Rooftop gardens of Kyoto", "Salt flats, no horizon",
	"Neon in the rain", "A quiet cartography", "Machines that dream",
	"Paper airplanes to Mars", "The color of patience", "Signal and moss",
}
var authors = []string{"mira", "koji", "petra", "sol", "wren", "arturo", "ines", "dax"}

// swatchHues returns two gradient colors derived from an index, so the feed is
// colorful and deterministic (no RNG — regenerating is reproducible).
func swatchHues(i int) (paint.Color, paint.Color) {
	h := float32(i*47%360) / 360
	return paint.HSV(h*360, 0.55, 0.95), paint.HSV(mod01(h+0.12)*360, 0.65, 0.75)
}

func makeCards(n, seed int) []card {
	cards := make([]card, n)
	for i := range cards {
		k := i + seed
		cards[i] = card{
			id:     k*100 + 7, // stable id independent of position
			title:  titles[k%len(titles)],
			author: authors[k%len(authors)],
			img:    genImage(k),
			likes:  (k*37)%90 + 3,
		}
	}
	return cards
}

// genImage builds a deterministic, photographic-ish image (layered plasma over
// a two-tone gradient, with a vignette and a little grain) so the feed shows
// real decoded images — exercising the image decode/blit/scale path.
func genImage(seed int) image.Image {
	const n = 220 // large enough to stay crisp scaled up into the detail header
	img := image.NewRGBA(image.Rect(0, 0, n, n))
	a, b := swatchHues(seed)
	fs := float64(seed)
	for y := 0; y < n; y++ {
		fy := float64(y) / n
		for x := 0; x < n; x++ {
			fx := float64(x) / n
			v := 0.5 + 0.25*math.Sin((fx*3.7+fs)*math.Pi) +
				0.22*math.Cos((fy*2.9-fs*0.7)*math.Pi) +
				0.18*math.Sin((fx+fy)*5*math.Pi+fs)
			v = clamp01f(v)
			t := float32(v)*0.7 + float32(fy)*0.3
			col := paint.Lerp(a, b, t)
			dx, dy := fx-0.5, fy-0.5
			vig := float32(clamp01f(1 - (dx*dx+dy*dy)*0.9))
			if vig < 0.4 {
				vig = 0.4
			}
			grain := float32((x*131+y*197+seed*17)%19)/19*0.06 - 0.03
			img.SetRGBA(x, y, color.RGBA{
				R: to8(col.R*vig + grain),
				G: to8(col.G*vig + grain),
				B: to8(col.B*vig + grain),
				A: 255,
			})
		}
	}
	return img
}

func clamp01f(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func to8(v float32) uint8 {
	if v < 0 {
		v = 0
	}
	if v > 1 {
		v = 1
	}
	return uint8(v * 255)
}

func mod01(v float32) float32 {
	for v >= 1 {
		v -= 1
	}
	return v
}

func bodyFor(c card) string {
	return fmt.Sprintf("%s is a study in restraint by @%s — long exposures, "+
		"muted palettes, and a stubborn belief that the frame should breathe. "+
		"Try selecting this text: press and drag, then Cmd/Ctrl+C to copy.", c.title, c.author)
}

func commentsFor(c card) []string {
	return []string{
		"the gradient in the corner is doing a lot of work here",
		"@" + c.author + " what lens was this?",
		"saved to my board immediately",
		"that second color choice is inspired",
		"reminds me of early Saul Leiter",
		"how long was the exposure?",
		"the restraint is the whole point",
		"instant favorite, no notes",
	}
}

// --- Feed page ---------------------------------------------------------------

type feedPage struct{}

func (feedPage) CreateState() widget.State { return &feedState{} }

type feedState struct {
	widget.StateBase[feedPage]
	cards      []card
	seed       int
	refreshing bool
}

// feedHook lets tests observe the mounted feed.
var feedHook func(*feedState)

func (s *feedState) Init(widget.Ctx) {
	if feedHook != nil {
		feedHook(s)
	}
	s.cards = makeCards(12, 0)
}

func (s *feedState) refresh(ctx widget.Ctx) {
	s.SetState(func() { s.refreshing = true })
	// Regenerate from a new seed on the next frame (no network to await; a real
	// app would fetch here, then clear refreshing when done).
	post := ctx.Post()
	post(func() {
		s.SetState(func() {
			s.seed += 12
			s.cards = makeCards(12, s.seed)
			s.refreshing = false
		})
	})
}

func (s *feedState) remove(id int) {
	s.SetState(func() {
		for i, c := range s.cards {
			if c.id == id {
				s.cards = append(s.cards[:i], s.cards[i+1:]...)
				return
			}
		}
	})
}

func (s *feedState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	nav := widget.MustOf[widget.Nav](ctx)
	list := widget.LazyList{
		Count:           len(s.cards),
		EstimatedExtent: 96,
		Build: func(i int) widget.Widget {
			c := s.cards[i]
			return widget.Interactive{
				Handler: widget.Handler{OnTap: func() { nav.Push(detailPage{card: c}) }},
				Child:   cardTile(th, c),
			}
		},
	}
	return scaffold(ctx, "Navigator & Hero", "tap a row — the swatch flies into the detail page", widget.Expand(list))
}

func cardTile(th theme.Theme, c card) widget.Widget {
	info := widget.Column(
		widget.Text{S: c.title, Font: theme.FontBold, Size: th.Type.Heading, Color: th.Text, MaxLines: 1, Ellipsis: true},
		widget.Sized{H: 4},
		widget.Text{S: "@" + c.author + " · " + fmt.Sprintf("%d likes", c.likes), Size: th.Type.Label, Color: th.Muted},
	)
	info.CrossAlign = layout.CrossStart
	row := widget.Row(
		widget.Hero{Tag: heroTag(c.id), Child: swatch(c.img, 60, 60, 14)},
		widget.Sized{W: 14},
		widget.Expand(info),
	)
	return widget.Padding{All: 8, Child: theme.Card{Pad: 12, Child: row}}
}

func dismissPanel(th theme.Theme) widget.Widget {
	label := widget.Text{S: "remove", Font: theme.FontBold, Size: th.Type.Label, Color: th.OnPrimary}
	return widget.Padding{All: 8, Child: widget.Decorated{Color: th.Danger, Radius: th.Radius,
		Child: widget.Padding{Insets: geom.Insets{Left: 24, Right: 24},
			Child: widget.Row(label, widget.Spacer(), label)}}}
}

// --- Detail page -------------------------------------------------------------

type detailPage struct{ card card }

func (d detailPage) CreateState() widget.State { return &detailState{card: d.card} }

type detailState struct {
	widget.StateBase[detailPage]
	card  card
	liked bool
}

func (s *detailState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	nav := widget.MustOf[widget.Nav](ctx)
	c := s.card

	// Full-bleed hero header — tapping it (or the back chip) pops.
	header := widget.Interactive{
		Handler: widget.Handler{OnTap: func() { nav.Pop() }},
		Child:   widget.Hero{Tag: heroTag(c.id), Child: swatch(c.img, 0, 200, 0)},
	}

	heartSize := float32(22)
	heartColor := th.Muted
	if s.liked {
		heartSize, heartColor = 30, th.Danger
	}
	likeCount := c.likes
	if s.liked {
		likeCount++
	}
	// Animate the glyph's font size (re-rasterized each frame, so it stays crisp)
	// rather than scaling a cached glyph bitmap, which would soften it.
	like := widget.Interactive{
		Handler: widget.Handler{OnTap: func() { s.SetState(func() { s.liked = !s.liked }) }},
		Child: widget.Row(
			widget.Sized{W: 30, Child: widget.Center(
				widget.AnimateFloat(heartSize, 140*time.Millisecond, func(sz float32) widget.Widget {
					return widget.Text{S: "♥", Size: sz, Color: heartColor}
				}))},
			widget.Sized{W: 8},
			widget.Text{S: fmt.Sprintf("%d", likeCount), Size: th.Type.Body, Color: th.Text},
		),
	}

	body := widget.Column(
		theme.Button{Label: "← back", OnTap: func() { nav.Pop() }},
		widget.Sized{H: 10},
		widget.Text{S: c.title, Font: theme.FontBold, Size: th.Type.Title, Color: th.Text, Wrap: true},
		widget.Sized{H: 4},
		widget.Text{S: "by @" + c.author, Size: th.Type.Label, Color: th.Muted},
		widget.Sized{H: 14},
		widget.SelectableText{S: bodyFor(c), Size: th.Type.Body, Color: th.Text, Wrap: true, SelectionColor: th.Selection},
		widget.Sized{H: 16},
		like,
		widget.Sized{H: 18},
		widget.Text{S: "COMMENTS", Font: theme.FontBold, Size: th.Type.Caption, Color: th.Muted},
	)
	body.CrossAlign = layout.CrossStart

	page := widget.Column(
		header,
		widget.Padding{All: 16, Child: body},
		widget.Expand(commentList(th, c)),
	)
	page.CrossAlign = layout.CrossStretch
	return widget.Fill{Color: th.Bg, Child: page}
}

// commentList is a reverse (bottom-anchored) list: newest comment rests at the
// bottom, scroll up for older — the chat-log layout.
func commentList(th theme.Theme, c card) widget.Widget {
	comments := commentsFor(c)
	return widget.LazyList{
		Count:           len(comments),
		EstimatedExtent: 52,
		Reverse:         true,
		Build: func(i int) widget.Widget {
			return widget.Padding{Insets: geom.Insets{Left: 16, Right: 16, Top: 5, Bottom: 5},
				Child: widget.Decorated{Color: th.Surface, Radius: th.Radius, Child: widget.Padding{All: 12,
					Child: widget.Text{S: comments[i], Size: th.Type.Body, Color: th.Text, Wrap: true}}}}
		},
	}
}

func heroTag(id int) string { return fmt.Sprintf("swatch-%d", id) }

// swatch draws a real image clipped to a rounded rectangle — the card
// thumbnail and detail header, and the Hero that flies between them. W or H of 0
// fills the available space.
func swatch(img image.Image, w, h, radius float32) widget.Widget {
	return widget.Canvas{W: w, H: h, Draw: func(c paint.Canvas, size geom.Size) {
		r := geom.Rect{Max: size.Pt()}
		if radius > 0 {
			c.PushClipRRect(r, radius)
			c.Image(img, r)
			c.PopClip()
		} else {
			c.Image(img, r)
		}
	}}
}
