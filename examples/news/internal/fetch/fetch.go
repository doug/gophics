// Package fetch performs polite, conditional HTTP retrieval.
//
// It exists to keep three concerns out of the rest of the program: not
// re-downloading unchanged feeds, not hammering a host, and distinguishing
// "temporarily unavailable" from "gone".
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"sync"
	"time"
)

// DefaultUserAgent identifies the reader honestly. Some hosts (notably
// rachelbythebay.com) rate-limit browser-impersonating agents, and an honest UA
// with contact information is the better citizen anyway.
const DefaultUserAgent = "rss-reader/1.0 (+https://github.com/dougfritz/rss)"

// MaxBodyBytes caps a single response. Feeds are text; anything larger is a
// misconfiguration or a trap.
const MaxBodyBytes = 16 << 20

// Client is a polite HTTP client. It is safe for concurrent use.
type Client struct {
	HTTP      *http.Client
	UserAgent string

	// MinHostInterval is the minimum delay between requests to the same host.
	MinHostInterval time.Duration

	// MaxRetries counts retries after the first attempt.
	MaxRetries int

	// Clock indirection for tests.
	sleep func(context.Context, time.Duration) error

	mu    sync.Mutex
	hosts map[string]*hostState
}

type hostState struct {
	mu   sync.Mutex
	last time.Time
}

// NewClient returns a Client with sensible defaults.
func NewClient() *Client {
	return &Client{
		HTTP: &http.Client{
			Timeout: 45 * time.Second,
			// Cap redirects; some feed URLs bounce through trackers.
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 8 {
					return errors.New("too many redirects")
				}
				return nil
			},
		},
		UserAgent:       DefaultUserAgent,
		MinHostInterval: 1500 * time.Millisecond,
		MaxRetries:      2,
	}
}

// Request describes a conditional fetch.
type Request struct {
	URL string

	// ETag and LastModified come from a previous Response and turn the fetch
	// into a conditional one.
	ETag         string
	LastModified string

	// UserAgent overrides the client default for this request.
	UserAgent string

	// Cookies authenticate the request, so a paid subscription can be used to
	// retrieve article bodies. They are placed in a per-request jar rather than
	// set as a header, which keeps them attached across redirects and scoped to
	// their own domain.
	Cookies []*http.Cookie

	// MinInterval overrides the client's per-host delay. arXiv's API asks for
	// three seconds between requests and answers 429 when pushed harder.
	MinInterval time.Duration
}

// Response is the outcome of a fetch.
type Response struct {
	Status       int
	Body         []byte
	ETag         string
	LastModified string
	ContentType  string
	FinalURL     string

	// NotModified is true when the server answered 304 and Body is empty.
	NotModified bool
}

// ErrPermanent marks a status that will not improve on retry, so callers can
// disable a feed rather than keep asking.
var ErrPermanent = errors.New("permanent failure")

// StatusError reports a non-success HTTP status.
type StatusError struct {
	Status   int
	URL      string
	FinalURL string
}

func (e *StatusError) Error() string {
	if e.FinalURL != "" && e.FinalURL != e.URL {
		return fmt.Sprintf("http %d for %s (redirected to %s)", e.Status, e.URL, e.FinalURL)
	}
	return fmt.Sprintf("http %d for %s", e.Status, e.URL)
}

// Unwrap lets errors.Is(err, ErrPermanent) work for 4xx statuses that will not
// recover: not found, gone, and the auth/forbidden family.
func (e *StatusError) Unwrap() error {
	switch e.Status {
	case http.StatusNotFound, http.StatusGone, http.StatusForbidden,
		http.StatusUnauthorized, http.StatusPaymentRequired:
		return ErrPermanent
	}
	return nil
}

// Do performs the request, retrying transient failures with backoff.
func (c *Client) Do(ctx context.Context, r Request) (*Response, error) {
	for attempt := 0; ; attempt++ {
		if err := c.waitForHost(ctx, r.URL, r.MinInterval); err != nil {
			return nil, err
		}

		resp, retryAfter, err := c.attempt(ctx, r)
		if err == nil {
			return resp, nil
		}

		// Never retry a permanent failure or a cancelled context.
		if errors.Is(err, ErrPermanent) || ctx.Err() != nil {
			return nil, err
		}
		if attempt >= c.MaxRetries {
			return nil, err
		}

		delay := retryAfter
		if delay <= 0 {
			delay = backoff(attempt)
		}
		if err := c.doSleep(ctx, delay); err != nil {
			return nil, err
		}
	}
}

