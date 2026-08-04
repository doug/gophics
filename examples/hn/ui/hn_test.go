package ui

import (
	"fmt"
	"image/png"
	"os"
	"strings"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// fakeAPI serves a deterministic HN corpus instantly.
type fakeAPI struct{ stories, commentsPer int }

func (f fakeAPI) TopStories() ([]int, error) {
	ids := make([]int, f.stories)
	for i := range ids {
		ids[i] = 1_000_000 + i
	}
	return ids, nil
}

func (f fakeAPI) Item(id int) (Item, error) {
	if id >= 1_000_000 {
		i := id - 1_000_000
		kids := make([]int, f.commentsPer)
		for k := range kids {
			kids[k] = i*100 + k + 1 // comment ids stay below 1_000_000
		}
		return Item{
			ID: id, Type: "story", By: fmt.Sprintf("user%d", i),
			Title: fmt.Sprintf("Story number %d: gophics reaches the front page", i),
			URL:   "https://example.com/post", Score: 100 + i, Descendants: f.commentsPer,
			Kids: kids,
		}, nil
	}
	return Item{
		ID: id, Type: "comment", By: "commenter",
		Text: `<p>This is a <i>comment</i> with a <a href="https://go.dev/blog">link to the Go blog</a> and enough text to wrap across lines.</p>`,
	}, nil
}

func harness(t *testing.T) (*app.Headless, *feedState) {
	t.Helper()
	var st *feedState
	stateHook = func(s *feedState) { st = s }
	defer func() { stateHook = nil }()
	h, err := app.NewHeadless(HN{API: fakeAPI{stories: 500, commentsPer: 5}, PageSize: 500},
		app.Config{Size: geom.Size{W: 480, H: 720}, Background: colBg, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	deadline := time.Now().Add(5 * time.Second)
	for st.loading && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
	}
	if st.loading {
		t.Fatal("feed never loaded")
	}
	return h, st
}

func settle(h *app.Headless) {
	for i := 0; i < 300 && h.Step(0.016); i++ {
		h.Render()
	}
	h.Render()
}

func semLabels(h *app.Headless) []string {
	var out []string
	for _, n := range layout.FlattenSemantics(h.Core.Semantics()) {
		out = append(out, n.Label)
	}
	return out
}

func hasLabel(h *app.Headless, substr string) bool {
	for _, l := range semLabels(h) {
		if strings.Contains(l, substr) {
			return true
		}
	}
	return false
}

func TestFeedLoadsAndScrolls(t *testing.T) {
	h, st := harness(t)
	if len(st.stories) != 500 {
		t.Fatalf("stories = %d", len(st.stories))
	}
	if out := os.Getenv("GOPHICS_RENDER_FEED"); out != "" {
		img := h.Render()
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, img)
	}
	h.Move(geom.Pt{X: 240, Y: 400})
	for range 10 {
		h.Scroll(geom.Pt{Y: -600})
		h.Render()
	}
	if !hasLabel(h, "Story number") {
		t.Fatal("feed rows missing after scroll")
	}
}

func TestNavigateThreadLinksAndBack(t *testing.T) {
	h, _ := harness(t)

	// Tap the first story row; wait for the thread to load and slide in.
	h.Tap(geom.Pt{X: 240, Y: 80})
	settle(h)
	deadline := time.Now().Add(5 * time.Second)
	for !hasLabel(h, "link to the Go blog") && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
		settle(h)
	}
	if !hasLabel(h, "link to the Go blog") {
		t.Fatalf("thread comments not shown; labels=%v", semLabels(h)[:min(8, len(semLabels(h)))])
	}

	// Tap around the first comment area until the link opens.
	opened := false
scan:
	for y := float32(120); y < 500; y += 8 {
		for x := float32(30); x < 460; x += 24 {
			h.Tap(geom.Pt{X: x, Y: y})
			if len(h.OpenedURLs) > 0 {
				opened = true
				break scan
			}
		}
	}
	if !opened || h.OpenedURLs[0] != "https://go.dev/blog" {
		t.Fatalf("link tap did not open URL: %v", h.OpenedURLs)
	}

	if out := os.Getenv("GOPHICS_RENDER_OUT"); out != "" {
		img := h.Render()
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, img)
	}

	// Back returns to the feed.
	h.Tap(geom.Pt{X: 30, Y: 20})
	settle(h)
	if hasLabel(h, "link to the Go blog") {
		t.Fatal("back did not leave the thread")
	}
	if !hasLabel(h, "Story number") {
		t.Fatal("feed not restored after back")
	}
}

func TestParseSpans(t *testing.T) {
	spans := parseSpans(`<p>a &amp; b</p><p><i>it</i> <a href="https://x.y">z</a></p>`, commentStyle)
	var full string
	var link string
	for _, sp := range spans {
		full += sp.Text
		if sp.Link != "" {
			link = sp.Link
		}
	}
	if !strings.Contains(full, "a & b") || !strings.Contains(full, "it z") {
		t.Fatalf("spans text = %q", full)
	}
	if link != "https://x.y" {
		t.Fatalf("link = %q", link)
	}
	if domain("https://www.example.com/a/b") != "example.com" {
		t.Fatal("domain extraction")
	}
}
