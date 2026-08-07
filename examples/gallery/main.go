// Command gallery is a polished showcase of gophics's higher-level widgets,
// built entirely on procedural (network-free) content so it runs anywhere and
// stays testable headless.
//
// It exercises, in one cohesive app:
//   - Navigator with Hero shared-element transitions (a card's swatch flies
//     into the detail header and back)
//   - LazyList with pull-to-refresh (reshuffles the feed)
//   - Dismissible cards (swipe to remove)
//   - SelectableText (drag-select + copy the detail body)
//   - AnimatedScale (the like button pops)
//   - a reverse / bottom-anchored LazyList (the comments read like a chat log)
//   - dark/light theming from the platform color scheme
package main

import (
	"fmt"
	"image"
	"image/color"
	"log"
	"math"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Chrome (backgrounds, surfaces, text, accents) is drawn entirely from the
// theme package's design tokens, provided at the root and read via
// theme.Of(ctx), so the whole showcase follows the platform light/dark scheme
// and demonstrates gophics's own design system. The procedural swatch imagery
// below is content, not chrome, and keeps its own hues.

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
// real decoded images — exercising the image decode/blit/scale path — instead
// of flat gradients.
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

func bodyFor(c card) string {
	return fmt.Sprintf("%s is a study in restraint by @%s — long exposures, "+
		"muted palettes, and a stubborn belief that the frame should breathe. "+
		"Try selecting this text: press and drag, then Cmd/Ctrl+C to copy.", c.title, c.author)
}

func commentsFor(c card) []string {
	base := []string{
		"the gradient in the corner is doing a lot of work here",
		"@" + c.author + " what lens was this?",
		"saved to my board immediately",
		"that second color choice is inspired",
		"reminds me of early Saul Leiter",
		"how long was the exposure?",
		"the restraint is the whole point",
		"instant favorite, no notes",
	}
	return base
}

// --- Root & feed -------------------------------------------------------------

// Gallery is the root widget: a Navigator over the feed.
type Gallery struct{}

func (Gallery) CreateState() widget.State { return &galleryState{} }

type galleryState struct{ widget.StateBase[Gallery] }

func (s *galleryState) Build(ctx widget.Ctx) widget.Widget {
	// Resolve the theme from the platform color scheme and provide it to the
	// tree; every page below reads colors with theme.Of(ctx), so the whole
	// showcase follows light/dark automatically.
	th := theme.Auto(ctx)
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Fill{Color: th.Bg, Child: widget.Navigator{Home: feedPage{}}},
	}
}

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
	// Regenerate from a new seed on the next frame (no network to await; a
	// real app would fetch here, then clear refreshing when done).
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
		Refreshing:      s.refreshing,
		OnRefresh:       func() { s.refresh(ctx) },
		Build: func(i int) widget.Widget {
			c := s.cards[i]
			row := widget.Interactive{
				Handler: widget.Handler{OnTap: func() { nav.Push(detailPage{card: c}) }},
				Child:   cardTile(th, c),
			}
			return widget.WithKey{Key: c.id, Child: widget.Dismissible{
				OnDismissed: func() { s.remove(c.id) },
				Background:  dismissPanel(th),
				Child:       row,
			}}
		},
	}
	return scaffold(ctx, "Gallery", "pull to refresh · swipe a card away", widget.Expand(list))
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
	// theme.Card provides the surface chrome (Surface fill + themed radius).
	return widget.Padding{All: 10, Child: theme.Card{Pad: 12, Child: row}}
}

func dismissPanel(th theme.Theme) widget.Widget {
	label := widget.Text{S: "remove", Font: theme.FontBold, Size: th.Type.Label, Color: th.OnPrimary}
	return widget.Padding{All: 10, Child: widget.Decorated{Color: th.Danger, Radius: th.Radius,
		Child: widget.Padding{Insets: geom.Insets{Left: 24, Right: 24},
			Child: widget.Row(label, widget.Spacer(), label)}}}
}

// --- Detail ------------------------------------------------------------------

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
	// Animate the glyph's font size (re-rasterized each frame, so it stays
	// crisp) rather than scaling a cached glyph bitmap, which would soften it.
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
		backChip(nav),
		widget.Sized{H: 10},
		widget.Text{S: c.title, Font: theme.FontBold, Size: th.Type.Title, Color: th.Text, Wrap: true},
		widget.Sized{H: 4},
		widget.Text{S: "by @" + c.author, Size: th.Type.Label, Color: th.Muted},
		widget.Sized{H: 14},
		widget.SelectableText{S: bodyFor(c), Size: th.Type.Body, Color: th.Text, Wrap: true},
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

// backChip is the themed "back" button — a ready-made theme.Button, so the
// showcase demonstrates the design system's own control.
func backChip(nav widget.Nav) widget.Widget {
	return theme.Button{Label: "← back", OnTap: func() { nav.Pop() }}
}

// commentList is a reverse (bottom-anchored) list: newest comment rests at the
// bottom, scroll up for older — the chat-log layout.
func commentList(th theme.Theme, c card) widget.Widget {
	comments := commentsFor(c)
	list := widget.LazyList{
		Count:           len(comments),
		EstimatedExtent: 52,
		Reverse:         true,
		Build: func(i int) widget.Widget {
			return widget.Padding{Insets: geom.Insets{Left: 16, Right: 16, Top: 5, Bottom: 5},
				Child: widget.Decorated{Color: th.Surface, Radius: th.Radius, Child: widget.Padding{All: 12,
					Child: widget.Text{S: comments[i], Size: th.Type.Body, Color: th.Text, Wrap: true}}}}
		},
	}
	return list
}

// --- Shared chrome & helpers -------------------------------------------------

func scaffold(ctx widget.Ctx, title, subtitle string, body widget.Widget) widget.Widget {
	th := theme.Of(ctx)
	head := widget.Column(
		theme.Title(title),
		widget.Sized{H: 2},
		theme.Label(subtitle),
	)
	head.CrossAlign = layout.CrossStart
	col := widget.Column(
		widget.Padding{Insets: geom.Insets{Left: 16, Right: 16, Top: 20, Bottom: 8}, Child: head},
		body,
	)
	col.CrossAlign = layout.CrossStretch
	return widget.Fill{Color: th.Bg, Child: col}
}

func heroTag(id int) string { return fmt.Sprintf("swatch-%d", id) }

// swatch draws a real image clipped to a rounded rectangle — the card
// thumbnail and detail header, and the Hero that flies between them (the
// image scales with bilinear filtering, so the flight stays smooth). W or H
// of 0 fills the available space.
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

// hsv converts to an sRGB-ish Color (h,s,v in [0,1]).

func mod01(v float32) float32 {
	for v >= 1 {
		v -= 1
	}
	return v
}

func main() {
	err := app.Run(Gallery{}, app.Config{
		Title:        "Gophics Gallery",
		Size:         geom.Size{W: 420, H: 680},
		Background:   theme.Dark().Bg,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	})
	if err != nil {
		log.Fatal(err)
	}
}
