package main

import (
	"fmt"
	"image/png"
	"os"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
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
			Title: fmt.Sprintf("Story number %d: gossamer reaches the front page", i),
			URL:   "https://example.com/post", Score: 100 + i, Descendants: f.commentsPer,
			Kids: kids,
		}, nil
	}
	return Item{
		ID: id, Type: "comment", By: "commenter",
		Text: "<p>This is a <i>comment</i> with &quot;entities&quot; and enough text to wrap across multiple lines when rendered in the thread view.</p>",
	}, nil
}

func harness(t *testing.T) (*app.Headless, *hnState) {
	t.Helper()
	var st *hnState
	stateHook = func(s *hnState) { st = s }
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

func TestFeedLoadsAndScrolls(t *testing.T) {
	h, st := harness(t)
	if len(st.stories) != 500 {
		t.Fatalf("stories = %d", len(st.stories))
	}
	if out := os.Getenv("GOSSAMER_RENDER_FEED"); out != "" {
		img := h.Render()
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, img)
	}
	// Fling down the feed and make sure it keeps rendering.
	h.Move(geom.Pt{X: 240, Y: 400})
	for range 10 {
		h.Scroll(geom.Pt{Y: -600})
		h.Render()
	}
	if h.Core.LastDamage.IsEmpty() && !h.Core.Skipped {
		t.Fatal("scrolling produced no damage")
	}
}

func TestTapOpensThreadAndBack(t *testing.T) {
	h, st := harness(t)
	// First story row sits below the header (~40px): tap it.
	h.Tap(geom.Pt{X: 240, Y: 80})
	deadline := time.Now().Add(5 * time.Second)
	for (st.open == nil || st.loadingC) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
	}
	if st.open == nil {
		t.Fatal("tap did not open a story")
	}
	if len(st.comments) != 5 {
		t.Fatalf("comments = %d, want 5", len(st.comments))
	}
	if got := plainText(st.comments[0].Text); got == "" || got[0] != 'T' {
		t.Fatalf("comment text not cleaned: %q", got)
	}

	// Back returns to the feed.
	h.Render()
	h.Tap(geom.Pt{X: 30, Y: 20})
	h.Render()
	if st.open != nil {
		t.Fatal("back did not return to feed")
	}

	if out := os.Getenv("GOSSAMER_RENDER_OUT"); out != "" {
		h.Tap(geom.Pt{X: 240, Y: 80})
		for st.loadingC {
			time.Sleep(time.Millisecond)
			h.Render()
		}
		img := h.Render()
		f, err := os.Create(out)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPlainText(t *testing.T) {
	got := plainText("<p>a &amp; b</p><a href=\"x\">link</a>")
	if got != "a & b\n\nlink" && got != "a & blink" {
		// <p> prefix becomes a break; leading one is trimmed.
		t.Fatalf("plainText = %q", got)
	}
	if domain("https://www.example.com/a/b") != "example.com" {
		t.Fatal("domain extraction")
	}
}
