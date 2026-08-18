package library

import (
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/rank"
	"github.com/doug/gophics/examples/news/internal/store"
)

// Library is the whole reader as one object: subscriptions, the article store,
// the ranking model and the image cache. The UI holds exactly one and never
// touches the layers underneath directly.
//
// Every method is safe to call from any goroutine, because a refresh runs in
// the background while the UI keeps drawing.
type Library struct {
	Subs   *Subscriptions
	Store  *store.Store
	Rank   *rank.Ranker
	Images *ImageCache
	Prefs  *Settings

	mu sync.RWMutex
	// impressed is the set of articles already counted as seen this run; see
	// Impression. byID indexes the articles the last queue produced; see
	// remember.
	impressed   map[string]bool
	byID        map[string]*store.Item
	savePending bool
	refreshing  bool
	lastRefresh time.Time
	lastErr     string
	openErr     string
}

// Open brings up the library against the current data directory. It never
// returns an error: a phone reader that refuses to start because one file is
// unreadable is worse than one that starts empty and says so, and OpenError
// carries whatever went wrong for the UI to show.
func Open() *Library {
	l := &Library{
		Subs:   LoadSubscriptions(),
		Rank:   rank.Open(ModelPath()),
		Images: NewImageCache(ImageCacheDir()),
		Prefs:  LoadSettings(),
	}
	st, err := store.Open(StorePath())
	if err != nil {
		l.openErr = err.Error()
	}
	l.Store = st
	return l
}

// OpenError is whatever stopped the store from opening, or "".
func (l *Library) OpenError() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.openErr
}

// Meta answers the ranking model's questions about a feed from the user's own
// subscription list, falling back to the suggestion catalog for articles whose
// feed has since been removed.
func (l *Library) Meta(feedID string) (catalog.Priority, catalog.Kind) {
	if f, ok := l.Subs.ByID(feedID); ok {
		return f.Priority, f.Kind
	}
	if c, err := Suggestions(); err == nil {
		if f, ok := c.ByID(feedID); ok {
			return f.Priority, f.Kind
		}
	}
	return 0, ""
}

// QueueOptions selects which articles the queue is built from.
type QueueOptions struct {
	// Since bounds how far back to look. Zero means two weeks, which is long
	// enough that a slow week still fills a queue.
	Since time.Duration
	// Category, when set, restricts the queue to one category.
	Category string
	// FeedID, when set, restricts the queue to one feed.
	FeedID string
	// IncludeRead keeps articles already read, for browsing history.
	IncludeRead bool
	// Limit caps the result. Zero means 300.
	Limit int
}

// Queue returns the unread articles, best first. This is the home screen.
func (l *Library) Queue(o QueueOptions) []rank.Scored {
	items := l.queueItems(o)
	if items == nil {
		return nil
	}
	limit := o.Limit
	if limit == 0 {
		limit = 300
	}
	scored := l.Rank.Rank(items, l.Meta, time.Now())
	if len(scored) > limit {
		scored = scored[:limit]
	}
	l.remember(items)
	return scored
}

// queueItems is the selection half of Queue, without the ranking. Categories
// counts the same set, and the two disagreeing is how the filter row came to
// show "tech" with no number beside articles that were plainly in the queue.
func (l *Library) queueItems(o QueueOptions) []*store.Item {
	if l.Store == nil {
		return nil
	}
	since := o.Since
	if since == 0 {
		since = 14 * 24 * time.Hour
	}
	limit := o.Limit
	if limit == 0 {
		limit = 300
	}
	q := store.Query{
		Since:      time.Now().Add(-since),
		UnreadOnly: !o.IncludeRead,
		Limit:      limit * 2, // room for the ranker to reorder before we cut
	}
	if o.Category != "" {
		q.Categories = []string{o.Category}
	}
	if o.FeedID != "" {
		q.FeedIDs = []string{o.FeedID}
	}
	items, err := l.Store.Items(q)
	if err != nil {
		return nil
	}
	items = l.dropUnsubscribed(items)

	// Reach further back when the window is thin.
	//
	// Subscribing to a blog stores its whole archive — several ship every post
	// they have ever published — and almost none of it falls inside two weeks.
	// A first run that downloads 173 articles and displays 10 looks broken, so
	// when the recent window cannot fill a screen, widen it. Ranking already
	// discounts age heavily, so today's news still leads; the archive simply
	// fills in underneath instead of leaving a gap.
	if len(items) < thinQueue && o.Since == 0 {
		q.Since = time.Time{}
		if wider, err := l.Store.Items(q); err == nil {
			items = l.dropUnsubscribed(wider)
		}
	}
	return items
}

// remember indexes the articles the queue just produced, so opening one does
// not have to search for it. Pages carry an article's ID rather than the
// article, because that keeps them plain serialisable values that survive a
// hot restart — but it means the reader has to look the article up, and a
// linear search reads every file in the store.
func (l *Library) remember(items []*store.Item) {
	byID := make(map[string]*store.Item, len(items))
	for _, it := range items {
		byID[it.ID] = it
	}
	l.mu.Lock()
	l.byID = byID
	l.mu.Unlock()
}

// thinQueue is the point below which the recent window is not worth showing on
// its own — roughly two screens of articles.
const thinQueue = 15

