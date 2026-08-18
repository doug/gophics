package library

import (
	"context"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/cookies"
	"github.com/doug/gophics/examples/news/internal/extract"
	"github.com/doug/gophics/examples/news/internal/feed"
	"github.com/doug/gophics/examples/news/internal/fetch"
	"github.com/doug/gophics/examples/news/internal/store"
)

// Progress is one step of a refresh, delivered to the UI as it happens so the
// pull-to-refresh spinner can say what it is doing instead of spinning blankly.
type Progress struct {
	Done, Total int
	Feed        string
	NewItems    int
	Err         error

	// Phase is "feeds" while polling and "images" while prefetching bodies and
	// pictures for offline reading.
	Phase string
}

// RefreshResult summarises a completed refresh.
type RefreshResult struct {
	// Skipped is set when a poll was already in flight and this call did
	// nothing. Without it the zero result is indistinguishable from a refresh
	// that found no sources at all, and the UI reported "no sources" to someone
	// with twenty-five of them.
	Skipped bool

	Feeds     int
	NewItems  int
	Failed    int
	Extracted int
	Images    int
	Elapsed   time.Duration
	Errors    []string
}

// refreshConcurrency is how many feeds are polled at once. Phones are not
// short of CPU for this — the limit is being a good citizen to the hosts and
// not opening a hundred sockets on a flaky mobile connection.
const refreshConcurrency = 6

// Refresh polls every enabled feed, stores what is new, and then downloads the
// images those articles need. onProgress may be nil; when set it is called from
// a background goroutine, so a UI must marshal it onto its own thread.
func (l *Library) Refresh(ctx context.Context, onProgress func(Progress)) RefreshResult {
	l.mu.Lock()
	if l.refreshing {
		l.mu.Unlock()
		return RefreshResult{Skipped: true}
	}
	l.refreshing = true
	l.mu.Unlock()

	start := time.Now()
	res := l.pollAll(ctx, onProgress)

	// Everything new is now on disk as text. Fetching the pictures is the other
	// half of being readable underground, and it happens after the text so a
	// refresh interrupted halfway still leaves a readable queue.
	res.Images = l.prefetchImages(ctx, onProgress)
	res.Elapsed = time.Since(start)

	l.mu.Lock()
	l.refreshing = false
	l.lastRefresh = time.Now()
	l.lastErr = ""
	if len(res.Errors) > 0 {
		l.lastErr = res.Errors[0]
	}
	l.mu.Unlock()
	return res
}

func (l *Library) pollAll(ctx context.Context, onProgress func(Progress)) RefreshResult {
	feeds := l.Subs.Enabled()
	client := fetch.NewClient()
	skip := l.titleSkipper()

	var (
		mu   sync.Mutex
		res  = RefreshResult{Feeds: len(feeds)}
		done int
		wg   sync.WaitGroup
		sem  = make(chan struct{}, refreshConcurrency)
	)

	for _, fd := range feeds {
		wg.Add(1)
		go func(fd catalog.Feed) {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			added, extracted, err := l.poll(ctx, client, fd, skip)

			mu.Lock()
			done++
			res.NewItems += added
			res.Extracted += extracted
			if err != nil {
				res.Failed++
				if len(res.Errors) < 8 {
					res.Errors = append(res.Errors, fd.ID+": "+err.Error())
				}
			}
			p := Progress{Done: done, Total: len(feeds), Feed: fd.Title, NewItems: added, Err: err, Phase: "feeds"}
			mu.Unlock()

			if onProgress != nil {
				onProgress(p)
			}
		}(fd)
	}
	wg.Wait()
	return res
}

