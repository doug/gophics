package ui

import (
	"context"
	"fmt"
	"image/png"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// fakeAPI serves a deterministic HN corpus instantly.
type fakeAPI struct{ stories, commentsPer int }

func (f fakeAPI) TopStories(context.Context) ([]int, error) {
	ids := make([]int, f.stories)
	for i := range ids {
		ids[i] = 1_000_000 + i
	}
	return ids, nil
}

func (f fakeAPI) Item(_ context.Context, id int) (Item, error) {
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
	h, err := app.NewHeadless(HN{PageSize: 500},
		app.Config{
			Size: geom.Size{W: 480, H: 720}, Background: colBg, Font: goregular.TTF,
			Provide: []any{fakeAPI{stories: 500, commentsPer: 5}},
		}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	deadline := time.Now().Add(5 * time.Second)
	for !st.feed.done && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
	}
	if !st.feed.done {
		t.Fatal("feed never loaded")
	}
	if st.feed.err != nil {
		t.Fatalf("feed load failed: %v", st.feed.err)
	}
	return h, st
}

// slowHarness mounts the feed over an API that answers each item slowly, so
// the load is observable while it is in flight, and returns before it is done.
func slowHarness(t *testing.T, api API, stories int) (*app.Headless, *feedState) {
	t.Helper()
	var st *feedState
	stateHook = func(s *feedState) { st = s }
	defer func() { stateHook = nil }()
	h, err := app.NewHeadless(HN{PageSize: stories},
		app.Config{
			Size: geom.Size{W: 480, H: 720}, Background: colBg, Font: goregular.TTF,
			Provide: []any{api},
		}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if st == nil {
		t.Fatal("feed state not mounted")
	}
	return h, st
}

// awaitFeed renders until the feed's newest load has run to its end. It waits
// on the load generation, not on refreshing: a stale load used to clear that
// flag while the live one was still in flight, and a test waiting on it
// passed with the bug live.
func awaitFeed(t *testing.T, h *app.Headless, st *feedState) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for st.loaded != st.gen && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
	}
	if st.loaded != st.gen {
		t.Fatal("feed never finished loading")
	}
	if st.refreshing {
		t.Fatal("load finished with the refresh spinner still up")
	}
}

// The load streams: the stories at the top are on screen while the ones at
// the bottom are still in flight. The fetch-layer tests prove the prefix is
// reported; this mounts the feed and proves it is painted — the feed used to
// store every partial and draw "loading…" over all of them until the last
// item landed.
func TestFeedPaintsPartialPrefix(t *testing.T) {
	api := &slowAPI{fakeAPI: fakeAPI{stories: 48}, delay: 20 * time.Millisecond}
	h, st := slowHarness(t, api, 48)

	partialShown := false
	deadline := time.Now().Add(10 * time.Second)
	for !st.feed.done && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
		h.Render()
		if !st.feed.done && len(st.feed.items) > 0 && hasLabel(h, "Story number") {
			partialShown = true
		}
	}
	if !st.feed.done {
		t.Fatal("feed never finished loading")
	}
	if !partialShown {
		t.Error("the list was never painted before the last story landed")
	}
	if len(st.feed.items) != 48 {
		t.Errorf("loaded %d stories, want 48", len(st.feed.items))
	}
}

// Pull-to-refresh keeps the list up until the new page is whole. The first
// partial of a refresh used to replace the mounted list with the loading
// placeholder, unmounting the rows the reader was looking at — and the
// refresh spinner with them.
func TestRefreshKeepsTheListUntilTheNewPageLands(t *testing.T) {
	api := &slowAPI{fakeAPI: fakeAPI{stories: 40}, delay: 15 * time.Millisecond}
	h, st := slowHarness(t, api, 40)
	awaitFeed(t, h, st)
	if !hasLabel(h, "Story number") {
		t.Fatal("feed not shown after the first load")
	}

	st.refresh()
	h.Render()
	if !st.refreshing {
		t.Fatal("refresh did not start")
	}
	listGone := false
	deadline := time.Now().Add(10 * time.Second)
	for st.loaded != st.gen && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
		h.Render()
		if !hasLabel(h, "Story number") {
			listGone = true
		}
	}
	if st.loaded != st.gen || st.refreshing {
		t.Fatal("refresh never finished")
	}
	if listGone {
		t.Error("the list was replaced by the loading placeholder during the refresh")
	}
	if len(st.feed.items) != 40 {
		t.Errorf("refreshed to %d stories, want 40", len(st.feed.items))
	}
}

// gatedAPI serves each TopStories call its own page of ids, and once the first
// four stories of a page are in, holds the rest behind a gate the test opens.
// Two loads can then overlap and finish in the order the test chooses, which
// a slow API cannot promise on a loaded machine. Items past the fourth are not
// held until the first four are in because the fetch runs eight at a time:
// eight held items would fill that and starve the four the test waits for.
type gatedAPI struct {
	fakeAPI
	mu    sync.Mutex
	pages int
	gates map[int]chan struct{} // by page
	front map[int]int           // of the first four items, how many returned, by page
	done  map[int]int           // items returned, by page
}

