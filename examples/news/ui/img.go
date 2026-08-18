package ui

import (
	"context"
	"image"
	"time"

	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Img draws a picture from the library's disk cache, fetching it only if the
// cache has not already got it.
//
// gophics ships widget.NetworkImage, which is the right widget for an app that
// is always online: it caches decoded images in memory and fetches over HTTP.
// A reader needs the other half. Pictures are downloaded ahead of time during a
// refresh, and the memory cache does not survive the app being backgrounded on
// the walk to the platform — so this draws from disk, and only reaches the
// network for something the prefetch missed.
//
// Fit is the important layout property: an article image knows its own aspect
// ratio and should keep it while filling the column width. Setting both W and
// H on widget.Image would stretch it.
type Img struct {
	URL string
	// W and H, when both set, fix the drawn size (a thumbnail). When only W is
	// set — or neither, meaning fill the available width — the height follows
	// the picture's own proportions.
	W, H float32
	// MaxH caps a very tall image so a portrait photograph cannot push the
	// article's first paragraph off the screen.
	//
	// There is deliberately no corner-radius option: the framework can round a
	// painted surface but cannot clip a child to it, so an image drawn "rounded"
	// would simply have square corners. Pictures are drawn square on purpose.
	MaxH float32
}

func (i Img) CreateState() widget.State { return &imgState{} }

type imgState struct {
	widget.StateBase[Img]
	url    string
	img    image.Image
	failed bool
	post   func(func())
	cache  *library.ImageCache
}

func (s *imgState) Init(ctx widget.Ctx) {
	s.post = ctx.Post()
	s.cache = env(ctx).Lib.Images
	s.start(s.W().URL)
}

// start loads off the frame goroutine. A warm picture is taken from the memory
// cache synchronously so a scroll back up does not flash a placeholder.
func (s *imgState) start(url string) {
	s.url, s.img, s.failed = url, nil, false
	if url == "" || s.cache == nil {
		return
	}
	if img, ok := s.cache.Cached(url); ok {
		s.img = img
		return
	}
	cache, post := s.cache, s.post
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		img, err := cache.Load(ctx, url)
		if post == nil {
			return
		}
		post(func() {
			if s.url != url {
				return // the row was recycled onto a different article
			}
			s.SetState(func() {
				if err != nil {
					s.failed = true
				} else {
					s.img = img
				}
			})
		})
	}()
}

func (s *imgState) Build(ctx widget.Ctx) widget.Widget {
	w := s.W()
	if u := w.URL; u != s.url {
		s.start(u)
	}
	th := theme.Of(ctx)

	// A picture that is missing must take no space at all. A grey rectangle
	// where a photograph failed is worse than the text closing over the gap.
	if s.failed || s.url == "" {
		if w.W > 0 && w.H > 0 {
			return placeholder(th, w)
		}
		return widget.Sized{}
	}
	if s.img == nil {
		return placeholder(th, w)
	}

	b := s.img.Bounds()
	if b.Dx() == 0 || b.Dy() == 0 {
		return widget.Sized{}
	}
	aspect := float32(b.Dy()) / float32(b.Dx())

	// Fixed size: a thumbnail, drawn as given.
	if w.W > 0 && w.H > 0 {
		return widget.Image{Src: s.img, W: w.W, H: w.H}
	}
	// Fixed width: height follows the picture.
	if w.W > 0 {
		h := w.W * aspect
		if w.MaxH > 0 && h > w.MaxH {
			h = w.MaxH
		}
		return widget.Image{Src: s.img, W: w.W, H: h}
	}
	// Fill the column: measure at build time so the aspect ratio survives
	// rotation and window resizing.
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		width := cs.Max.W
		if !cs.BoundedW() || width <= 0 {
			width = float32(b.Dx())
		}
		h := width * aspect
		if w.MaxH > 0 && h > w.MaxH {
			h = w.MaxH
		}
		return widget.Image{Src: s.img, W: width, H: h}
	}}
}

// placeholder reserves a thumbnail's space while it loads, so a list does not
// jump as pictures arrive.
func placeholder(th theme.Theme, w Img) widget.Widget {
	if w.W <= 0 || w.H <= 0 {
		return widget.Sized{}
	}
	return widget.Decorated{
		Color: th.Surface,
		Child: widget.Sized{W: w.W, H: w.H},
	}
}
