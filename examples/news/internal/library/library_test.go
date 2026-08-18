package library

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/doug/gophics/examples/news/internal/store"
)

// useTempDir points the library at a scratch directory for the duration of one
// test, so nothing touches the developer's real reading history.
func useTempDir(t *testing.T) {
	t.Helper()
	dirMu.Lock()
	old := dataDir
	dataDir = t.TempDir()
	dirMu.Unlock()
	t.Cleanup(func() {
		dirMu.Lock()
		dataDir = old
		dirMu.Unlock()
	})
}

func TestFirstRunSeedsSubscriptions(t *testing.T) {
	useTempDir(t)

	s := LoadSubscriptions()
	if len(s.Feeds) == 0 {
		t.Fatal("a new install must start with feeds, or the first screen is empty")
	}
	cats := map[string]bool{}
	for _, f := range s.Feeds {
		cats[f.Category] = true
	}
	if len(cats) < 5 {
		t.Errorf("starter set covers only %d categories: %v", len(cats), cats)
	}

	// It must survive a restart without reseeding.
	if err := s.Add(s.Feeds[0]); err != nil {
		t.Fatal(err)
	}
	again := LoadSubscriptions()
	if len(again.Feeds) != len(s.Feeds) {
		t.Errorf("reload changed the list: %d -> %d", len(s.Feeds), len(again.Feeds))
	}
}

func TestSubscriptionEditing(t *testing.T) {
	useTempDir(t)
	s := LoadSubscriptions()
	n := len(s.Feeds)

	c := Candidate{Title: "Example Blog", URL: "https://example.com/feed.xml"}
	f := c.FeedFor("tech")
	if err := s.Add(f); err != nil {
		t.Fatal(err)
	}
	if len(s.Feeds) != n+1 {
		t.Fatalf("add did not take: %d", len(s.Feeds))
	}
	if !s.Has(f.ID) {
		t.Fatal("added feed is not reported as subscribed")
	}

	if err := s.SetEnabled(f.ID, false); err != nil {
		t.Fatal(err)
	}
	for _, e := range s.Enabled() {
		if e.ID == f.ID {
			t.Fatal("a disabled feed must not be polled")
		}
	}
	if !s.Has(f.ID) {
		t.Fatal("disabling must not unsubscribe")
	}

	if err := s.Remove(f.ID); err != nil {
		t.Fatal(err)
	}
	if s.Has(f.ID) || len(s.Feeds) != n {
		t.Fatal("remove did not take")
	}
}

func TestAddRejectsAFeedWithNoURL(t *testing.T) {
	useTempDir(t)
	s := LoadSubscriptions()
	if err := s.Add(Candidate{Title: "Nothing"}.FeedFor("tech")); err == nil {
		t.Fatal("expected an error for a feed with no URL")
	}
}

const sampleFeed = `<?xml version="1.0"?>
<rss version="2.0"><channel>
  <title>Example Blog</title>
  <link>https://example.com</link>
  <item>
    <title>How indexes actually work</title>
    <link>https://example.com/indexes</link>
    <guid>https://example.com/indexes</guid>
    <pubDate>Mon, 17 Aug 2026 09:00:00 GMT</pubDate>
    <description>A short teaser about indexes.</description>
  </item>
  <item>
    <title>Open thread</title>
    <link>https://example.com/open</link>
    <guid>https://example.com/open</guid>
    <pubDate>Mon, 17 Aug 2026 10:00:00 GMT</pubDate>
    <description>Chat away.</description>
  </item>
</channel></rss>`

const articlePage = `<html><head><title>How indexes actually work</title></head><body>
<article><h1>How indexes actually work</h1>
<p>` + longPara + `</p><p>` + longPara + `</p>
<img src="/pic.png"/>
</article></body></html>`

const longPara = `A B-tree keeps its keys in sorted order and its leaves at a uniform depth, which is what makes a lookup cost a logarithm rather than a scan. The interesting part is not the shape but the fan-out: a node sized to a disk page holds hundreds of keys, so even a very large table is three or four reads deep. This is why an index that fits in memory changes the performance of a query by orders of magnitude and why one that does not still helps.`

func TestDiscoverFindsADeclaredFeed(t *testing.T) {
	useTempDir(t)
	var mux http.ServeMux
	srv := httptest.NewServer(&mux)
	defer srv.Close()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprintf(w, `<html><head>
			<link rel="alternate" type="application/rss+xml" title="Posts" href="%s/feed.xml">
			</head><body>hello</body></html>`, srv.URL)
	})
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/rss+xml")
		fmt.Fprint(w, sampleFeed)
	})

	got, err := Discover(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one candidate, got %d: %+v", len(got), got)
	}
	if got[0].Title != "Example Blog" {
		t.Errorf("title = %q", got[0].Title)
	}
	if len(got[0].Items) != 2 {
		t.Errorf("expected a preview of both entries, got %d", len(got[0].Items))
	}
	if got[0].Fulltext != "teaser" {
		t.Errorf("a feed of one-line descriptions should classify as teaser, got %q", got[0].Fulltext)
	}
}

