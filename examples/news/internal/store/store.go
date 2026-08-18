// Package store persists fetched items on the filesystem.
//
// The design goal is durability without a database: every item is one JSON file
// under a date-partitioned directory, and the file's existence *is* the
// deduplication record. That makes the store inspectable with ls and grep,
// trivially backed up, and impossible to corrupt beyond a single item.
package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Item is a stored article.
type Item struct {
	ID       string   `json:"id"`
	FeedID   string   `json:"feed_id"`
	FeedName string   `json:"feed_name"`
	Category string   `json:"category"`
	Tags     []string `json:"tags,omitempty"`

	Title  string `json:"title"`
	Link   string `json:"link"`
	Author string `json:"author,omitempty"`
	GUID   string `json:"guid"`

	Published time.Time `json:"published"`
	Fetched   time.Time `json:"fetched"`

	// Summary is a short teaser, plain text.
	Summary string `json:"summary,omitempty"`

	// ContentHTML is the body to render: the extracted article when extraction
	// succeeded, otherwise whatever the feed supplied. Already sanitised.
	ContentHTML string `json:"content_html,omitempty"`

	// Source records where ContentHTML came from, so a later pass can retry
	// only the items that fell back.
	Source ContentSource `json:"source"`

	WordCount int    `json:"word_count"`
	LeadImage string `json:"lead_image,omitempty"`

	// ExtractError explains why extraction was skipped or failed.
	ExtractError string `json:"extract_error,omitempty"`

	// Read marks items already delivered in a build.
	Read bool `json:"read,omitempty"`
}

// ContentSource identifies the provenance of an item's body.
type ContentSource string

const (
	SourceExtracted ContentSource = "extracted" // fetched and parsed the article page
	SourceFeed      ContentSource = "feed"      // the feed carried the full body
	SourceSummary   ContentSource = "summary"   // only a teaser was available
)

// FeedState is the per-feed bookkeeping that makes polling cheap and polite.
type FeedState struct {
	FeedID       string    `json:"feed_id"`
	URL          string    `json:"url"`
	ETag         string    `json:"etag,omitempty"`
	LastModified string    `json:"last_modified,omitempty"`
	LastFetch    time.Time `json:"last_fetch"`
	LastSuccess  time.Time `json:"last_success"`

	// LastItemCount is the number of entries the feed served last time. Zero is
	// a legitimate value: arXiv and several others serve empty channels, and a
	// zero must never be treated as an error or cause state to be discarded.
	LastItemCount int `json:"last_item_count"`

	NewItems       int    `json:"new_items"`
	ConsecutiveErr int    `json:"consecutive_errors"`
	LastError      string `json:"last_error,omitempty"`
}

// Store is a rooted filesystem store.
type Store struct {
	root string
}

// Open prepares a store rooted at dir, creating it if necessary.
func Open(dir string) (*Store, error) {
	for _, sub := range []string{"items", "feeds"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return nil, err
		}
	}
	return &Store{root: dir}, nil
}

// Root returns the store directory.
func (s *Store) Root() string { return s.root }

// ItemID derives a stable identifier from the feed and the entry's own GUID.
// The same entry therefore always maps to the same file, which is what makes
// existence a sufficient dedup check.
func ItemID(feedID, guid string) string {
	h := sha256.New()
	h.Write([]byte(feedID))
	h.Write([]byte{0})
	h.Write([]byte(guid))
	return hex.EncodeToString(h.Sum(nil)[:16])
}

// itemPath partitions items by publication date so that queries over a time
// window touch only the relevant directories.
func (s *Store) itemPath(published time.Time, feedID, id string) string {
	day := published.UTC().Format("2006/01/02")
	if published.IsZero() {
		day = "0000/00/00"
	}
	return filepath.Join(s.root, "items", day, feedID+"-"+id+".json")
}

// Has reports whether an item is already stored. It searches the expected date
// directory first and falls back to a scan, since a feed may revise an entry's
// timestamp between polls.
func (s *Store) Has(published time.Time, feedID, id string) bool {
	if _, err := os.Stat(s.itemPath(published, feedID, id)); err == nil {
		return true
	}
	found, _ := s.findByID(feedID, id)
	return found != ""
}

