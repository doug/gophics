package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func mustOpen(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mkItem(feedID, guid, title string, published time.Time, words int) *Item {
	return &Item{
		ID:          ItemID(feedID, guid),
		FeedID:      feedID,
		FeedName:    feedID,
		Category:    "tech",
		Tags:        []string{"blog"},
		Title:       title,
		Link:        "https://x.test/" + guid,
		GUID:        guid,
		Published:   published,
		Fetched:     time.Now().UTC(),
		ContentHTML: "<p>body</p>",
		Source:      SourceFeed,
		WordCount:   words,
	}
}

func TestPutAndQuery(t *testing.T) {
	s := mustOpen(t)
	now := time.Now().UTC().Truncate(time.Second)

	a := mkItem("blog", "1", "First", now.Add(-1*time.Hour), 500)
	b := mkItem("blog", "2", "Second", now.Add(-48*time.Hour), 100)
	for _, it := range []*Item{a, b} {
		if err := s.Put(it); err != nil {
			t.Fatal(err)
		}
	}

	all, err := s.Items(Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d items, want 2", len(all))
	}
	// Newest first.
	if all[0].Title != "First" {
		t.Errorf("order wrong: %s then %s", all[0].Title, all[1].Title)
	}

	recent, err := s.Items(Query{Since: now.Add(-24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Title != "First" {
		t.Errorf("Since filter got %d items", len(recent))
	}

	long, err := s.Items(Query{MinWords: 200})
	if err != nil {
		t.Fatal(err)
	}
	if len(long) != 1 || long[0].Title != "First" {
		t.Errorf("MinWords filter got %d items", len(long))
	}

	none, err := s.Items(Query{Categories: []string{"cooking"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(none) != 0 {
		t.Errorf("category filter should have excluded everything, got %d", len(none))
	}

	tagged, err := s.Items(Query{Tags: []string{"BLOG"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(tagged) != 2 {
		t.Errorf("tag match should be case-insensitive, got %d", len(tagged))
	}
}

func TestDedupViaItemID(t *testing.T) {
	s := mustOpen(t)
	now := time.Now().UTC()

	it := mkItem("blog", "guid-1", "Title", now, 100)
	if s.Has(it.Published, it.FeedID, it.ID) {
		t.Error("item should not exist yet")
	}
	if err := s.Put(it); err != nil {
		t.Fatal(err)
	}
	if !s.Has(it.Published, it.FeedID, it.ID) {
		t.Error("item should exist after Put")
	}

	// The same GUID must map to the same ID, and therefore the same file.
	again := mkItem("blog", "guid-1", "Title Revised", now, 100)
	if again.ID != it.ID {
		t.Errorf("ID not stable: %s vs %s", again.ID, it.ID)
	}

	// A different feed with the same GUID is a different item.
	other := mkItem("otherblog", "guid-1", "Title", now, 100)
	if other.ID == it.ID {
		t.Error("IDs must be scoped per feed")
	}
}

// A feed that revises an entry's timestamp must not produce a duplicate.
func TestHasFindsItemAfterDateChange(t *testing.T) {
	s := mustOpen(t)
	orig := time.Now().UTC().Add(-72 * time.Hour)
	it := mkItem("blog", "g", "T", orig, 100)
	if err := s.Put(it); err != nil {
		t.Fatal(err)
	}
	// Same feed and GUID, but the feed now claims a different date.
	revised := orig.Add(48 * time.Hour)
	if !s.Has(revised, it.FeedID, it.ID) {
		t.Error("Has should locate the item despite a changed publication date")
	}
}

func TestPerFeedLimitAndLimit(t *testing.T) {
	s := mustOpen(t)
	now := time.Now().UTC()
	for i := 0; i < 5; i++ {
		if err := s.Put(mkItem("noisy", string(rune('a'+i)), "N", now.Add(-time.Duration(i)*time.Minute), 100)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 2; i++ {
		if err := s.Put(mkItem("quiet", string(rune('x'+i)), "Q", now.Add(-time.Duration(i)*time.Minute), 100)); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.Items(Query{PerFeedLimit: 2})
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, it := range got {
		counts[it.FeedID]++
	}
	if counts["noisy"] != 2 {
		t.Errorf("noisy feed contributed %d items, want 2", counts["noisy"])
	}
	if counts["quiet"] != 2 {
		t.Errorf("quiet feed contributed %d items, want 2", counts["quiet"])
	}

	limited, err := s.Items(Query{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 3 {
		t.Errorf("Limit got %d items, want 3", len(limited))
	}
}

func TestMarkReadAndUnreadOnly(t *testing.T) {
	s := mustOpen(t)
	now := time.Now().UTC()
	a := mkItem("blog", "1", "A", now, 100)
	b := mkItem("blog", "2", "B", now.Add(-time.Minute), 100)
	s.Put(a)
	s.Put(b)

	if err := s.MarkRead([]*Item{a}); err != nil {
		t.Fatal(err)
	}
	unread, err := s.Items(Query{UnreadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(unread) != 1 || unread[0].Title != "B" {
		t.Errorf("unread got %d items", len(unread))
	}
}

func TestFeedState(t *testing.T) {
	s := mustOpen(t)

	// A feed never seen before yields a usable zero state.
	st, err := s.LoadState("newfeed")
	if err != nil {
		t.Fatal(err)
	}
	if st.FeedID != "newfeed" || st.ETag != "" {
		t.Errorf("unexpected zero state: %+v", st)
	}

	st.ETag = `"abc"`
	st.LastItemCount = 0 // an empty feed is a valid observation
	st.LastSuccess = time.Now().UTC().Truncate(time.Second)
	if err := s.SaveState(st); err != nil {
		t.Fatal(err)
	}

	back, err := s.LoadState("newfeed")
	if err != nil {
		t.Fatal(err)
	}
	if back.ETag != `"abc"` {
		t.Errorf("ETag = %q", back.ETag)
	}
	if !back.LastSuccess.Equal(st.LastSuccess) {
		t.Errorf("LastSuccess = %v, want %v", back.LastSuccess, st.LastSuccess)
	}

	states, err := s.States()
	if err != nil {
		t.Fatal(err)
	}
	if len(states) != 1 {
		t.Errorf("States got %d, want 1", len(states))
	}
}

func TestCorruptStateFileDoesNotBlockPolling(t *testing.T) {
	s := mustOpen(t)
	path := filepath.Join(s.Root(), "feeds", "broken.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	st, err := s.LoadState("broken")
	if err != nil {
		t.Fatalf("a corrupt state file should not be an error: %v", err)
	}
	if st.FeedID != "broken" {
		t.Errorf("want a usable zero state, got %+v", st)
	}
}

func TestCorruptItemIsSkipped(t *testing.T) {
	s := mustOpen(t)
	now := time.Now().UTC()
	if err := s.Put(mkItem("blog", "good", "Good", now, 100)); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.Root(), "items", now.Format("2006/01/02"))
	if err := os.WriteFile(filepath.Join(dir, "blog-bad.json"), []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	items, err := s.Items(Query{})
	if err != nil {
		t.Fatalf("a corrupt item should not fail the query: %v", err)
	}
	if len(items) != 1 || items[0].Title != "Good" {
		t.Errorf("got %d items, want just the good one", len(items))
	}
}

func TestZeroDateItemsAreQueryable(t *testing.T) {
	s := mustOpen(t)
	it := mkItem("blog", "nodate", "No Date", time.Time{}, 100)
	if err := s.Put(it); err != nil {
		t.Fatal(err)
	}
	// The unknown-date bucket must be visited even by a windowed query.
	items, err := s.Items(Query{Since: time.Now().Add(-24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	// It is filtered out by date, but must not crash the walk.
	if len(items) != 0 {
		t.Errorf("got %d items", len(items))
	}
	all, err := s.Items(Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Errorf("undated item should still be stored, got %d", len(all))
	}
}

func TestStats(t *testing.T) {
	s := mustOpen(t)
	now := time.Now().UTC()
	a := mkItem("blog", "1", "A", now, 500)
	a.Source = SourceExtracted
	b := mkItem("news", "2", "B", now.Add(-time.Hour), 100)
	b.Category = "news"
	b.Source = SourceSummary
	s.Put(a)
	s.Put(b)
	s.MarkRead([]*Item{a})

	st, err := s.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Items != 2 {
		t.Errorf("Items = %d", st.Items)
	}
	if st.Unread != 1 {
		t.Errorf("Unread = %d", st.Unread)
	}
	if st.Feeds != 2 {
		t.Errorf("Feeds = %d", st.Feeds)
	}
	if st.BySource[SourceExtracted] != 1 || st.BySource[SourceSummary] != 1 {
		t.Errorf("BySource = %v", st.BySource)
	}
	if st.ByCategory["tech"] != 1 || st.ByCategory["news"] != 1 {
		t.Errorf("ByCategory = %v", st.ByCategory)
	}
}

func TestPrune(t *testing.T) {
	s := mustOpen(t)
	now := time.Now().UTC()
	old := mkItem("blog", "old", "Old", now.Add(-60*24*time.Hour), 100)
	oldUnread := mkItem("blog", "oldu", "Old Unread", now.Add(-60*24*time.Hour), 100)
	fresh := mkItem("blog", "new", "New", now, 100)
	s.Put(old)
	s.Put(oldUnread)
	s.Put(fresh)
	s.MarkRead([]*Item{old})

	n, err := s.Prune(now.Add(-30*24*time.Hour), true)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("pruned %d, want 1 (only the read old item)", n)
	}
	left, err := s.Items(Query{})
	if err != nil {
		t.Fatal(err)
	}
	if len(left) != 2 {
		t.Errorf("%d items left, want 2", len(left))
	}
}

func TestAtomicWriteLeavesNoTempFiles(t *testing.T) {
	s := mustOpen(t)
	now := time.Now().UTC()
	if err := s.Put(mkItem("blog", "1", "A", now, 100)); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(s.Root(), "items", now.Format("2006/01/02"))
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if len(e.Name()) > 0 && e.Name()[0] == '.' {
			t.Errorf("temporary file left behind: %s", e.Name())
		}
	}
}
