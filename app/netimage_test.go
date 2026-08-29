package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

func pngServer(t *testing.T, hits *int32) *httptest.Server {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for y := range 16 {
		for x := range 16 {
			img.Set(x, y, color.RGBA{200, 200, 200, 255})
		}
	}
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	body := buf.Bytes()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(hits, 1)
		time.Sleep(10 * time.Millisecond)
		w.Header().Set("Content-Type", "image/png")
		w.Write(body)
	}))
}

type imgApp struct {
	url  string
	hook func(*imgAppState)
}

func (a imgApp) CreateState() widget.State { s := &imgAppState{url: a.url}; s.hook = a.hook; return s }

type imgAppState struct {
	widget.StateBase[imgApp]
	url  string
	hook func(*imgAppState)
}

func (s *imgAppState) Init(widget.Ctx) { s.hook(s) }

func (s *imgAppState) Build(widget.Ctx) widget.Widget {
	return widget.Fill{Color: paint.RGB(0, 0, 0), Child: widget.Center(
		widget.NetworkImage{
			URL: s.url, W: 64, H: 64,
			Placeholder: widget.Fill{Color: paint.RGB(0, 0, 1)}, // blue=loading
			Error:       widget.Fill{Color: paint.RGB(1, 0, 0)}, // red=failed
		},
	)}
}

func TestNetworkImageLoadsAndCaches(t *testing.T) {
	var hits int32
	srv := pngServer(t, &hits)
	defer srv.Close()

	var st *imgAppState
	h, err := NewHeadless(imgApp{url: srv.URL + "/a.png", hook: func(s *imgAppState) { st = s }},
		Config{Size: geom.Size{W: 120, H: 120}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = st
	img := h.Render()
	// Placeholder first: center pixel is grey, not the image's 200.
	if r, _, _, _ := img.At(60, 60).RGBA(); r>>8 > 120 {
		t.Fatalf("expected grey placeholder first, got r=%d", r>>8)
	}
	// Wait for the async load.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		h.Render()
		if r, _, _, _ := h.Render().At(60, 60).RGBA(); r>>8 > 150 {
			break
		}
	}
	if r, _, _, _ := h.Render().At(60, 60).RGBA(); r>>8 < 150 {
		t.Fatal("image did not resolve (center still not bright)")
	}
	if hits != 1 {
		t.Fatalf("server hits = %d, want 1", hits)
	}

	// A second widget for the same URL must hit the cache, not the network.
	h2, _ := NewHeadless(imgApp{url: srv.URL + "/a.png", hook: func(*imgAppState) {}},
		Config{Size: geom.Size{W: 120, H: 120}, Font: goregular.TTF}, 1)
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
		if r, _, _, _ := h2.Render().At(60, 60).RGBA(); r>>8 > 150 {
			break
		}
	}
	if hits != 1 {
		t.Fatalf("cache miss: server hits = %d, want 1", hits)
	}
}
