package widget

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// testPNG returns the bytes of a small valid PNG.
func testPNG(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newTestLoader() *imgLoader {
	return &imgLoader{
		cache:    map[string]*imgCacheEntry{},
		inflight: map[string]chan struct{}{},
		sem:      make(chan struct{}, imgFetchConcurrency),
	}
}

// The fetch semaphore bounds concurrent fetch+decode work: with far more
// distinct URLs in flight than the cap, the server must never see more than
// imgFetchConcurrency simultaneous requests.
func TestImgLoaderFetchConcurrencyBounded(t *testing.T) {
	pngBytes := testPNG(t)

	var mu sync.Mutex
	cur, peak := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		cur++
		if cur > peak {
			peak = cur
		}
		mu.Unlock()
		time.Sleep(30 * time.Millisecond) // hold the slot so overlap is observable
		mu.Lock()
		cur--
		mu.Unlock()
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	l := newTestLoader()
	const n = 32
	var wg sync.WaitGroup
	results := make([]loadResult, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = l.fetch(fmt.Sprintf("%s/img-%d.png", srv.URL, i))
		}(i)
	}
	wg.Wait()

	if peak > imgFetchConcurrency {
		t.Fatalf("peak concurrent fetches = %d, want <= %d", peak, imgFetchConcurrency)
	}
	if peak < 2 {
		t.Fatalf("peak concurrent fetches = %d — fetches did not overlap at all", peak)
	}
	for i, r := range results {
		if r.err != nil || r.img == nil {
			t.Fatalf("fetch %d failed: %v", i, r.err)
		}
	}
}

// Single-flight de-duplication: many concurrent fetches of one URL hit the
// server exactly once and all receive the decoded image.
func TestImgLoaderSingleFlight(t *testing.T) {
	pngBytes := testPNG(t)

	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		time.Sleep(30 * time.Millisecond) // widen the window for racers to pile up
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	l := newTestLoader()
	url := srv.URL + "/one.png"
	const n = 16
	var wg sync.WaitGroup
	results := make([]loadResult, n)
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = l.fetch(url)
		}(i)
	}
	wg.Wait()

	if hits != 1 {
		t.Fatalf("server hit %d times for one URL, want 1 (single-flight)", hits)
	}
	for i, r := range results {
		if r.err != nil || r.img == nil {
			t.Fatalf("waiter %d got bad result: %v", i, r.err)
		}
	}
}

// At the cache bound, eviction drops the least-recently-touched half — not
// the whole map — and recently-hit entries survive.
func TestImgLoaderEvictsOldHalfKeepsHot(t *testing.T) {
	pngBytes := testPNG(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	l := newTestLoader()
	// Fill the cache to its limit with entries of ascending age.
	for i := range imgCacheLimit {
		l.gen++
		l.cache[fmt.Sprintf("u%d", i)] = &imgCacheEntry{gen: l.gen}
	}
	// Touch the oldest entry: a cache hit must refresh its generation.
	l.fetch("u0")

	// The next miss pushes the cache over the bound and evicts.
	newURL := srv.URL + "/fresh.png"
	if r := l.fetch(newURL); r.err != nil || r.img == nil {
		t.Fatalf("fresh fetch failed: %v", r.err)
	}

	if want := imgCacheLimit/2 + 1; len(l.cache) != want {
		t.Fatalf("cache size after eviction = %d, want %d (half kept + new entry)", len(l.cache), want)
	}
	if _, ok := l.cache["u0"]; !ok {
		t.Fatal("recently-touched entry was evicted — eviction is not LRU-aware")
	}
	if _, ok := l.cache["u1"]; ok {
		t.Fatal("oldest untouched entry survived eviction")
	}
	if _, ok := l.cache[newURL]; !ok {
		t.Fatal("newly fetched entry missing from cache")
	}
	// A follow-up fetch of an evicted URL misses (would refetch), a kept one hits.
	if _, ok := l.cache[fmt.Sprintf("u%d", imgCacheLimit-1)]; !ok {
		t.Fatal("newest prefilled entry was evicted")
	}
}

// The widget path: NetworkImage shows its placeholder, then swaps to the
// decoded image when the fetch result is posted back to the UI goroutine.
func TestNetworkImageLoadsAndSwapsToImage(t *testing.T) {
	pngBytes := testPNG(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	posted := make(chan func(), 16)
	o := newOwner()
	o.Post = func(fn func()) { posted <- fn }
	o.SetRoot(NetworkImage{URL: srv.URL + "/a.png", W: 10, H: 10, Placeholder: Sized{W: 10, H: 10}})
	o.FlushBuilds()

	st := digState[NetworkImage](o.root).(*netImgState)
	if st.img != nil || st.failed {
		t.Fatalf("expected loading state first: img=%v failed=%v", st.img, st.failed)
	}

	select {
	case fn := <-posted:
		fn() // deliver the fetch result on the "UI goroutine"
	case <-time.After(5 * time.Second):
		t.Fatal("fetch result never posted")
	}
	o.FlushBuilds()
	if st.failed || st.img == nil {
		t.Fatalf("image not loaded: failed=%v", st.failed)
	}
	if st.img.Bounds().Dx() != 4 {
		t.Fatalf("decoded bounds = %v, want 4x4", st.img.Bounds())
	}
}

// A result for a URL the widget has moved past must be dropped, and the
// current URL's result must land.
func TestNetworkImageSupersededURLIgnored(t *testing.T) {
	small, large := testPNG(t), func() []byte {
		var buf bytes.Buffer
		_ = png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8)))
		return buf.Bytes()
	}()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		if r.URL.Path == "/large.png" {
			w.Write(large)
			return
		}
		w.Write(small)
	}))
	defer srv.Close()

	posted := make(chan func(), 16)
	o := newOwner()
	o.Post = func(fn func()) { posted <- fn }
	o.SetRoot(NetworkImage{URL: srv.URL + "/small.png"})
	o.FlushBuilds()
	st := digState[NetworkImage](o.root).(*netImgState)

	var stale func()
	select {
	case stale = <-posted: // hold the first URL's result back
	case <-time.After(5 * time.Second):
		t.Fatal("first fetch never posted")
	}

	// Reconcile to a new URL before the first result is delivered.
	o.SetRoot(NetworkImage{URL: srv.URL + "/large.png"})
	o.FlushBuilds()

	stale() // late delivery for the old URL: must be ignored
	o.FlushBuilds()
	if st.img != nil {
		t.Fatal("stale result for a superseded URL was applied")
	}

	select {
	case fn := <-posted:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("second fetch never posted")
	}
	o.FlushBuilds()
	if st.img == nil || st.img.Bounds().Dx() != 8 {
		t.Fatalf("current URL's image not applied: %v", st.img)
	}
}

// Errors are cached like successes (negative caching): a failing URL is not
// re-fetched on every build.
func TestImgLoaderCachesErrors(t *testing.T) {
	var mu sync.Mutex
	hits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits++
		mu.Unlock()
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer srv.Close()

	l := newTestLoader()
	url := srv.URL + "/missing.png"
	r1 := l.fetch(url)
	r2 := l.fetch(url)
	if r1.err == nil || r2.err == nil {
		t.Fatal("404 fetch did not error")
	}
	if hits != 1 {
		t.Fatalf("server hit %d times, want 1 (error result cached)", hits)
	}
}