// Pasting a feed address directly must work too — it is what happens when
// someone taps an "RSS" button and copies the link.
func TestDiscoverAcceptsAFeedURLDirectly(t *testing.T) {
	useTempDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sampleFeed)
	}))
	defer srv.Close()

	got, err := Discover(context.Background(), srv.URL+"/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Title != "Example Blog" {
		t.Fatalf("got %+v", got)
	}
}

func TestRefreshStoresExtractsAndRanks(t *testing.T) {
	useTempDir(t)

	var mux http.ServeMux
	srv := httptest.NewServer(&mux)
	defer srv.Close()
	mux.HandleFunc("/feed.xml", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, sampleFeed)
	})
	mux.HandleFunc("/indexes", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, articlePage)
	})

	l := Open()
	if l.OpenError() != "" {
		t.Fatal(l.OpenError())
	}
	// Replace the seeded catalog with one feed under our control.
	for _, f := range l.Subs.All() {
		l.Subs.Remove(f.ID)
	}
	c, err := Preview(context.Background(), srv.URL+"/feed.xml")
	if err != nil {
		t.Fatal(err)
	}
	fd := c.FeedFor("tech")
	// Point the article link at the test server: the fixture's links are
	// absolute to example.com, so rewrite by pointing extraction at our host.
	fd.URL = srv.URL + "/feed.xml"
	if err := l.Subs.Add(fd); err != nil {
		t.Fatal(err)
	}

	res := l.Refresh(context.Background(), nil)
	if res.Failed != 0 {
		t.Fatalf("refresh reported failures: %v", res.Errors)
	}
	// "Open thread" is a recurring non-article and must be filtered out by the
	// catalog's skip patterns.
	if res.NewItems != 1 {
		t.Fatalf("expected 1 stored item (the other is an open thread), got %d", res.NewItems)
	}

	q := l.Queue(QueueOptions{})
	if len(q) != 1 {
		t.Fatalf("queue has %d items, want 1", len(q))
	}
	if q[0].Item.Title != "How indexes actually work" {
		t.Errorf("wrong item surfaced: %q", q[0].Item.Title)
	}
	if q[0].Score <= 0 || q[0].Score >= 1 {
		t.Errorf("score %v is not a probability", q[0].Score)
	}

	// A second refresh must not duplicate anything.
	res2 := l.Refresh(context.Background(), nil)
	if res2.NewItems != 0 {
		t.Errorf("re-polling stored %d duplicates", res2.NewItems)
	}
}

func TestMarkReadRemovesFromQueueAndTeachesTheModel(t *testing.T) {
	useTempDir(t)
	l := Open()
	for _, f := range l.Subs.All() {
		l.Subs.Remove(f.ID)
	}
	l.Subs.Add(Candidate{Title: "T", URL: "https://t.example/feed"}.FeedFor("tech"))

	it := storeItem(t, l, "t-example", "A headline about caching")
	if got := len(l.Queue(QueueOptions{})); got != 1 {
		t.Fatalf("queue has %d, want 1", got)
	}
	before := l.Rank.Trained()
	l.MarkRead(it, true)
	if l.Rank.Trained() <= before {
		t.Error("reading an article taught the model nothing")
	}
	if got := len(l.Queue(QueueOptions{})); got != 0 {
		t.Errorf("a read article is still in the queue (%d)", got)
	}
}

func TestCookiesRoundTripAndReport(t *testing.T) {
	useTempDir(t)

	if err := SaveCookies("economist.com", "Cookie: session=abc123; other=xyz"); err != nil {
		t.Fatal(err)
	}
	st := Cookies("www.economist.com") // the www must not matter
	if !st.Present || st.Count != 2 {
		t.Fatalf("status = %+v", st)
	}
	if !st.Healthy() {
		t.Error("freshly saved session-cookies should be healthy")
	}
	if err := ClearCookies("economist.com"); err != nil {
		t.Fatal(err)
	}
	if Cookies("economist.com").Present {
		t.Error("cookies survived being cleared")
	}
	if err := SaveCookies("economist.com", "   "); err == nil {
		t.Error("expected empty cookies to be rejected")
	}
}

