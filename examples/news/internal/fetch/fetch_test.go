package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newTestClient returns a client with politeness delays replaced by a recorder,
// so tests neither sleep nor lose coverage of the delay logic.
func newTestClient(slept *[]time.Duration) *Client {
	c := NewClient()
	c.MinHostInterval = 10 * time.Millisecond
	c.sleep = func(ctx context.Context, d time.Duration) error {
		*slept = append(*slept, d)
		return ctx.Err()
	}
	return c
}

func TestConditionalGet(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if r.Header.Get("If-None-Match") == `"v1"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		w.Header().Set("Last-Modified", "Tue, 12 Aug 2026 09:30:00 GMT")
		w.Write([]byte("<rss/>"))
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)

	first, err := c.Do(context.Background(), Request{URL: srv.URL})
	if err != nil {
		t.Fatalf("first fetch: %v", err)
	}
	if first.NotModified {
		t.Error("first fetch should not be a 304")
	}
	if first.ETag != `"v1"` {
		t.Errorf("ETag = %q", first.ETag)
	}
	if string(first.Body) != "<rss/>" {
		t.Errorf("Body = %q", first.Body)
	}

	second, err := c.Do(context.Background(), Request{
		URL: srv.URL, ETag: first.ETag, LastModified: first.LastModified,
	})
	if err != nil {
		t.Fatalf("second fetch: %v", err)
	}
	if !second.NotModified {
		t.Error("second fetch should report NotModified")
	}
	if len(second.Body) != 0 {
		t.Errorf("304 should carry no body, got %q", second.Body)
	}
	if hits != 2 {
		t.Errorf("server saw %d requests, want 2", hits)
	}
}

func TestRetryOn503ThenSucceed(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hits.Add(1) == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)

	resp, err := c.Do(context.Background(), Request{URL: srv.URL})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if string(resp.Body) != "ok" {
		t.Errorf("Body = %q", resp.Body)
	}
	// The Retry-After value must be preferred over computed backoff.
	found := false
	for _, d := range slept {
		if d == 7*time.Second {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a 7s sleep from Retry-After, got %v", slept)
	}
}

func TestNoRetryOnPermanentStatus(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)

	_, err := c.Do(context.Background(), Request{URL: srv.URL})
	if err == nil {
		t.Fatal("want an error for 404")
	}
	if !errors.Is(err, ErrPermanent) {
		t.Errorf("404 should be permanent, got %v", err)
	}
	if hits != 1 {
		t.Errorf("server saw %d requests, want 1 (no retries on 404)", hits)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.Status != 404 {
		t.Errorf("want a StatusError with 404, got %v", err)
	}
}

func TestRetryExhaustionReturnsError(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)
	c.MaxRetries = 2

	if _, err := c.Do(context.Background(), Request{URL: srv.URL}); err == nil {
		t.Fatal("want an error")
	}
	if hits != 3 {
		t.Errorf("server saw %d requests, want 3 (initial + 2 retries)", hits)
	}
}

func TestUserAgentOverride(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("User-Agent")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)
	if _, err := c.Do(context.Background(), Request{URL: srv.URL, UserAgent: "custom/9"}); err != nil {
		t.Fatal(err)
	}
	if ua := <-got; ua != "custom/9" {
		t.Errorf("User-Agent = %q, want custom/9", ua)
	}
}

func TestHostPolitenessDelaysSecondRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)

	ctx := context.Background()
	if _, err := c.Do(ctx, Request{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if len(slept) != 0 {
		t.Errorf("first request to a host should not wait, slept %v", slept)
	}
	if _, err := c.Do(ctx, Request{URL: srv.URL}); err != nil {
		t.Fatal(err)
	}
	if len(slept) == 0 {
		t.Error("second request to the same host should wait for MinHostInterval")
	}
}

func TestContextCancellation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := NewClient()
	c.MinHostInterval = 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Do(ctx, Request{URL: srv.URL}); err == nil {
		t.Fatal("want an error from a cancelled context")
	}
}

func TestBodySizeCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1<<16)
		for range 8 {
			w.Write(buf)
		}
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)
	resp, err := c.Do(context.Background(), Request{URL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Body) > MaxBodyBytes {
		t.Errorf("body of %d bytes exceeds the cap", len(resp.Body))
	}
}

func TestParseRetryAfter(t *testing.T) {
	if d := parseRetryAfter("30"); d != 30*time.Second {
		t.Errorf("delta-seconds: got %v", d)
	}
	if d := parseRetryAfter("99999"); d != 5*time.Minute {
		t.Errorf("absurd delta should clamp to 5m, got %v", d)
	}
	if d := parseRetryAfter("-5"); d != 0 {
		t.Errorf("negative should be 0, got %v", d)
	}
	if d := parseRetryAfter(""); d != 0 {
		t.Errorf("empty should be 0, got %v", d)
	}
	if d := parseRetryAfter("garbage"); d != 0 {
		t.Errorf("garbage should be 0, got %v", d)
	}
	future := time.Now().Add(20 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future); d <= 0 || d > 21*time.Second {
		t.Errorf("http-date: got %v", d)
	}
	past := time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(past); d != 0 {
		t.Errorf("past date should be 0, got %v", d)
	}
}

// Cookies must reach the server, and cookies belonging to other domains must
// not. Both halves matter: the first is what makes a subscription usable, the
// second is what makes it safe to point at a broadly-scoped cookie file.
func TestCookiesAreSentAndScoped(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Cookie")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)

	_, err := c.Do(context.Background(), Request{
		URL: srv.URL,
		Cookies: []*http.Cookie{
			{Name: "session", Value: "wanted"},                           // host-only
			{Name: "elsewhere", Value: "nope", Domain: "example.com"},    // other site
			{Name: "alsoelsewhere", Value: "nope", Domain: "google.com"}, // other site
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	header := <-got
	if !strings.Contains(header, "session=wanted") {
		t.Errorf("the request should carry the host's own cookie, got %q", header)
	}
	if strings.Contains(header, "nope") {
		t.Errorf("cookies for other domains must not be sent, got %q", header)
	}
}

func TestExpiredCookiesAreNotSent(t *testing.T) {
	got := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got <- r.Header.Get("Cookie")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	var slept []time.Duration
	c := newTestClient(&slept)
	_, err := c.Do(context.Background(), Request{
		URL: srv.URL,
		Cookies: []*http.Cookie{
			{Name: "stale", Value: "v", Expires: time.Now().Add(-time.Hour)},
			{Name: "fresh", Value: "v", Expires: time.Now().Add(time.Hour)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	header := <-got
	if strings.Contains(header, "stale") {
		t.Errorf("an expired cookie must not be sent, got %q", header)
	}
	if !strings.Contains(header, "fresh=v") {
		t.Errorf("a live cookie should be sent, got %q", header)
	}
}