func newGatedAPI() *gatedAPI {
	return &gatedAPI{fakeAPI: fakeAPI{stories: 40},
		gates: map[int]chan struct{}{}, front: map[int]int{}, done: map[int]int{}}
}

func (g *gatedAPI) TopStories(context.Context) ([]int, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.pages++
	g.gates[g.pages] = make(chan struct{})
	ids := make([]int, g.stories)
	for i := range ids {
		ids[i] = g.pages*1_000_000 + i
	}
	return ids, nil
}

func (g *gatedAPI) Item(ctx context.Context, id int) (Item, error) {
	page, i := id/1_000_000, id%1_000_000
	g.mu.Lock()
	gate, held := g.gates[page], g.front[page] == 4
	g.mu.Unlock()
	if i >= 4 && held {
		select {
		case <-gate:
		case <-ctx.Done():
			return Item{}, ctx.Err()
		}
	}
	defer func() {
		g.mu.Lock()
		if i < 4 {
			g.front[page]++
		}
		g.done[page]++
		g.mu.Unlock()
	}()
	return g.fakeAPI.Item(ctx, id)
}

func (g *gatedAPI) open(page int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	close(g.gates[page])
}

func (g *gatedAPI) returned(page int) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.done[page]
}

// A pull while the first page is still streaming in. Two loads are then in
// flight, and the first one's end used to be taken for the refresh's: it
// cleared the spinner, and the refresh's next partial — no longer held back —
// replaced the full list with a shorter one, which is the shrink the refresh
// guard exists to prevent.
func TestRefreshDuringInitialLoadNeverShrinksTheList(t *testing.T) {
	api := newGatedAPI()
	h, st := slowHarness(t, api, 40)

	// The first stories land and the rest wait at the gate.
	deadline := time.Now().Add(10 * time.Second)
	for len(st.feed.items) < 4 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
	}
	if len(st.feed.items) < 4 || st.feed.done {
		t.Fatalf("before the pull: %d stories, done=%v; want a partial page", len(st.feed.items), st.feed.done)
	}
	st.refresh()
	h.Render()
	if !st.refreshing || st.gen != 2 {
		t.Fatalf("refresh did not start: refreshing=%v gen=%d", st.refreshing, st.gen)
	}

	shown, shrank, retracted := len(st.feed.items), 0, false
	observe := func() {
		if n := len(st.feed.items); n < shown {
			shrank++
		}
		shown = len(st.feed.items)
		if !st.refreshing && st.loaded != st.gen {
			retracted = true
		}
	}
	// Let the first load run to its end while the refresh is still held.
	api.open(1)
	for api.returned(1) < 40 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
		observe()
	}
	for range 20 { // and let its result reach the feed, if it is going to
		time.Sleep(time.Millisecond)
		h.Render()
		observe()
	}
	// Now the refresh.
	api.open(2)
	for st.loaded != st.gen && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
		h.Render()
		observe()
	}
	if st.loaded != st.gen {
		t.Fatal("the refresh never finished")
	}
	if shrank > 0 {
		t.Errorf("the list shrank %d times after the pull — a superseded load wrote the feed", shrank)
	}
	if retracted {
		t.Error("the refresh spinner went down before the refresh finished — a superseded load cleared it")
	}
	if len(st.feed.items) != 40 || st.refreshing {
		t.Errorf("after the refresh: %d stories, refreshing=%v; want 40 and idle", len(st.feed.items), st.refreshing)
	}
	if st.feed.items[0].ID != 2_000_000 {
		t.Errorf("the feed shows page %d, want the refreshed one", st.feed.items[0].ID/1_000_000)
	}
}

// failingTopAPI serves one page and then refuses, as a network that drops
// mid-session does.
type failingTopAPI struct {
	fakeAPI
	mu    sync.Mutex
	calls int
}

func (f *failingTopAPI) TopStories(ctx context.Context) ([]int, error) {
	f.mu.Lock()
	f.calls++
	n := f.calls
	f.mu.Unlock()
	if n > 1 {
		return nil, fmt.Errorf("top stories: connection reset")
	}
	return f.fakeAPI.TopStories(ctx)
}

// A refresh that fails leaves the stories it was replacing on screen, and says
// so above them. They are still stories; the error has nothing better to offer
// than them, but a pull that changed nothing and said nothing would look like
// a front page that has not moved.
func TestRefreshFailureKeepsTheList(t *testing.T) {
	api := &failingTopAPI{fakeAPI: fakeAPI{stories: 12}}
	h, st := slowHarness(t, api, 12)
	awaitFeed(t, h, st)

	st.refresh()
	awaitFeed(t, h, st)
	if len(st.feed.items) != 12 || !hasLabel(h, "Story number") {
		t.Errorf("a failed refresh dropped the list: %d items, err=%v", len(st.feed.items), st.feed.err)
	}
	if !hasLabel(h, "connection reset") {
		t.Errorf("the failure is not shown; labels=%v", semLabels(h))
	}
}

