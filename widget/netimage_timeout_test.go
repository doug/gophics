//go:build !js

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
// This can only assert the native half: the js half cannot run here. What
// keeps the two together is that neither writes a duration of its own —
// http.Client.Timeout and setTimeout both read imgFetchTimeout — so this
// catches the native path losing its bound, and there is no second literal
// for the js path to drift to.
func TestImageFetchesAreBounded(t *testing.T) {
	if imgFetchTimeout <= 0 {
		t.Fatalf("imgFetchTimeout is %v: an unbounded fetch holds its concurrency slot forever", imgFetchTimeout)
	}
	if httpClient.Timeout != imgFetchTimeout {
		t.Errorf("native client timeout is %v, want the shared imgFetchTimeout %v",
			httpClient.Timeout, imgFetchTimeout)
	}
}