// poll fetches one feed and stores whatever is new. It is the rssfetch pipeline
// with the reporting removed and cookies resolved from the app's own directory
// rather than from a path in the catalog.
func (l *Library) poll(ctx context.Context, client *fetch.Client, fd catalog.Feed,
	skip func(string) bool) (added, extracted int, err error) {

	if l.Store == nil {
		return 0, 0, nil
	}
	state, err := l.Store.LoadState(fd.ID)
	if err != nil {
		return 0, 0, err
	}

	req := fetch.Request{
		URL:          fd.URL,
		UserAgent:    fd.UserAgent,
		MinInterval:  fd.MinInterval(),
		ETag:         state.ETag,
		LastModified: state.LastModified,
		Cookies:      l.cookiesFor(fd),
	}

	resp, err := client.Do(ctx, req)
	state.LastFetch = time.Now().UTC()
	state.URL = fd.URL

	if err != nil {
		state.ConsecutiveErr++
		state.LastError = err.Error()
		l.Store.SaveState(state)
		return 0, 0, err
	}

	// An unchanged feed is the common case and costs one 304.
	if resp.NotModified {
		state.ConsecutiveErr, state.LastError = 0, ""
		state.LastSuccess = time.Now().UTC()
		l.Store.SaveState(state)
		return 0, 0, nil
	}

	parsed, err := feed.Parse(resp.Body)
	if err != nil {
		state.ConsecutiveErr++
		state.LastError = err.Error()
		l.Store.SaveState(state)
		return 0, 0, err
	}

	// A successful fetch with no entries is a normal observation, not a
	// failure: several sources serve empty channels routinely. The validators
	// must be kept either way so the next poll stays conditional.
	state.ETag, state.LastModified = resp.ETag, resp.LastModified
	state.LastSuccess = time.Now().UTC()
	state.LastItemCount = len(parsed.Items)
	state.ConsecutiveErr, state.LastError = 0, ""

	feedName := fd.Title
	if feedName == "" {
		feedName = parsed.Title
	}
	opts := extract.DefaultOptions()
	opts.KeepImages = true

	for _, it := range parsed.Items {
		if ctx.Err() != nil {
			break
		}
		if skip != nil && skip(it.Title) {
			continue
		}
		id := store.ItemID(fd.ID, it.GUID)
		published := it.Date()
		if published.IsZero() {
			// Without a date an item cannot be windowed; treat first sighting as
			// its date so it still reaches the queue.
			published = time.Now().UTC()
		}
		if l.Store.Has(published, fd.ID, id) {
			continue
		}

		rec := &store.Item{
			ID:        id,
			FeedID:    fd.ID,
			FeedName:  feedName,
			Category:  fd.Category,
			Tags:      fd.Tags,
			Title:     strings.TrimSpace(it.Title),
			Link:      it.Link,
			Author:    it.Author,
			GUID:      it.GUID,
			Published: published,
			Fetched:   time.Now().UTC(),
			Summary:   truncate(feed.StripTags(it.Summary), 400),
			LeadImage: leadImage(it),
		}
		if rec.Title == "" {
			rec.Title = "(untitled)"
		}
		if l.fillContent(ctx, client, fd, it, rec, opts) {
			extracted++
		}
		if err := l.Store.Put(rec); err != nil {
			break
		}
		added++
	}

	state.NewItems = added
	l.Store.SaveState(state)
	return added, extracted, nil
}