// itemFailAPI serves the id list but, from the second page on, no items — a
// network that drops after the cheap request and before the expensive ones.
// The optional first-page failure covers a launch into a dead network.
type itemFailAPI struct {
	fakeAPI
	failFrom int // the TopStories call from which items fail, 1-based
	mu       sync.Mutex
	tops     int
}

func (f *itemFailAPI) TopStories(ctx context.Context) ([]int, error) {
	f.mu.Lock()
	f.tops++
	f.mu.Unlock()
	return f.fakeAPI.TopStories(ctx)
}

func (f *itemFailAPI) Item(ctx context.Context, id int) (Item, error) {
	f.mu.Lock()
	n := f.tops
	f.mu.Unlock()
	if n >= f.failFrom {
		return Item{}, fmt.Errorf("item %d: connection reset", id)
	}
	return f.fakeAPI.Item(ctx, id)
}

// The id list loads and then every item fails. That used to complete as an
// empty page with no error: the refresh replaced twelve stories with nothing
// and no message, and the first load drew a blank page instead of the error.
func TestRefreshWithFailingItemsKeepsTheList(t *testing.T) {
	api := &itemFailAPI{fakeAPI: fakeAPI{stories: 12}, failFrom: 2}
	h, st := slowHarness(t, api, 12)
	awaitFeed(t, h, st)

	st.refresh()
	awaitFeed(t, h, st)
	if len(st.feed.items) != 12 || !hasLabel(h, "Story number") {
		t.Errorf("a refresh whose items failed dropped the list: %d items", len(st.feed.items))
	}
	if !hasLabel(h, "connection reset") {
		t.Errorf("the failure is not shown; labels=%v", semLabels(h))
	}
	if hasLabel(h, "loading") {
		t.Error("the loading placeholder came back over a loaded list")
	}
}

func TestFirstLoadWithFailingItemsShowsTheError(t *testing.T) {
	api := &itemFailAPI{fakeAPI: fakeAPI{stories: 12}, failFrom: 1}
	h, st := slowHarness(t, api, 12)
	awaitFeed(t, h, st)

	if st.feed.err == nil {
		t.Error("a page with no stories in it completed without an error")
	}
	if !hasLabel(h, "connection reset") {
		t.Errorf("the failure is not shown; labels=%v", semLabels(h))
	}
	if hasLabel(h, "loading") {
		t.Error("still showing the loading placeholder after the load failed")
	}
}

func settle(h *app.Headless) {
	for i := 0; i < 300 && h.Step(0.016); i++ {
		h.Render()
	}
	h.Render()
}

func semLabels(h *app.Headless) []string {
	var out []string
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
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
	if len(st.feed.items) != 500 {
		t.Fatalf("stories = %d", len(st.feed.items))
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

// countingAPI records how many items were fetched and can cancel mid-walk.
type countingAPI struct {
	fakeAPI
	mu     sync.Mutex
	items  int
	cancel context.CancelFunc
	after  int
}

func (c *countingAPI) Item(ctx context.Context, id int) (Item, error) {
	c.mu.Lock()
	c.items++
	n := c.items
	c.mu.Unlock()
	if n == c.after && c.cancel != nil {
		c.cancel() // the reader closed the thread at this point
	}
	return c.fakeAPI.Item(ctx, id)
}

func (c *countingAPI) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.items
}

// Walking a comment tree is the expensive part of opening a thread: 80 items,
// one request each. Closing the thread has to stop the walk, or every abandoned
// thread keeps fetching to the end against a page nobody is looking at.
func TestLoadCommentsStopsWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	api := &countingAPI{stories: 20, commentsPer: 20, cancel: cancel, after: 5}
	story, err := api.Item(context.Background(), 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	before := api.count()

	loadComments(ctx, api, story, 80)

	if got := api.count() - before; got > 8 {
		t.Errorf("kept fetching after cancellation: %d items past the cancel at 5, "+
			"want the walk to stop promptly", got)
	}
}

// The guard must not truncate an ordinary load: an uncancelled walk still
// returns the comments it was asked for.
func TestLoadCommentsCompletesWhenNotCancelled(t *testing.T) {
	api := &countingAPI{stories: 20, commentsPer: 20}
	story, err := api.Item(context.Background(), 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	// The fixture's comments have no children, so a story with commentsPer=20
	// yields exactly 20 however high the limit goes.
	got := loadComments(context.Background(), api, story, 40)
	if len(got) != 20 {
		t.Errorf("loaded %d comments, want all 20 the fixture has — the "+
			"cancellation guard is firing on a live context", len(got))
	}
}
