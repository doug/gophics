package library

import (
	"context"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"

	"github.com/doug/gophics/examples/news/internal/store"
)

// ImageBudget is how much disk the picture cache may use. A quarter of a
// gigabyte is a few thousand article images — far more than two weeks of
// reading — and small enough that nobody notices it in their storage settings.
const ImageBudget = 250 << 20

// prefetchConcurrency is how many pictures download at once. Higher than the
// feed limit because these are static CDN objects, not polls a publisher might
// object to.
const prefetchConcurrency = 8

// prefetchImages downloads the pictures for everything currently in the queue,
// which is what makes the reader work with no connection at all. Text is
// already on disk by the time this runs — it arrives with the feed — so an
// interrupted prefetch degrades to articles that read fine but show gaps where
// photographs would be.
func (l *Library) prefetchImages(ctx context.Context, onProgress func(Progress)) int {
	if l.Prefs != nil && !l.Prefs.Prefetch() {
		return 0
	}
	urls := l.queueImageURLs()
	if len(urls) == 0 {
		return 0
	}

	var (
		wg   sync.WaitGroup
		sem  = make(chan struct{}, prefetchConcurrency)
		mu   sync.Mutex
		got  int
		done int
	)
	for _, u := range urls {
		wg.Add(1)
		go func(u string) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			err := l.Images.Fetch(ctx, u)

			mu.Lock()
			done++
			if err == nil {
				got++
			}
			p := Progress{Done: done, Total: len(urls), Phase: "images"}
			mu.Unlock()

			if onProgress != nil {
				onProgress(p)
			}
		}(u)
	}
	wg.Wait()

	l.Images.Prune(ImageBudget)
	return got
}

// maxPrefetchImages bounds one refresh. A single photo-essay can carry eighty
// pictures, and a queue of them would turn a refresh into a download session.
const maxPrefetchImages = 400

// queueImageURLs is every picture the current queue needs and does not have.
func (l *Library) queueImageURLs() []string {
	scored := l.Queue(QueueOptions{Limit: 120})
	seen := map[string]bool{}
	var out []string

	add := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] || len(out) >= maxPrefetchImages {
			return
		}
		if !strings.HasPrefix(u, "http") {
			return // data: URIs are already inline; relative ones never resolved
		}
		seen[u] = true
		if !l.Images.Have(u) {
			out = append(out, u)
		}
	}

	for _, s := range scored {
		add(s.Item.LeadImage)
		for _, u := range ImageURLs(s.Item.ContentHTML) {
			add(u)
		}
	}
	return out
}

// ImageURLs lists the pictures an article body references, in document order.
// The body has already been sanitised by the extractor, so this only has to
// walk it, not defend against it.
func ImageURLs(fragment string) []string {
	if !strings.Contains(fragment, "<img") {
		return nil
	}
	// html.Parse rather than html.ParseFragment: the parser wraps a bare
	// fragment in html/body itself, and ParseFragment needs a context node with
	// a real atom to behave. This is what the extractor already does.
	doc, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil
	}
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "img" {
			for _, a := range n.Attr {
				if a.Key == "src" && a.Val != "" {
					out = append(out, a.Val)
					break
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

// PrefetchItem downloads one article's pictures on demand, for opening
// something the last refresh did not reach.
func (l *Library) PrefetchItem(ctx context.Context, it *store.Item) {
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	if it.LeadImage != "" {
		l.Images.Fetch(ctx, it.LeadImage)
	}
	for _, u := range ImageURLs(it.ContentHTML) {
		if ctx.Err() != nil {
			return
		}
		l.Images.Fetch(ctx, u)
	}
}
