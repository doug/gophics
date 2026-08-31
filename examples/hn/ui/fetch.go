package ui

import (
	"context"
	"sync"
)

// fetchConcurrency bounds in-flight item requests.
//
// The HN API has no batch endpoint — a story list of 30 is 31 requests and a
// comment thread of 80 is 80, and that is inherent. What is not inherent is
// doing them one at a time: serially, opening a story took 80 round trips
// before it drew anything, which is where the multi-second stall came from.
// Eight is well inside what Firebase serves without complaint, and turns those
// 80 trips into ten.
const fetchConcurrency = 8

// fetchItems resolves ids concurrently and returns the ones that loaded, in
// the order they were asked for — the feed is ranked, so arrival order is not
// an acceptable substitute.
//
// onPrefix, when non-nil, is called each time the finished run from the front
// grows, with the items resolved so far. That is what lets the UI paint the
// first stories while the tail is still in flight. Reporting on *any*
// completion instead would let item 20 appear above item 3 and then jump when
// 3 landed; a prefix only grows, so the list only ever appends.
func fetchItems(ctx context.Context, api API, ids []int, onPrefix func([]Item)) []Item {
	var (
		mu       sync.Mutex
		out      = make([]Item, len(ids))
		ok       = make([]bool, len(ids)) // loaded successfully
		resolved = make([]bool, len(ids)) // request finished, either way
		sent     int                      // length of the prefix already reported
	)

	// collect must be called with mu held.
	collect := func(n int) []Item {
		res := make([]Item, 0, n)
		for i := 0; i < n; i++ {
			if ok[i] {
				res = append(res, out[i])
			}
		}
		return res
	}

	sem := make(chan struct{}, fetchConcurrency)
	var wg sync.WaitGroup
	for i, id := range ids {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(i, id int) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			// Checked again after acquiring: a request that waited behind the
			// semaphore may be for a page the reader has since left.
			if ctx.Err() != nil {
				return
			}
			it, err := api.Item(ctx, id)

			mu.Lock()
			defer mu.Unlock()
			resolved[i] = true
			if err == nil {
				out[i], ok[i] = it, true
			}
			// A failed item still advances the prefix — otherwise one dead id
			// holds every story behind it off the screen.
			grew := false
			for sent < len(ids) && resolved[sent] {
				sent++
				grew = true
			}
			if grew && onPrefix != nil {
				onPrefix(collect(sent))
			}
		}(i, id)
	}
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	return collect(len(ids))
}
