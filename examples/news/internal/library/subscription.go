package library

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/cookies"
	"github.com/doug/gophics/examples/news/internal/extract"
	"github.com/doug/gophics/examples/news/internal/feed"
	"github.com/doug/gophics/examples/news/internal/fetch"
)

// This file is how a paid subscription reaches the reader.
//
// The publishers worth paying for are exactly the ones whose feeds carry a
// teaser and nothing else: the entry announces an article and the body is
// behind a login. Sending the subscriber's own session cookie with the article
// fetch is what turns those entries into something readable, and it is the same
// mechanism a browser uses when you click the link yourself.
//
// On a phone the cookies come from a web view the host presents for logging in,
// handed over through the gomobile bind surface. On desktop, where there is no
// such view, they are pasted in from a browser's network panel. Both end in the
// same place: one file per domain, 0600, under the data directory, sent only to
// the host it came from.

// SaveCookies stores a session for one publisher. The value may be a raw
// "Cookie:" request header — which is what a host web view and a browser's
// network panel both produce — or the contents of a Netscape cookies.txt
// export.
func SaveCookies(domain, value string) error {
	domain = FeedDomain(domain)
	if domain == "" {
		return fmt.Errorf("no domain")
	}
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("no cookies to save")
	}
	if err := os.MkdirAll(CookieDir(), 0o700); err != nil {
		return err
	}
	path := CookiePath(domain)
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		return err
	}
	// Reject what cannot be read back now rather than failing silently at the
	// next refresh, when nobody is watching.
	cs, err := cookies.Load(path)
	if err != nil || len(cs) == 0 {
		os.Remove(path)
		if err == nil {
			err = fmt.Errorf("no cookies found in that text")
		}
		return err
	}
	return nil
}

// CookieStatus describes the stored session for a publisher.
type CookieStatus struct {
	Domain  string
	Present bool
	Count   int
	Expired int
	Saved   time.Time
}

// Healthy reports whether the session is worth sending. A few expired cookies
// are normal — they are usually analytics and are never sent — but a file where
// everything has expired means the login needs doing again.
func (s CookieStatus) Healthy() bool {
	return s.Present && s.Count > 0 && s.Expired < s.Count
}

// Cookies reports what is stored for a domain.
func Cookies(domain string) CookieStatus {
	domain = FeedDomain(domain)
	st := CookieStatus{Domain: domain}
	path := CookiePath(domain)
	info, err := os.Stat(path)
	if err != nil {
		return st
	}
	st.Present, st.Saved = true, info.ModTime()
	cs, err := cookies.Load(path)
	if err != nil {
		return st
	}
	st.Count = len(cs)
	st.Expired = len(cookies.Expired(cs, time.Now()))
	return st
}

// ClearCookies forgets a publisher's session.
func ClearCookies(domain string) error {
	err := os.Remove(CookiePath(FeedDomain(domain)))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// VerifyResult is the outcome of testing a subscription.
type VerifyResult struct {
	WithoutWords int
	WithWords    int
	Err          string
}

// Working reports whether authenticating actually got more article.
func (v VerifyResult) Working() bool {
	return v.Err == "" && v.WithWords > v.WithoutWords+50
}

// Summary is a sentence for the settings screen.
func (v VerifyResult) Summary() string {
	switch {
	case v.Err != "":
		return v.Err
	case v.Working():
		return fmt.Sprintf("Working — signing in added %d words (%d → %d).",
			v.WithWords-v.WithoutWords, v.WithoutWords, v.WithWords)
	case v.WithWords == 0:
		return "Could not read the article at all. The cookies may be stale."
	default:
		return fmt.Sprintf("No difference (%d words either way). Either this feed is not paywalled, or the session is not being accepted.",
			v.WithWords)
	}
}

// VerifyCookies fetches one of a feed's articles twice — once anonymously and
// once with the stored session — and compares how much text came back.
//
// This exists because every other way of checking is a guess. Cookie names are
// undocumented and change; expiry dates say nothing about whether the publisher
// still honours the session. Fetching the same article both ways and counting
// the words is the only check that answers the question actually being asked,
// which is "will my subscription work tomorrow morning".
func (l *Library) VerifyCookies(ctx context.Context, fd catalog.Feed) VerifyResult {
	client := fetch.NewClient()

	resp, err := client.Do(ctx, fetch.Request{URL: fd.URL, UserAgent: fd.UserAgent})
	if err != nil {
		return VerifyResult{Err: "could not fetch the feed: " + err.Error()}
	}
	parsed, err := feed.Parse(resp.Body)
	if err != nil || len(parsed.Items) == 0 {
		return VerifyResult{Err: "the feed has no articles to test with"}
	}

	link := ""
	for _, it := range parsed.Items {
		if it.Link != "" {
			link = it.Link
			break
		}
	}
	if link == "" {
		return VerifyResult{Err: "the feed's entries have no links"}
	}

	opts := extract.DefaultOptions()
	words := func(cs []*http.Cookie) int {
		req := fetch.Request{URL: link, MinInterval: fd.MinInterval(), Cookies: cs}
		if fd.ArticleUserAgent != "" {
			req.UserAgent = fd.ArticleUserAgent
		} else if fd.UserAgent != "" {
			req.UserAgent = fd.UserAgent
		}
		r, err := client.Do(ctx, req)
		if err != nil {
			return 0
		}
		art, err := extract.FromHTML(r.Body, r.FinalURL, opts)
		if err != nil {
			return 0
		}
		return art.WordCount
	}

	var res VerifyResult
	res.WithoutWords = words(nil)
	jar := l.cookiesFor(fd)
	if len(jar) == 0 {
		return VerifyResult{Err: "no session is saved for " + FeedDomain(fd.URL)}
	}
	res.WithWords = words(jar)
	return res
}

// PaywalledFeeds are the subscribed feeds whose bodies are gated: the ones that
// ship a teaser and need a session to be worth reading. These are what the
// settings screen offers to sign in to, rather than making someone pick their
// publisher out of a list of everything.
func (l *Library) PaywalledFeeds() []catalog.Feed {
	var out []catalog.Feed
	for _, f := range l.Subs.All() {
		if f.Fulltext == catalog.Teaser && f.ShouldExtract() {
			out = append(out, f)
		}
	}
	return out
}

// LoginURL is where to send the host's web view to sign in. The feed's own site
// is the right destination: publishers put the login on their home page, and
// landing there logged in is the confirmation that it worked.
func LoginURL(fd catalog.Feed) string {
	return "https://" + FeedDomain(fd.URL) + "/"
}
