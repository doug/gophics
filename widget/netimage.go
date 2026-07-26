package widget

import (
	"fmt"
	"image"
	_ "image/gif"  // decoders
	_ "image/jpeg"
	_ "image/png"
	"net/http"
	"sync"
	"time"

	_ "golang.org/x/image/webp" // webp decode
)

// NetworkImage fetches and decodes an image from a URL off the UI goroutine
// (via Ctx.Post), showing Placeholder while loading and Error on failure.
// Decoded images are cached and de-duplicated by URL across widgets, so
// scrolling back to an image is instant. Supports png, jpeg, gif, webp.
//
// W/H set the drawn size (zero uses the image's pixel size). The M-HN media
// archetype (docs/comparison-and-gaps) needs this. Rounded/circular clipping
// is a follow-up.
type NetworkImage struct {
	URL         string
	W, H        float32
	Placeholder Widget
	Error       Widget
}

func (n NetworkImage) CreateState() State { return &netImgState{} }

type netImgState struct {
	StateBase[NetworkImage]
	post   func(func())
	url    string
	img    image.Image
	failed bool
}

func (s *netImgState) Init(ctx Ctx) {
	s.post = ctx.Post()
	s.start(s.W().URL)
}

func (s *netImgState) Build(Ctx) Widget {
	// URL changed since mount: reload.
	if u := s.W().URL; u != s.url {
		s.start(u)
	}
	switch {
	case s.img != nil:
		return Image{Src: s.img, W: s.W().W, H: s.W().H}
	case s.failed:
		if s.W().Error != nil {
			return s.sized(s.W().Error)
		}
		return s.sized(nil)
	default:
		if s.W().Placeholder != nil {
			return s.sized(s.W().Placeholder)
		}
		return s.sized(nil)
	}
}

// sized wraps content at the requested W/H so the layout doesn't jump when
// the image resolves.
func (s *netImgState) sized(child Widget) Widget {
	return Sized{W: s.W().W, H: s.W().H, Child: child}
}

func (s *netImgState) start(url string) {
	s.url, s.img, s.failed = url, nil, false
	if url == "" {
		return
	}
	go func() {
		res := imgLoad.fetch(url)
		if s.post == nil {
			return
		}
		s.post(func() {
			if s.url != url {
				return // superseded by a newer URL
			}
			s.SetState(func() {
				if res.err != nil {
					s.failed = true
				} else {
					s.img = res.img
				}
			})
		})
	}()
}

// imgLoad is the process-wide image loader: a single-flight cache keyed by
// URL. The first request for a URL fetches; concurrent requests wait; the
// result is cached.
type imgLoader struct {
	mu       sync.Mutex
	cache    map[string]loadResult
	inflight map[string]chan struct{}
	client   http.Client
}

type loadResult struct {
	img image.Image
	err error
}

var imgLoad = &imgLoader{
	cache:    map[string]loadResult{},
	inflight: map[string]chan struct{}{},
	client:   http.Client{Timeout: 20 * time.Second},
}

const imgCacheLimit = 512

func (l *imgLoader) fetch(url string) loadResult {
	l.mu.Lock()
	if r, ok := l.cache[url]; ok {
		l.mu.Unlock()
		return r
	}
	if ch, ok := l.inflight[url]; ok {
		l.mu.Unlock()
		<-ch
		l.mu.Lock()
		r := l.cache[url]
		l.mu.Unlock()
		return r
	}
	ch := make(chan struct{})
	l.inflight[url] = ch
	l.mu.Unlock()

	r := l.do(url)

	l.mu.Lock()
	if len(l.cache) >= imgCacheLimit {
		clear(l.cache)
	}
	l.cache[url] = r
	delete(l.inflight, url)
	close(ch)
	l.mu.Unlock()
	return r
}

func (l *imgLoader) do(url string) loadResult {
	resp, err := l.client.Get(url)
	if err != nil {
		return loadResult{err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return loadResult{err: fmt.Errorf("netimage: %s: %s", url, resp.Status)}
	}
	img, _, err := image.Decode(resp.Body)
	if err != nil {
		return loadResult{err: fmt.Errorf("netimage: decode %s: %w", url, err)}
	}
	return loadResult{img: img}
}
