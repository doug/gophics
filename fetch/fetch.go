// Package fetch makes an HTTP request without linking net/http into a browser
// build.
//
// On every platform but the browser this is net/http, and using it directly
// would be simpler. On js/wasm it is the browser's own fetch(), and the
// difference is 2MB.
//
// The reason is not that net/http fails there — Go's js/wasm net/http already
// calls fetch() under the hood, so the network behaviour is identical. It is
// what the linker cannot remove. Transport.RoundTrip branches on
// jsFetchMissing, a value only known at run time, which keeps the socket path
// reachable and drags crypto/tls, crypto/x509 and http2 in behind it. None of
// it can execute in a browser, where fetch() terminates TLS itself. Measured on
// two programs differing in nothing else: 2.61MB gzipped against 0.55MB.
//
// That is worth hiding rather than asking every app to rediscover. gophics
// already made this trade internally for NetworkImage, and before this package
// existed the hn example carried its own copy — 87 lines of syscall/js in a
// program meant to demonstrate a news reader.
//
// The API deliberately mentions no net/http type. Exposing an *http.Client or
// an http.Header would pull the package back in on js and undo the whole point,
// so requests and responses are described by the small types here.
//
//	body, err := fetch.Get(ctx, "https://example.com/items.json")
//
// Cancellation is the context's: cancelling it aborts the request in flight on
// both paths. There is no built-in timeout — use context.WithTimeout, so the
// deadline is visible where the call is made rather than hidden in here.
package fetch

import (
	"context"
	"fmt"
)

// Request is one HTTP request. Only Method and URL are required; the zero
// Method means GET.
type Request struct {
	Method string            // "GET" when empty
	URL    string            // required
	Header map[string]string // sent as-is; nil for none
	Body   []byte            // nil for none
}

// Response is what came back. Body is read fully before Do returns, because
// the browser's fetch() hands over a promise rather than a stream this package
// can expose identically on both platforms.
type Response struct {
	Status int
	Header map[string]string
	Body   []byte
}

// StatusError is returned by Get for a response outside 2xx. Do does not
// return it: a caller giving a full Request usually wants to inspect the
// status itself.
type StatusError struct {
	URL    string
	Status int
}

func (e *StatusError) Error() string { return fmt.Sprintf("fetch %s: HTTP %d", e.URL, e.Status) }

// Get requests url and returns the response body, treating any status outside
// 2xx as an error (*StatusError).
//
// The common case, and the one worth making a one-liner: most callers want the
// bytes or a reason, and turning a 404 into an empty body with a nil error is
// how a parse failure gets blamed on the parser.
func Get(ctx context.Context, url string) ([]byte, error) {
	resp, err := Do(ctx, Request{URL: url})
	if err != nil {
		return nil, err
	}
	if resp.Status < 200 || resp.Status > 299 {
		return nil, &StatusError{URL: url, Status: resp.Status}
	}
	return resp.Body, nil
}