// fillContent decides where an article's body comes from: the feed itself, or
// the article page. The catalog's fulltext classification chooses, and an
// extraction failure always falls back rather than losing the item.
func (l *Library) fillContent(ctx context.Context, client *fetch.Client, fd catalog.Feed,
	it feed.Item, rec *store.Item, opts extract.Options) bool {

	feedBody := it.Content
	feedText := it.TextLen()

	if fd.ShouldExtract() && rec.Link != "" {
		art, err := l.extractArticle(ctx, client, fd, rec.Link, opts)
		switch {
		case err != nil:
			rec.ExtractError = err.Error()
		case art.WordCount > 0 && len(art.Text) > feedText:
			// Only prefer the extraction when it actually beat the feed body.
			rec.ContentHTML = art.HTML
			rec.WordCount = art.WordCount
			rec.Source = store.SourceExtracted
			if art.Byline != "" && rec.Author == "" {
				rec.Author = art.Byline
			}
			if art.LeadImage != "" && rec.LeadImage == "" {
				rec.LeadImage = art.LeadImage
			}
			if rec.Summary == "" {
				rec.Summary = truncate(art.Excerpt, 400)
			}
			return true
		default:
			rec.ExtractError = "extraction produced less text than the feed"
		}
	}

	if strings.TrimSpace(feedBody) != "" {
		rec.ContentHTML = extract.Sanitize(feedBody, rec.Link, opts)
		rec.WordCount = len(strings.Fields(extract.PlainText(rec.ContentHTML)))
		// The same 800-character boundary the catalog uses, so the label the
		// reader shows matches how the feed was classified.
		if feedText >= 800 {
			rec.Source = store.SourceFeed
		} else {
			rec.Source = store.SourceSummary
		}
		return false
	}

	rec.Source = store.SourceSummary
	if rec.Summary != "" {
		rec.ContentHTML = "<p>" + htmlEscape(rec.Summary) + "</p>"
		rec.WordCount = len(strings.Fields(rec.Summary))
	}
	return false
}

func (l *Library) extractArticle(ctx context.Context, client *fetch.Client, fd catalog.Feed,
	link string, opts extract.Options) (*extract.Article, error) {

	req := fetch.Request{URL: link, MinInterval: fd.MinInterval(), Cookies: l.cookiesFor(fd)}
	switch {
	case fd.ArticleUserAgent != "":
		req.UserAgent = fd.ArticleUserAgent
	case fd.UserAgent != "":
		req.UserAgent = fd.UserAgent
	}
	resp, err := client.Do(ctx, req)
	if err != nil {
		return nil, err
	}
	return extract.FromHTML(resp.Body, resp.FinalURL, opts)
}

// cookiesFor returns the session to send with this feed's requests: the app's
// own captured cookies for the publisher's domain, or the catalog's cookie file
// on desktop where one was configured by hand.
func (l *Library) cookiesFor(fd catalog.Feed) []*http.Cookie {
	if p := CookiePath(FeedDomain(fd.URL)); fileExists(p) {
		if cs, err := cookies.Load(p); err == nil {
			return cs
		}
	}
	if fd.CookieFile != "" {
		if cs, err := cookies.Load(expandHome(fd.CookieFile)); err == nil {
			return cs
		}
	}
	return nil
}

// titleSkipper drops recurring posts that are not articles — open threads, link
// round-ups, site notices. They come from sources worth reading, which is why
// they cannot simply be given a lower rating.
func (l *Library) titleSkipper() func(string) bool {
	c, err := Suggestions()
	if err != nil {
		return nil
	}
	return c.TitleSkipper()
}

func leadImage(it feed.Item) string {
	for _, e := range it.Enclosures {
		if strings.HasPrefix(e.Type, "image/") || e.Type == "" {
			return e.URL
		}
	}
	return ""
}

func htmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// truncate shortens a teaser to about n bytes, preferring a word boundary.
//
// The cut has to respect rune boundaries. Slicing at a byte index lands
// mid-character in any script whose letters are multi-byte, and since scripts
// without inter-word spaces never find a space to back up to, a Chinese or
// Japanese summary was reliably stored with a broken byte on the end and shown
// in the queue as a replacement character.
func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	cut := s[:n]
	if i := strings.LastIndexByte(cut, ' '); i > n/2 {
		cut = cut[:i]
	} else {
		// No usable word boundary: back up to the start of the partial rune.
		for len(cut) > 0 && !utf8.RuneStart(cut[len(cut)-1]) {
			cut = cut[:len(cut)-1]
		}
		cut = strings.TrimRightFunc(cut, func(r rune) bool { return r == utf8.RuneError })
	}
	return strings.TrimSpace(cut) + "…"
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func expandHome(p string) string {
	if strings.HasPrefix(p, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return home + p[1:]
		}
	}
	return p
}
