// Package cookies loads Netscape-format cookie files, the export format every
// browser extension produces.
//
// This exists so a paid subscription can be used from the reader: The Economist
// and similar publishers serve teasers in RSS and gate the body, and sending the
// subscriber's own session cookie is what turns those entries into readable
// articles. The cookie file stays on disk under the user's control and is only
// ever sent to the host it came from.
package cookies

import (
	"bufio"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// Load reads a cookie file in either of the two formats a browser will readily
// give you.
//
// Netscape cookies.txt, which extensions export, is one cookie per line,
// tab-separated:
//
//	domain  includeSubdomains  path  secure  expires  name  value
//
// Lines beginning with # are comments, except the "#HttpOnly_" prefix that
// browsers use to mark HTTP-only cookies.
//
// A raw request header is also accepted, because copying one line out of the
// DevTools network panel is far less work than installing an extension:
//
//	Cookie: name=value; other=value
//
// Header-form cookies carry no domain, so they become host-only cookies for
// whichever URL is being fetched. That is the safer of the two forms: they can
// only ever be sent to the site the feed belongs to.
func Load(path string) ([]*http.Cookie, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if out := parseNetscape(string(data)); len(out) > 0 {
		return out, nil
	}
	if out := parseHeader(string(data)); len(out) > 0 {
		return out, nil
	}
	return nil, fmt.Errorf("%s: no cookies found (expected Netscape cookies.txt "+
		"lines or a \"Cookie: a=b; c=d\" header)", path)
}

// parseNetscape reads the tab-separated export format.
func parseNetscape(body string) []*http.Cookie {
	var out []*http.Cookie
	sc := bufio.NewScanner(strings.NewReader(body))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		text := sc.Text()
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			continue
		}

		httpOnly := false
		if strings.HasPrefix(trimmed, "#HttpOnly_") {
			httpOnly = true
			trimmed = strings.TrimPrefix(trimmed, "#HttpOnly_")
		} else if strings.HasPrefix(trimmed, "#") {
			continue
		}

		fields := strings.Split(trimmed, "\t")
		if len(fields) < 7 {
			// Some exporters use runs of spaces instead of tabs.
			fields = strings.Fields(trimmed)
		}
		if len(fields) < 7 {
			continue // not a cookie line; ignore rather than fail the file
		}

		domain := fields[0]
		path := fields[2]
		secure := strings.EqualFold(fields[3], "TRUE")
		expiresRaw := fields[4]
		name := fields[5]
		value := strings.Join(fields[6:], "\t")

		if name == "" {
			continue
		}

		c := &http.Cookie{
			Name:     name,
			Value:    value,
			Path:     path,
			Domain:   strings.TrimPrefix(domain, "."),
			Secure:   secure,
			HttpOnly: httpOnly,
		}
		if secs, err := strconv.ParseInt(expiresRaw, 10, 64); err == nil && secs > 0 {
			c.Expires = time.Unix(secs, 0).UTC()
		}
		out = append(out, c)
	}
	return out
}

// parseHeader reads a raw "Cookie: a=b; c=d" request header, with or without the
// header name, spread over any number of lines.
func parseHeader(body string) []*http.Cookie {
	// Ignore comment lines so a file may explain where it came from.
	var kept []string
	for _, line := range strings.Split(body, "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") {
			kept = append(kept, t)
		}
	}
	joined := strings.Join(kept, "; ")

	// Tolerate the header name in either case, and the "Cookie:" that DevTools
	// includes when copying a request header.
	if i := strings.Index(strings.ToLower(joined), "cookie:"); i >= 0 {
		joined = joined[i+len("cookie:"):]
	}

	var out []*http.Cookie
	for _, pair := range strings.Split(joined, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, value, ok := strings.Cut(pair, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		// No Domain: the cookie becomes host-only for the URL being fetched,
		// which is both simpler and safer than guessing a domain.
		out = append(out, &http.Cookie{Name: name, Value: strings.TrimSpace(value)})
	}
	return out
}

// Expired reports the cookies that have already lapsed, so the fetcher can warn
// that a subscription session needs re-exporting instead of silently returning
// teasers forever.
func Expired(cs []*http.Cookie, now time.Time) []string {
	var names []string
	for _, c := range cs {
		if !c.Expires.IsZero() && c.Expires.Before(now) {
			names = append(names, c.Name)
		}
	}
	return names
}
