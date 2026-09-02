//go:build !js

package fetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A response over the cap is an error, not a truncation. A truncated body
// would go on to fail in whatever parses it, far from the cause.
func TestResponseOverMaxBytesFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(make([]byte, 2048))
	}))
	defer srv.Close()

	_, err := Do(context.Background(), Request{URL: srv.URL, MaxBytes: 1024})
	if err == nil {
		t.Fatal("a response over MaxBytes was accepted")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Errorf("error is %v, want it to name the size cap", err)
	}
}

// Exactly at the cap is fine — the limit is a bound, not a strict inequality,
// and an off-by-one here would reject valid responses at round sizes.
func TestResponseAtMaxBytesSucceeds(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write(make([]byte, 1024))
	}))
	defer srv.Close()

	resp, err := Do(context.Background(), Request{URL: srv.URL, MaxBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Body) != 1024 {
		t.Errorf("body is %d bytes, want 1024", len(resp.Body))
	}
}
