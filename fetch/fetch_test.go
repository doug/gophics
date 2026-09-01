//go:build !js

package fetch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The js implementation cannot be exercised here — these cover the net/http
// path and, more usefully, pin the contract both paths have to meet.

func TestGetReturnsTheBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	defer srv.Close()

	b, err := Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"ok":true}` {
		t.Errorf("body = %q", b)
	}
}

// A 404 must be an error, not an empty body. Handing back nil bytes with a nil
// error is how a missing resource gets misreported as a parse failure.
func TestGetTreatsNon2xxAsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := Get(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("a 404 returned no error")
	}
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("error %v is not a *StatusError; a caller cannot tell 404 from a transport failure", err)
	}
	if se.Status != 404 {
		t.Errorf("Status = %d, want 404", se.Status)
	}
}

// Do carries the method, headers and body, and reports the status rather than
// treating it as an error — that judgement belongs to the caller.
func TestDoCarriesTheRequestAndReportsStatus(t *testing.T) {
	var gotMethod, gotHeader string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotHeader = r.Header.Get("X-Token")
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("X-Reply", "pong")
		w.WriteHeader(http.StatusTeapot)
	}))
	defer srv.Close()

	resp, err := Do(context.Background(), Request{
		Method: "POST", URL: srv.URL,
		Header: map[string]string{"X-Token": "abc"},
		Body:   []byte("hello"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotHeader != "abc" || string(gotBody) != "hello" {
		t.Errorf("server saw method=%q header=%q body=%q", gotMethod, gotHeader, gotBody)
	}
	if resp.Status != http.StatusTeapot {
		t.Errorf("Status = %d, want %d — Do reports the status, it does not judge it",
			resp.Status, http.StatusTeapot)
	}
	if resp.Header["X-Reply"] != "pong" {
		t.Errorf("response header = %v", resp.Header)
	}
}

// Cancelling the context has to stop the request, which is the whole reason
// the js path wires an AbortController.
func TestCancellingTheContextStopsTheRequest(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
	}))
	defer srv.Close()
	defer close(release)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	if _, err := Get(ctx, srv.URL); err == nil {
		t.Fatal("a cancelled request returned no error")
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Errorf("took %v — cancellation did not reach the request", d)
	}
}

// Binary must survive. The js path reads an ArrayBuffer rather than text for
// this reason: a string round-trips through UTF-16 and corrupts anything that
// is not text, which is what a caller fetching a PNG would hit.
func TestBinaryBodySurvives(t *testing.T) {
	want := []byte{0x89, 'P', 'N', 'G', 0x00, 0xFF, 0xFE, 0x7F}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(want)
	}))
	defer srv.Close()

	got, err := Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Errorf("body = % x, want % x", got, want)
	}
}
