package library

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/net/html"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/feed"
	"github.com/doug/gophics/examples/news/internal/fetch"
)

// Candidate is a feed that discovery found, with enough of its content to
// decide whether to subscribe without leaving the screen.
type Candidate struct {
	Title    string
	URL      string
	SiteURL  string
	Items    []PreviewItem
	Fulltext catalog.Fulltext
	Err      string
}

// PreviewItem is one recent entry, for the preview list.
type PreviewItem struct {
	Title     string
	Summary   string
	Published string
	Words     int
}

// Discover turns whatever the user typed into feeds they can subscribe to.
//
// People paste the address of a site they read, not the address of its feed —
// asking for the feed URL is asking them to do the app's job. So a plain site
// address is fetched and read for the <link rel="alternate"> tags that
// publishers put in their markup, and if a site has none, the handful of
// conventional paths are tried directly.
func Discover(ctx context.Context, input string) ([]Candidate, error) {
	raw := strings.TrimSpace(input)
	if raw == "" {
		return nil, fmt.Errorf("enter a website or feed address")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	if _, err := url.Parse(raw); err != nil {
		return nil, fmt.Errorf("that does not look like an address")
	}

	client := fetch.NewClient()
	resp, err := client.Do(ctx, fetch.Request{URL: raw})
	if err != nil {
		return nil, err
	}

	// The address might already be a feed. Try that first: it costs nothing,
	// and it is what happens when someone pastes a link they got from a "RSS"
	// button.
	if f, err := feed.Parse(resp.Body); err == nil && (len(f.Items) > 0 || f.Title != "") {
		return []Candidate{candidateFrom(f, resp.FinalURL, raw)}, nil
	}

	// Otherwise it is a web page: read the feeds it declares.
	links := feedLinks(resp.Body, resp.FinalURL)
	if len(links) == 0 {
		links = guessFeedPaths(resp.FinalURL)
	}
	if len(links) == 0 {
		return nil, fmt.Errorf("no feed found on that page")
	}

	// Fetch the candidates in parallel; a site commonly declares three.
	out := make([]Candidate, len(links))
	var wg sync.WaitGroup
	for i, l := range links {
		wg.Add(1)
		go func(i int, l discovered) {
			defer wg.Done()
			r, err := client.Do(ctx, fetch.Request{URL: l.href})
			if err != nil {
				out[i] = Candidate{Title: l.title, URL: l.href, Err: err.Error()}
				return
			}
			f, err := feed.Parse(r.Body)
			if err != nil {
				out[i] = Candidate{Title: l.title, URL: l.href, Err: "not a feed"}
				return
			}
			c := candidateFrom(f, r.FinalURL, resp.FinalURL)
			if c.Title == "" {
				c.Title = l.title
			}
			out[i] = c
		}(i, l)
	}
	wg.Wait()

	// Keep only what actually parsed, unless nothing did — in which case the
	// errors are the useful answer.
	var good []Candidate
	for _, c := range out {
		if c.Err == "" && (len(c.Items) > 0 || c.Title != "") {
			good = append(good, c)
		}
	}
	if len(good) == 0 {
		return out, nil
	}
	return good, nil
}

// Preview fetches a feed and summarises its recent entries, so a source can be
// looked at before it is subscribed to.
func Preview(ctx context.Context, feedURL string) (Candidate, error) {
	client := fetch.NewClient()
	resp, err := client.Do(ctx, fetch.Request{URL: feedURL})
	if err != nil {
		return Candidate{}, err
	}
	f, err := feed.Parse(resp.Body)
	if err != nil {
		return Candidate{}, err
	}
	return candidateFrom(f, resp.FinalURL, f.Link), nil
}

// previewItems is how many recent entries a preview shows. Enough to judge what
// a source publishes; few enough to read at a glance.
const previewItems = 6

func candidateFrom(f *feed.Feed, feedURL, siteURL string) Candidate {
	c := Candidate{Title: strings.TrimSpace(f.Title), URL: feedURL, SiteURL: siteURL}
	if f.Link != "" {
		c.SiteURL = f.Link
	}
	var totalText int
	for i, it := range f.Items {
		if i >= previewItems {
			break
		}
		summary := feed.StripTags(it.Summary)
		p := PreviewItem{
			Title:   strings.TrimSpace(it.Title),
			Summary: truncate(summary, 180),
			Words:   len(strings.Fields(feed.StripTags(it.Content))),
		}
		if d := it.Date(); !d.IsZero() {
			p.Published = d.Format("Jan 2")
		}
		c.Items = append(c.Items, p)
		totalText += it.TextLen()
	}
	c.Fulltext = classify(totalText, len(c.Items))
	return c
}

// classify records what the feed itself ships, using the same boundaries the
// catalog uses, so the reader knows up front whether articles will need
// fetching from the site.
func classify(totalText, n int) catalog.Fulltext {
	if n == 0 {
		return ""
	}
	switch avg := totalText / n; {
	case avg >= 4000:
		return catalog.FullText
	case avg >= 800:
		return catalog.Partial
	default:
		return catalog.Teaser
	}
}

type discovered struct {
	href  string
	title string
}

// feedLinks reads <link rel="alternate"> out of a page's markup.
func feedLinks(page []byte, baseURL string) []discovered {
	doc, err := html.Parse(strings.NewReader(string(page)))
	if err != nil {
		return nil
	}
	base, _ := url.Parse(baseURL)

	var out []discovered
	seen := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "link" {
			var rel, typ, href, title string
			for _, a := range n.Attr {
				switch strings.ToLower(a.Key) {
				case "rel":
					rel = strings.ToLower(a.Val)
				case "type":
					typ = strings.ToLower(a.Val)
				case "href":
					href = a.Val
				case "title":
					title = a.Val
				}
			}
			if strings.Contains(rel, "alternate") && isFeedType(typ) && href != "" {
				if abs := resolve(base, href); abs != "" && !seen[abs] {
					seen[abs] = true
					out = append(out, discovered{abs, strings.TrimSpace(title)})
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return out
}

func isFeedType(t string) bool {
	switch {
	case strings.Contains(t, "rss+xml"), strings.Contains(t, "atom+xml"),
		strings.Contains(t, "feed+json"), t == "application/xml", t == "text/xml":
		return true
	}
	return false
}

// guessFeedPaths is the fallback for sites that publish a feed but never say
// so in their markup, which is common on hand-built and static sites.
func guessFeedPaths(pageURL string) []discovered {
	u, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	root := u.Scheme + "://" + u.Host
	paths := []string{"/feed", "/rss", "/feed.xml", "/rss.xml", "/atom.xml", "/index.xml", "/feed/"}
	out := make([]discovered, 0, len(paths))
	for _, p := range paths {
		out = append(out, discovered{root + p, ""})
	}
	return out
}

func resolve(base *url.URL, href string) string {
	u, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if base != nil {
		u = base.ResolveReference(u)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return u.String()
}

// FeedFor builds a catalog entry for a discovered candidate, guessing the
// settings the fetcher needs from what the preview revealed.
func (c Candidate) FeedFor(category string) catalog.Feed {
	f := catalog.Feed{
		ID:       idFromURL(c.URL),
		Title:    c.Title,
		URL:      c.URL,
		Category: category,
		Fulltext: c.Fulltext,
		Priority: catalog.Normal,
	}
	if f.Title == "" {
		f.Title = FeedDomain(c.URL)
	}
	return f
}