func TestImageURLsFromArticleBody(t *testing.T) {
	got := ImageURLs(`<p>text</p><figure><img src="https://cdn.example/a.jpg" alt="x"/></figure>
		<p><img src="https://cdn.example/b.png"></p>`)
	want := []string{"https://cdn.example/a.jpg", "https://cdn.example/b.png"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
	if ImageURLs("<p>no pictures here</p>") != nil {
		t.Error("expected nil for a body with no images")
	}
}

func TestFeedDomain(t *testing.T) {
	for in, want := range map[string]string{
		"https://www.economist.com/latest/rss.xml": "economist.com",
		"http://example.com:8080/feed":             "example.com",
		"https://sub.example.co.uk/a/b?c=d":        "sub.example.co.uk",
	} {
		if got := FeedDomain(in); got != want {
			t.Errorf("FeedDomain(%q) = %q, want %q", in, got, want)
		}
	}
}

// storeItem puts one article straight into the store, for tests that care about
// what happens after fetching rather than about fetching.
func storeItem(t *testing.T, l *Library, feedID, title string) *store.Item {
	t.Helper()
	it := &store.Item{
		ID:        feedID + ":" + title,
		FeedID:    feedID,
		FeedName:  feedID,
		Category:  "tech",
		Title:     title,
		Published: time.Now().Add(-time.Hour),
		Fetched:   time.Now(),
		WordCount: 900,
	}
	if err := l.Store.Put(it); err != nil {
		t.Fatal(err)
	}
	return it
}

// Several blogs ship their entire archive in the feed, so a fresh install can
// download hundreds of articles and have almost none of them inside the recent
// window. Showing an empty screen in that situation looks like a broken app.
func TestQueueWidensWhenTheRecentWindowIsThin(t *testing.T) {
	useTempDir(t)
	l := Open()
	for _, f := range l.Subs.All() {
		l.Subs.Remove(f.ID)
	}
	l.Subs.Add(Candidate{Title: "Archive", URL: "https://archive.example/feed"}.FeedFor("tech"))

	// One recent post and a long tail of old ones, which is what an archive
	// feed actually looks like.
	mustPut(t, l, "recent", time.Now().Add(-2*time.Hour))
	for i := range 30 {
		mustPut(t, l, fmt.Sprintf("old-%d", i), time.Now().AddDate(0, 0, -60-i))
	}

	q := l.Queue(QueueOptions{})
	if len(q) < thinQueue {
		t.Fatalf("queue widened to only %d articles; the archive should have filled it", len(q))
	}
	// Fresh material must still lead: widening the window must not bury today.
	if q[0].Item.ID != "archive-example:recent" {
		t.Errorf("the recent article should still rank first, got %q", q[0].Item.ID)
	}

	// With plenty inside the window, the window must be respected.
	for i := range 20 {
		mustPut(t, l, fmt.Sprintf("fresh-%d", i), time.Now().Add(-time.Duration(i)*time.Hour))
	}
	q2 := l.Queue(QueueOptions{})
	for _, s := range q2 {
		if s.Item.Published.Before(time.Now().AddDate(0, 0, -30)) {
			t.Errorf("old article %q leaked into a full queue", s.Item.ID)
			break
		}
	}
}

func mustPut(t *testing.T, l *Library, id string, published time.Time) {
	t.Helper()
	it := &store.Item{
		ID: "archive-example:" + id, FeedID: "archive-example", FeedName: "Archive",
		Category: "tech", Title: "Post " + id, Published: published,
		Fetched: time.Now(), WordCount: 800,
	}
	if err := l.Store.Put(it); err != nil {
		t.Fatal(err)
	}
}

// The queue's row builder runs on every frame, so an impression must be counted
// once per run of the app, not once per call. Counting each call pushed
// articles past the skip threshold while the reader was still looking at them.
func TestImpressionsAreCountedOncePerRun(t *testing.T) {
	useTempDir(t)
	l := Open()
	for _, f := range l.Subs.All() {
		l.Subs.Remove(f.ID)
	}
	l.Subs.Add(Candidate{Title: "T", URL: "https://t.example/feed"}.FeedFor("tech"))
	mustPut(t, l, "one", time.Now().Add(-time.Hour))

	it, ok := l.Item("archive-example:one")
	if !ok {
		t.Fatal("stored article not found")
	}
	before := l.Rank.Trained()
	// What a few seconds of rebuilding the list looks like.
	for range 200 {
		l.Impression(it)
	}
	if after := l.Rank.Trained(); after != before {
		t.Errorf("200 rebuilds produced %v of evidence; one run must not add any", after-before)
	}

	// A second run of the app is a genuinely separate occasion. Saving is
	// deferred so a scroll does not write a file per row, so each run flushes
	// the way a host does when the app goes to the background.
	l.FlushRank()
	l2 := Open()
	l2.Impression(it)
	l2.FlushRank()
	l3 := Open()
	l3.Impression(it)
	l3.FlushRank()
	l4 := Open()
	l4.Impression(it)
	l4.FlushRank()
	if l4.Rank.Trained() <= before {
		t.Error("three separate runs without opening it should count as a skip")
	}
}

// A teaser is cut to a byte budget, so the cut has to respect rune boundaries.
// Scripts without inter-word spaces never find a space to back up to, and the
// summary was stored with a broken byte on the end.
func TestTruncateNeverSplitsARune(t *testing.T) {
	cases := []string{
		strings.Repeat("日本語のニュース記事", 20),        // no spaces anywhere
		strings.Repeat("Ελληνικά κείμενα ", 20), // multi-byte with spaces
		strings.Repeat("a", 500),                // plain ascii
		"短い",                                    // shorter than the budget
	}
	for _, in := range cases {
		got := truncate(in, 400)
		if !utf8.ValidString(got) {
			t.Errorf("truncate produced invalid UTF-8 for %.20q…: %q", in, got)
		}
		if strings.ContainsRune(got, utf8.RuneError) {
			t.Errorf("truncate left a replacement character for %.20q…", in)
		}
		if len(got) > 420 {
			t.Errorf("truncate returned %d bytes, well past the 400 budget", len(got))
		}
	}
}