// dropUnsubscribed hides articles from feeds no longer subscribed to. The
// articles stay on disk — unsubscribing is not a delete — but they should not
// keep appearing in the queue.
func (l *Library) dropUnsubscribed(items []*store.Item) []*store.Item {
	keep := items[:0]
	for _, it := range items {
		if l.Subs.Has(it.FeedID) {
			keep = append(keep, it)
		}
	}
	return keep
}

// Item finds one stored article by ID.
//
// The common case is an article the queue just listed, which is already in
// memory. The fallback — a full scan of the store — is for an ID that outlived
// its queue, such as a page restored by a hot restart.
func (l *Library) Item(id string) (*store.Item, bool) {
	l.mu.RLock()
	it, ok := l.byID[id]
	l.mu.RUnlock()
	if ok {
		return it, true
	}
	if l.Store == nil {
		return nil, false
	}
	items, err := l.Store.Items(store.Query{})
	if err != nil {
		return nil, false
	}
	for _, it := range items {
		if it.ID == id {
			return it, true
		}
	}
	return nil, false
}

// Categories lists the categories present in the subscription list, with the
// count of unread articles in each, so the filter row can show where the
// reading actually is.
func (l *Library) Categories() []CategoryCount {
	counts := map[string]int{}
	for _, it := range l.queueItems(QueueOptions{}) {
		counts[it.Category]++
	}
	var out []CategoryCount
	for _, c := range l.Subs.Categories() {
		out = append(out, CategoryCount{Name: c, Unread: counts[c]})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Unread > out[j].Unread })
	return out
}

// CategoryCount is a category and how much unread material it holds.
type CategoryCount struct {
	Name   string
	Unread int
}

// MarkRead records that an article was read, both in the store (so it leaves
// the queue) and in the ranking model (so the next queue is better).
func (l *Library) MarkRead(it *store.Item, finished bool) {
	if l.Store != nil {
		l.Store.MarkRead([]*store.Item{it})
	}
	sig := rank.Opened
	if finished {
		sig = rank.Finished
	}
	l.Rank.Observe(it, sig)
	l.Rank.Save()
}

// Vote records an explicit judgement.
func (l *Library) Vote(it *store.Item, up bool) {
	s := rank.ThumbDown
	if up {
		s = rank.ThumbUp
	}
	l.Rank.Observe(it, s)
	l.Rank.Save()
}

// Dismiss removes an article from the queue without opening it, and tells the
// model that is what you meant.
func (l *Library) Dismiss(it *store.Item) {
	if l.Store != nil {
		l.Store.MarkRead([]*store.Item{it})
	}
	l.Rank.Observe(it, rank.Skipped)
	l.Rank.Save()
}

// Impression notes that an article was on screen without being opened.
//
// It is called from the queue's row builder, which is the only place that knows
// what is actually visible — and which runs on every frame the list rebuilds.
// Counting each of those would be catastrophic: a row is built dozens of times
// a second, so an article would cross the skip threshold while the reader was
// still looking at it, and the queue would demote everything it had just shown.
// That is not a hypothetical; it demoted a day-old Quanta piece below a blog
// post from 2015 within one session.
//
// So an article counts once per run of the app. Three impressions then means
// three separate occasions of scrolling past it, which is what the ranking
// model's threshold is documented to mean.
func (l *Library) Impression(it *store.Item) {
	l.mu.Lock()
	if l.impressed == nil {
		l.impressed = map[string]bool{}
	}
	if l.impressed[it.ID] {
		l.mu.Unlock()
		return
	}
	l.impressed[it.ID] = true
	l.mu.Unlock()

	l.Rank.Impression(it)

	// The tally has to reach disk, since it accumulates across runs — but not
	// from here. This is called from the queue's row builder, on the frame
	// goroutine, so saving inline would marshal the whole model and write a
	// file for every row of a scroll. Hand it to a writer that coalesces.
	l.saveRankSoon()
}

// rankSaveDelay is how long the ranking model waits for the writes to stop
// before persisting. Long enough to collapse a fast scroll into one write,
// short enough that a reader who closes the app immediately afterwards keeps
// what it learned.
const rankSaveDelay = 2 * time.Second

// saveRankSoon schedules one save. Repeated calls while a save is already
// pending are absorbed by it rather than queueing more.
func (l *Library) saveRankSoon() {
	l.mu.Lock()
	if l.savePending {
		l.mu.Unlock()
		return
	}
	l.savePending = true
	l.mu.Unlock()

	time.AfterFunc(rankSaveDelay, func() {
		l.mu.Lock()
		l.savePending = false
		l.mu.Unlock()
		l.Rank.Save()
	})
}

// FlushRank writes any pending ranking state immediately. Hosts call it when
// the app goes to the background, which on a phone is the last moment anything
// is guaranteed to run.
func (l *Library) FlushRank() { l.Rank.Save() }

// Refreshing reports whether a poll is in flight, and when the last one ended.
func (l *Library) Refreshing() (bool, time.Time, string) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.refreshing, l.lastRefresh, l.lastErr
}

// FeedDomain is the host a feed belongs to, used to find its cookies.
func FeedDomain(rawURL string) string {
	s := rawURL
	for _, p := range []string{"https://", "http://"} {
		s = strings.TrimPrefix(s, p)
	}
	if i := strings.IndexAny(s, "/?#"); i > 0 {
		s = s[:i]
	}
	if i := strings.Index(s, ":"); i > 0 {
		s = s[:i]
	}
	return strings.ToLower(strings.TrimPrefix(s, "www."))
}
