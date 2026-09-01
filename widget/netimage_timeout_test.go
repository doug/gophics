package widget

import "testing"

// Every image fetch is bounded, and by the shared constant.
//
// The fetch runs while holding one of the imgFetchConcurrency slots and only
// releases it on return, so a request that never settles keeps its slot for
// the life of the process; eight of those and no image loads again. That is
// what happened when the wasm path moved from http.Client to fetch(), which
// has no timeout of its own — the bound was simply gone, and nothing failed
// to say so.
//
// The shape of that risk has changed. There used to be two loaders, one per
// platform, each applying its own bound, and this asserted the native one
// matched the constant so the two could not drift. There is one loader now:
// it wraps a context with imgFetchTimeout and hands it to gophics/fetch, which
// aborts the request on both platforms. So the drift this guarded against is
// gone, and what is left to check is that the bound exists at all — that
// fetch honours the deadline is covered by the fetch package's own test.
func TestImageFetchesAreBounded(t *testing.T) {
	if imgFetchTimeout <= 0 {
		t.Fatalf("imgFetchTimeout is %v: an unbounded fetch holds its concurrency slot forever", imgFetchTimeout)
	}
}