func (c *Client) attempt(ctx context.Context, r Request) (*Response, time.Duration, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.URL, nil)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %v", ErrPermanent, err)
	}

	ua := r.UserAgent
	if ua == "" {
		ua = c.UserAgent
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "application/atom+xml, application/rss+xml, application/xml;q=0.9, text/xml;q=0.9, text/html;q=0.8, */*;q=0.5")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	if r.ETag != "" {
		req.Header.Set("If-None-Match", r.ETag)
	}
	if r.LastModified != "" {
		req.Header.Set("If-Modified-Since", r.LastModified)
	}

	client := c.HTTP
	if len(r.Cookies) > 0 {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, 0, fmt.Errorf("%w: cookie jar: %v", ErrPermanent, err)
		}
		jar.SetCookies(req.URL, r.Cookies)
		withJar := *c.HTTP
		withJar.Jar = jar
		client = &withJar
	}

	httpResp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("get %s: %w", r.URL, err)
	}
	defer httpResp.Body.Close()

	out := &Response{
		Status:       httpResp.StatusCode,
		ETag:         httpResp.Header.Get("ETag"),
		LastModified: httpResp.Header.Get("Last-Modified"),
		ContentType:  httpResp.Header.Get("Content-Type"),
		FinalURL:     httpResp.Request.URL.String(),
	}

	if httpResp.StatusCode == http.StatusNotModified {
		out.NotModified = true
		return out, 0, nil
	}

	if httpResp.StatusCode != http.StatusOK {
		retryAfter := parseRetryAfter(httpResp.Header.Get("Retry-After"))
		// Drain a little so the connection can be reused.
		io.CopyN(io.Discard, httpResp.Body, 4096)
		return nil, retryAfter, &StatusError{Status: httpResp.StatusCode, URL: r.URL, FinalURL: out.FinalURL}
	}

	body, err := io.ReadAll(io.LimitReader(httpResp.Body, MaxBodyBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("read %s: %w", r.URL, err)
	}
	out.Body = body
	return out, 0, nil
}

// waitForHost enforces the per-host delay, preferring an explicit override.
func (c *Client) waitForHost(ctx context.Context, rawURL string, override time.Duration) error {
	interval := c.MinHostInterval
	if override > 0 {
		interval = override
	}
	if interval <= 0 {
		return nil
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return nil
	}

	c.mu.Lock()
	if c.hosts == nil {
		c.hosts = make(map[string]*hostState)
	}
	hs := c.hosts[u.Host]
	if hs == nil {
		hs = &hostState{}
		c.hosts[u.Host] = hs
	}
	c.mu.Unlock()

	// Serialise requests to this host so the interval is actually honoured
	// when many goroutines target the same site.
	hs.mu.Lock()
	defer hs.mu.Unlock()

	if wait := interval - time.Since(hs.last); wait > 0 && !hs.last.IsZero() {
		if err := c.doSleep(ctx, wait); err != nil {
			return err
		}
	}
	hs.last = time.Now()
	return nil
}

func (c *Client) doSleep(ctx context.Context, d time.Duration) error {
	if c.sleep != nil {
		return c.sleep(ctx, d)
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// backoff returns an exponentially growing delay with jitter, so a batch of
// feeds failing at once does not retry in lockstep.
func backoff(attempt int) time.Duration {
	base := min(time.Duration(1<<uint(attempt))*time.Second, 30*time.Second)
	jitter := time.Duration(rand.Int63n(int64(base / 2)))
	return base + jitter
}

// parseRetryAfter understands both the delta-seconds and HTTP-date forms.
func parseRetryAfter(v string) time.Duration {
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		if d := time.Duration(secs) * time.Second; d <= 5*time.Minute {
			return d
		}
		return 5 * time.Minute
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			if d > 5*time.Minute {
				return 5 * time.Minute
			}
			return d
		}
	}
	return 0
}
