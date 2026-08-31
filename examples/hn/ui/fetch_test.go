package ui

import (
	"context"
	"sync"
	"testing"
	"time"
)

// slowAPI blocks each Item call long enough that a serial loader is obvious in
// wall-clock, and records how many calls were ever in flight at once.
type slowAPI struct {
	fakeAPI
	delay time.Duration

	mu       sync.Mutex
	inFlight int
	peak     int
	calls    int
}

func (s *slowAPI) Item(ctx context.Context, id int) (Item, error) {
	s.mu.Lock()
	s.inFlight++
	s.calls++
	if s.inFlight > s.peak {
		s.peak = s.inFlight
	}
	s.mu.Unlock()

	select {
	case <-time.After(s.delay):
	case <-ctx.Done():
	}

	s.mu.Lock()
	s.inFlight--
	s.mu.Unlock()
	return s.fakeAPI.Item(ctx, id)
}

func (s *slowAPI) stats() (peak, calls int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.peak, s.calls
}

// The whole point of the rewrite: items are fetched concurrently. Serially,
// 24 items at 20ms each is half a second before anything is drawn; the failure
// this guards is a future edit quietly restoring the one-at-a-time loop.
func TestFetchItemsRunsConcurrently(t *testing.T) {
	api := &slowAPI{fakeAPI: fakeAPI{stories: 100, commentsPer: 0}, delay: 20 * time.Millisecond}
	ids := make([]int, 24)
	for i := range ids {
		ids[i] = 1_000_000 + i
	}

	start := time.Now()
	got := fetchItems(context.Background(), api, ids, nil)
	elapsed := time.Since(start)

	if len(got) != len(ids) {
		t.Fatalf("loaded %d items, want %d", len(got), len(ids))
	}
	peak, _ := api.stats()
	if peak < 2 {
		t.Errorf("peak in-flight requests = %d — the fetch is still serial", peak)
	}
	if peak > fetchConcurrency {
		t.Errorf("peak in-flight = %d, exceeds the %d cap", peak, fetchConcurrency)
	}
	if serial := time.Duration(len(ids)) * api.delay; elapsed > serial/2 {
		t.Errorf("took %v; serial would be ~%v — concurrency is not helping", elapsed, serial)
	}
}

// Order is not arrival order. The feed is ranked, so a slow item must hold its
// place rather than letting the ones behind it overtake.
func TestFetchItemsPreservesRequestOrder(t *testing.T) {
	api := &slowAPI{fakeAPI: fakeAPI{stories: 100, commentsPer: 0}, delay: time.Millisecond}
	ids := []int{1_000_005, 1_000_001, 1_000_009, 1_000_003}
	got := fetchItems(context.Background(), api, ids, nil)
	if len(got) != len(ids) {
		t.Fatalf("loaded %d, want %d", len(got), len(ids))
	}
	for i, want := range ids {
		if got[i].ID != want {
			t.Fatalf("item %d = id %d, want %d — results came back in arrival order", i, got[i].ID, want)
		}
	}
}

// onPrefix exists so the list paints before the tail lands. It must only ever
// grow, and only ever report a completed run from the front.
func TestFetchItemsReportsGrowingPrefix(t *testing.T) {
	api := &slowAPI{fakeAPI: fakeAPI{stories: 100, commentsPer: 0}, delay: 2 * time.Millisecond}
	ids := make([]int, 16)
	for i := range ids {
		ids[i] = 1_000_000 + i
	}

	var mu sync.Mutex
	var lens []int
	fetchItems(context.Background(), api, ids, func(partial []Item) {
		mu.Lock()
		defer mu.Unlock()
		lens = append(lens, len(partial))
		// Whatever is reported must be a correct prefix of the request order.
		for i, it := range partial {
			if it.ID != ids[i] {
				t.Errorf("prefix[%d] = %d, want %d", i, it.ID, ids[i])
			}
		}
	})

	mu.Lock()
	defer mu.Unlock()
	if len(lens) == 0 {
		t.Fatal("onPrefix never fired — nothing would paint until the whole feed landed")
	}
	for i := 1; i < len(lens); i++ {
		if lens[i] <= lens[i-1] {
			t.Fatalf("prefix shrank or repeated: %v", lens)
		}
	}
	if lens[len(lens)-1] != len(ids) {
		t.Errorf("final prefix %d, want %d", lens[len(lens)-1], len(ids))
	}
}

// Opening a thread should show the top-level comments after one round trip,
// not after the deepest reply in the tree. The fixture nests, so a depth-first
// fetch would report nothing until it had walked a whole chain.
func TestStreamCommentsReportsTopLevelFirst(t *testing.T) {
	api := &slowAPI{fakeAPI: fakeAPI{stories: 20, commentsPer: 4}, delay: 3 * time.Millisecond}
	story, err := api.fakeAPI.Item(context.Background(), 1_000_000)
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var first []Comment
	final := streamComments(context.Background(), api, story, 80, func(partial []Comment) {
		mu.Lock()
		defer mu.Unlock()
		if first == nil && len(partial) > 0 {
			first = append([]Comment(nil), partial...)
		}
	})

	mu.Lock()
	defer mu.Unlock()
	if len(first) == 0 {
		t.Fatal("no progress was reported — the thread would spin until fully loaded")
	}
	if first[0].Depth != 0 {
		t.Errorf("first reported comment is at depth %d, want a top-level one", first[0].Depth)
	}
	if len(final) < len(first) {
		t.Errorf("final tree (%d) smaller than the first report (%d)", len(final), len(first))
	}
}