func (s *Store) findByID(feedID, id string) (string, error) {
	want := feedID + "-" + id + ".json"
	var found string
	err := filepath.WalkDir(filepath.Join(s.root, "items"), func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() && d.Name() == want {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	return found, err
}

// Put writes an item atomically: a temporary file plus a rename, so a crash can
// never leave a half-written record behind.
func (s *Store) Put(it *Item) error {
	if it.ID == "" {
		return errors.New("store: item has no ID")
	}
	path := s.itemPath(it.Published, it.FeedID, it.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(it, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

func writeFileAtomic(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

// Query selects stored items.
type Query struct {
	Since      time.Time
	Until      time.Time
	Categories []string
	Tags       []string
	FeedIDs    []string
	MinWords   int
	MaxWords   int
	UnreadOnly bool
	Limit      int

	// PerFeedLimit caps how many items any single feed contributes, which stops
	// a high-volume source from crowding out everything else.
	PerFeedLimit int
}

// Items returns the matching items, newest first.
func (s *Store) Items(q Query) ([]*Item, error) {
	var out []*Item

	root := filepath.Join(s.root, "items")
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // an unreadable directory should not abort the query
		}
		if d.IsDir() {
			if skipDir(root, path, q) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".json") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		it, err := readItem(path)
		if err != nil {
			return nil // skip a corrupt record rather than fail the whole query
		}
		if q.matches(it) {
			out = append(out, it)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Published.Equal(out[j].Published) {
			return out[i].Title < out[j].Title
		}
		return out[i].Published.After(out[j].Published)
	})

	if q.PerFeedLimit > 0 {
		perFeed := map[string]int{}
		kept := out[:0]
		for _, it := range out {
			if perFeed[it.FeedID] >= q.PerFeedLimit {
				continue
			}
			perFeed[it.FeedID]++
			kept = append(kept, it)
		}
		out = kept
	}
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, nil
}

// skipDir prunes date directories that cannot contain matches, so a query over
// the last week does not read years of history.
func skipDir(root, path string, q Query) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return false
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) > 3 {
		return false
	}
	// The unknown-date bucket must always be visited.
	if parts[0] == "0000" {
		return false
	}
	layouts := []string{"2006", "2006/01", "2006/01/02"}
	layout := layouts[len(parts)-1]
	t, err := time.Parse(layout, strings.Join(parts, "/"))
	if err != nil {
		return false
	}
	// end is the last instant covered by this directory.
	var end time.Time
	switch len(parts) {
	case 1:
		end = t.AddDate(1, 0, 0)
	case 2:
		end = t.AddDate(0, 1, 0)
	default:
		end = t.AddDate(0, 0, 1)
	}
	if !q.Since.IsZero() && !end.After(q.Since) {
		return true
	}
	if !q.Until.IsZero() && t.After(q.Until) {
		return true
	}
	return false
}

func (q Query) matches(it *Item) bool {
	if !q.Since.IsZero() && it.Published.Before(q.Since) {
		return false
	}
	if !q.Until.IsZero() && it.Published.After(q.Until) {
		return false
	}
	if q.UnreadOnly && it.Read {
		return false
	}
	if q.MinWords > 0 && it.WordCount < q.MinWords {
		return false
	}
	if q.MaxWords > 0 && it.WordCount > q.MaxWords {
		return false
	}
	if len(q.FeedIDs) > 0 && !containsFold(q.FeedIDs, it.FeedID) {
		return false
	}
	if len(q.Categories) > 0 && !containsFold(q.Categories, it.Category) {
		return false
	}
	if len(q.Tags) > 0 {
		var ok bool
		for _, t := range q.Tags {
			if containsFold(it.Tags, t) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	return true
}

func readItem(path string) (*Item, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var it Item
	if err := json.Unmarshal(data, &it); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &it, nil
}

// MarkRead flags items as delivered.
func (s *Store) MarkRead(items []*Item) error {
	for _, it := range items {
		if it.Read {
			continue
		}
		it.Read = true
		if err := s.Put(it); err != nil {
			return err
		}
	}
	return nil
}

// LoadState reads a feed's bookkeeping. A missing file yields a zero state,
// not an error.
func (s *Store) LoadState(feedID string) (*FeedState, error) {
	path := filepath.Join(s.root, "feeds", feedID+".json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return &FeedState{FeedID: feedID}, nil
	}
	if err != nil {
		return nil, err
	}
	var st FeedState
	if err := json.Unmarshal(data, &st); err != nil {
		// A corrupt state file costs one redundant fetch, which is preferable
		// to refusing to poll the feed at all.
		return &FeedState{FeedID: feedID}, nil
	}
	return &st, nil
}

// SaveState persists a feed's bookkeeping.
func (s *Store) SaveState(st *FeedState) error {
	path := filepath.Join(s.root, "feeds", st.FeedID+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// States returns every stored feed state, sorted by feed id.
func (s *Store) States() ([]*FeedState, error) {
	entries, err := os.ReadDir(filepath.Join(s.root, "feeds"))
	if err != nil {
		return nil, err
	}
	var out []*FeedState
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		st, err := s.LoadState(strings.TrimSuffix(e.Name(), ".json"))
		if err == nil {
			out = append(out, st)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].FeedID < out[j].FeedID })
	return out, nil
}

// Stats summarises the store.
type Stats struct {
	Items      int
	Unread     int
	Feeds      int
	Oldest     time.Time
	Newest     time.Time
	BySource   map[ContentSource]int
	ByCategory map[string]int
}

// Stats walks the store and summarises it.
func (s *Store) Stats() (*Stats, error) {
	st := &Stats{
		BySource:   map[ContentSource]int{},
		ByCategory: map[string]int{},
	}
	items, err := s.Items(Query{})
	if err != nil {
		return nil, err
	}
	feeds := map[string]bool{}
	for _, it := range items {
		st.Items++
		if !it.Read {
			st.Unread++
		}
		st.BySource[it.Source]++
		st.ByCategory[it.Category]++
		feeds[it.FeedID] = true
		if st.Oldest.IsZero() || it.Published.Before(st.Oldest) {
			st.Oldest = it.Published
		}
		if it.Published.After(st.Newest) {
			st.Newest = it.Published
		}
	}
	st.Feeds = len(feeds)
	return st, nil
}

// Prune deletes read items published before cutoff and reports how many went.
func (s *Store) Prune(cutoff time.Time, keepUnread bool) (int, error) {
	items, err := s.Items(Query{Until: cutoff})
	if err != nil {
		return 0, err
	}
	var n int
	for _, it := range items {
		if keepUnread && !it.Read {
			continue
		}
		path, err := s.findByID(it.FeedID, it.ID)
		if err != nil || path == "" {
			continue
		}
		if err := os.Remove(path); err == nil {
			n++
		}
	}
	return n, nil
}

func containsFold(hay []string, needle string) bool {
	for _, h := range hay {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}
