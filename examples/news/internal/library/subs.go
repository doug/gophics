package library

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/doug/gophics/examples/news/internal/catalog"
)

// suggestionsJSON is the catalog compiled into the binary: 146 feeds verified
// end to end, each carrying what the reader needs to treat it well — whether
// the feed ships full text or a teaser, what sort of value it delivers, and how
// slowly the host wants to be polled.
//
// It is the *suggestion* list, never the subscription list. What the reader
// actually polls lives in subscriptions.json under the data directory, so
// upgrading the app can add suggestions without touching anyone's choices.
//
//go:embed feeds.json
var suggestionsJSON []byte

var (
	sugOnce sync.Once
	sugCat  *catalog.Catalog
	sugErr  error
)

// Suggestions returns the built-in catalog.
func Suggestions() (*catalog.Catalog, error) {
	sugOnce.Do(func() { sugCat, sugErr = catalog.Parse(suggestionsJSON) })
	return sugCat, sugErr
}

// Subscriptions is the user's own feed list. Entries are full catalog.Feed
// values rather than references into the suggestion catalog, so a feed added by
// URL behaves exactly like a suggested one and editing a suggested feed's
// settings does not fight with the next app update.
type Subscriptions struct {
	Feeds []catalog.Feed `json:"feeds"`

	mu sync.RWMutex
}

// LoadSubscriptions reads the user's feed list, seeding it on first run with
// the catalog's must-read feeds so the app has something to show before any
// setup. A corrupt file is not fatal: the reader reseeds rather than refusing
// to start, since the file is a preference and the articles live elsewhere.
func LoadSubscriptions() *Subscriptions {
	s := &Subscriptions{}
	data, err := os.ReadFile(SubscriptionsPath())
	if err == nil && json.Unmarshal(data, s) == nil && len(s.Feeds) > 0 {
		return s
	}
	s.Feeds = starterFeeds()
	s.Save()
	return s
}

// starterFeeds is the first-run selection: every feed the catalog rates
// must-read. That is 25 sources spread across all ten categories — enough that
// the first refresh fills a queue, few enough to prune by hand.
func starterFeeds() []catalog.Feed {
	c, err := Suggestions()
	if err != nil {
		return nil
	}
	var out []catalog.Feed
	for _, f := range c.Feeds {
		if f.Priority == catalog.MustRead && f.IsEnabled() {
			out = append(out, f)
		}
	}
	return out
}

// Save writes the list atomically, so an interrupted write cannot leave the
// reader with no subscriptions.
func (s *Subscriptions) Save() error {
	s.mu.RLock()
	data, err := json.MarshalIndent(struct {
		Feeds []catalog.Feed `json:"feeds"`
	}{s.Feeds}, "", "  ")
	s.mu.RUnlock()
	if err != nil {
		return err
	}
	tmp := SubscriptionsPath() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, SubscriptionsPath())
}

// All returns a copy of the subscription list.
func (s *Subscriptions) All() []catalog.Feed {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]catalog.Feed(nil), s.Feeds...)
}

// Enabled returns the feeds a refresh should poll.
func (s *Subscriptions) Enabled() []catalog.Feed {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []catalog.Feed
	for _, f := range s.Feeds {
		if f.IsEnabled() {
			out = append(out, f)
		}
	}
	return out
}

// ByID finds a subscribed feed.
func (s *Subscriptions) ByID(id string) (catalog.Feed, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.Feeds {
		if f.ID == id {
			return f, true
		}
	}
	return catalog.Feed{}, false
}

// Has reports whether the reader is subscribed to a feed.
func (s *Subscriptions) Has(id string) bool {
	_, ok := s.ByID(id)
	return ok
}

// Add subscribes to a feed, replacing any entry with the same ID. It returns an
// error only for a feed that could never be polled.
func (s *Subscriptions) Add(f catalog.Feed) error {
	if strings.TrimSpace(f.URL) == "" {
		return fmt.Errorf("feed has no URL")
	}
	if f.ID == "" {
		f.ID = idFromURL(f.URL)
	}
	if f.Title == "" {
		f.Title = f.ID
	}
	if f.Category == "" {
		f.Category = "unsorted"
	}
	s.mu.Lock()
	replaced := false
	for i := range s.Feeds {
		if s.Feeds[i].ID == f.ID {
			s.Feeds[i], replaced = f, true
			break
		}
	}
	if !replaced {
		s.Feeds = append(s.Feeds, f)
	}
	s.mu.Unlock()
	return s.Save()
}

// Remove unsubscribes. Stored articles are left alone: they are already
// downloaded, and deleting what you have not read yet because you unsubscribed
// today is not what anyone means by the button.
func (s *Subscriptions) Remove(id string) error {
	s.mu.Lock()
	out := s.Feeds[:0]
	for _, f := range s.Feeds {
		if f.ID != id {
			out = append(out, f)
		}
	}
	s.Feeds = out
	s.mu.Unlock()
	return s.Save()
}

// SetEnabled pauses or resumes polling without losing the feed's settings.
func (s *Subscriptions) SetEnabled(id string, on bool) error {
	s.mu.Lock()
	for i := range s.Feeds {
		if s.Feeds[i].ID == id {
			v := on
			s.Feeds[i].Enabled = &v
			break
		}
	}
	s.mu.Unlock()
	return s.Save()
}

// Update applies an edit to one feed.
func (s *Subscriptions) Update(f catalog.Feed) error {
	s.mu.Lock()
	for i := range s.Feeds {
		if s.Feeds[i].ID == f.ID {
			s.Feeds[i] = f
			break
		}
	}
	s.mu.Unlock()
	return s.Save()
}

// Categories lists the categories in use, sorted.
func (s *Subscriptions) Categories() []string {
	s.mu.RLock()
	seen := map[string]bool{}
	for _, f := range s.Feeds {
		if f.Category != "" {
			seen[f.Category] = true
		}
	}
	s.mu.RUnlock()
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// idFromURL derives a stable, readable ID for a hand-added feed.
func idFromURL(u string) string {
	s := u
	for _, p := range []string{"https://", "http://", "www."} {
		s = strings.TrimPrefix(s, p)
	}
	if i := strings.IndexAny(s, "/?#"); i > 0 {
		s = s[:i]
	}
	s = strings.TrimSuffix(s, ".com")
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteByte('-')
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = "feed"
	}
	return id
}
