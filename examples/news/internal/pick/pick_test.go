package pick

import (
	"testing"
	"time"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/store"
)

func item(feed, cat, title string, words int, ageHours float64) *store.Item {
	return &store.Item{
		FeedID:    feed,
		Category:  cat,
		Title:     title,
		WordCount: words,
		Source:    store.SourceExtracted,
		Published: time.Now().Add(-time.Duration(ageHours) * time.Hour),
	}
}

func titles(items []*store.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

func has(items []*store.Item, title string) bool {
	for _, it := range items {
		if it.Title == title {
			return true
		}
	}
	return false
}

func prio(m map[string]catalog.Priority) PriorityFunc {
	return func(feed string) catalog.Priority {
		if p, ok := m[feed]; ok {
			return p
		}
		return catalog.Normal
	}
}

func TestMinutes(t *testing.T) {
	cases := map[int]int{0: 1, 1: 1, 230: 1, 231: 2, 2300: 10, 29861: 130}
	for words, want := range cases {
		if got := Minutes(&store.Item{WordCount: words}); got != want {
			t.Errorf("Minutes(%d words) = %d, want %d", words, got, want)
		}
	}
}

// The whole point of a minute budget: an issue's length is bounded regardless of
// how long the individual articles are.
func TestBudgetIsRespected(t *testing.T) {
	items := []*store.Item{
		item("a", "science", "ten minutes", 2300, 1),
		item("b", "tech", "ten more", 2300, 2),
		item("c", "econ", "ten again", 2300, 3),
		item("d", "ai", "two minutes", 400, 4),
	}
	res := Select(items, nil, Params{Minutes: 15})

	if res.Minutes > 15 {
		t.Errorf("issue is %d minutes, over the 15 minute budget: %v", res.Minutes, titles(res.Selected))
	}
	if len(res.Selected) == 0 {
		t.Fatal("nothing selected")
	}
	// A short item should still be picked up after a long one no longer fits.
	if !has(res.Selected, "two minutes") {
		t.Errorf("a short item should fill the remaining budget, got %v", titles(res.Selected))
	}
	if len(res.Deferred) == 0 {
		t.Error("items that did not fit should be deferred, not dropped")
	}
}

// A single item larger than the whole budget must still produce an issue rather
// than an empty one.
func TestOversizedSingleItemStillProducesAnIssue(t *testing.T) {
	res := Select([]*store.Item{item("a", "science", "huge", 29861, 1)}, nil, Params{Minutes: 15})
	if len(res.Selected) != 1 {
		t.Errorf("expected the one item to be selected anyway, got %v", titles(res.Selected))
	}
}

func TestLongItemsGoToOverflow(t *testing.T) {
	items := []*store.Item{
		item("owl", "science", "129 minute essay", 29861, 1),
		item("owl", "science", "51 minute essay", 11854, 2),
		item("q", "science", "normal", 2000, 3),
	}
	res := Select(items, nil, Params{Minutes: 60, MaxItemMinutes: 25})

	if len(res.Overflow) != 2 {
		t.Fatalf("expected 2 overflow items, got %v", titles(res.Overflow))
	}
	// Newest first, so publishing one at a time works through them in order.
	if res.Overflow[0].Title != "129 minute essay" {
		t.Errorf("overflow should keep newest-first order, got %v", titles(res.Overflow))
	}
	if !has(res.Selected, "normal") {
		t.Errorf("the normal item should remain in the issue, got %v", titles(res.Selected))
	}
	for _, it := range res.Selected {
		if Minutes(it) > 25 {
			t.Errorf("%q is %d minutes, over MaxItemMinutes", it.Title, Minutes(it))
		}
	}
}

func TestPriorityOutranksRecency(t *testing.T) {
	items := []*store.Item{
		item("filler", "news", "fresh filler", 1000, 1),
		item("must", "science", "day old must-read", 1000, 25),
	}
	res := Select(items, prio(map[string]catalog.Priority{
		"filler": catalog.Filler, "must": catalog.MustRead,
	}), Params{Minutes: 5})

	if len(res.Selected) != 1 || res.Selected[0].Title != "day old must-read" {
		t.Errorf("a must-read source should win a tight budget, got %v", titles(res.Selected))
	}
}

// Within one priority tier, freshness decides.
func TestRecencyOrdersEqualPriority(t *testing.T) {
	items := []*store.Item{
		item("a", "news", "older", 1000, 48),
		item("b", "news", "newer", 1000, 1),
	}
	res := Select(items, nil, Params{Minutes: 5})
	if res.Selected[0].Title != "newer" {
		t.Errorf("expected the fresher item first, got %v", titles(res.Selected))
	}
}

// A very stale must-read should eventually lose to fresh normal material, or an
// issue would be permanently occupied by old favourites.
func TestStaleMustReadLosesToFresh(t *testing.T) {
	items := []*store.Item{
		item("must", "science", "ancient must-read", 1000, 24*200),
		item("normal", "news", "fresh normal", 1000, 1),
	}
	res := Select(items, prio(map[string]catalog.Priority{"must": catalog.MustRead}), Params{Minutes: 5})
	if res.Selected[0].Title != "fresh normal" {
		t.Errorf("a 200-day-old must-read should not outrank fresh material, got %v",
			titles(res.Selected))
	}
}

func TestPerFeedCap(t *testing.T) {
	var items []*store.Item
	for i := 0; i < 5; i++ {
		items = append(items, item("noisy", "news", string(rune('a'+i)), 200, float64(i)))
	}
	items = append(items, item("quiet", "science", "quiet piece", 200, 9))

	res := Select(items, nil, Params{Minutes: 60, PerFeed: 2})
	var noisy int
	for _, it := range res.Selected {
		if it.FeedID == "noisy" {
			noisy++
		}
	}
	if noisy != 2 {
		t.Errorf("noisy feed contributed %d items, want 2", noisy)
	}
	if !has(res.Selected, "quiet piece") {
		t.Error("capping a noisy feed should leave room for others")
	}
}

// The observed failure this exists to prevent: one category taking 46% of an
// issue because its articles happen to be the longest.
func TestCategoryShareCap(t *testing.T) {
	var items []*store.Item
	for i := 0; i < 6; i++ {
		items = append(items, item("sci", "science", "long science "+string(rune('a'+i)), 2300, float64(i)))
	}
	for i := 0; i < 6; i++ {
		items = append(items, item("ne", "newengland", "local "+string(rune('a'+i)), 700, float64(i)))
	}

	res := Select(items, nil, Params{Minutes: 60, MaxCategoryShare: 0.5})
	mins := ByCategoryMinutes(res.Selected)
	if mins["science"] > 35 {
		t.Errorf("science took %d of a 60 minute issue with a 50%% cap: %v", mins["science"], mins)
	}
	if mins["newengland"] == 0 {
		t.Errorf("the capped issue should make room for other categories: %v", mins)
	}
}

// A quota must never silence a category outright, even when its only item is
// larger than the quota.
func TestCategoryCapAllowsFirstItem(t *testing.T) {
	items := []*store.Item{
		item("sci", "science", "one long science piece", 4600, 1),
		item("ne", "newengland", "local", 700, 2),
	}
	res := Select(items, nil, Params{Minutes: 60, MaxCategoryShare: 0.1})
	if !has(res.Selected, "one long science piece") {
		t.Errorf("a category's first item should survive its quota, got %v", titles(res.Selected))
	}
}

func TestSummaryOnlyExcludedByDefault(t *testing.T) {
	teaser := item("hn", "tech", "eleven word teaser", 11, 1)
	teaser.Source = store.SourceSummary
	real := item("q", "science", "real article", 1000, 2)

	res := Select([]*store.Item{teaser, real}, nil, Params{Minutes: 30})
	if has(res.Selected, "eleven word teaser") {
		t.Error("a summary-only item should not appear in an issue")
	}
	if res.Excluded["summary only, no article body"] != 1 {
		t.Errorf("the exclusion should be counted and explained: %v", res.Excluded)
	}

	res = Select([]*store.Item{teaser, real}, nil, Params{Minutes: 30, AllowSummaryOnly: true})
	if !has(res.Selected, "eleven word teaser") {
		t.Error("AllowSummaryOnly should let teasers through")
	}
}

func TestMinPriorityFilter(t *testing.T) {
	items := []*store.Item{
		item("filler", "news", "filler", 500, 1),
		item("normal", "news", "normal", 500, 2),
		item("must", "news", "must", 500, 3),
	}
	res := Select(items, prio(map[string]catalog.Priority{
		"filler": catalog.Filler, "must": catalog.MustRead,
	}), Params{Minutes: 60, MinPriority: catalog.Normal})

	if has(res.Selected, "filler") {
		t.Error("a filler-rated feed should be dropped by MinPriority")
	}
	if !has(res.Selected, "normal") || !has(res.Selected, "must") {
		t.Errorf("normal and must-read should survive, got %v", titles(res.Selected))
	}
	if res.Excluded["below minimum priority"] != 1 {
		t.Errorf("Excluded = %v", res.Excluded)
	}
}

func TestUnboundedBudgetTakesEverything(t *testing.T) {
	items := []*store.Item{
		item("a", "news", "one", 5000, 1),
		item("b", "news", "two", 5000, 2),
	}
	res := Select(items, nil, Params{})
	if len(res.Selected) != 2 {
		t.Errorf("with no budget everything eligible should be selected, got %v", titles(res.Selected))
	}
	if len(res.Deferred) != 0 {
		t.Errorf("nothing should be deferred: %v", titles(res.Deferred))
	}
}

func TestEmptyInput(t *testing.T) {
	res := Select(nil, nil, Params{Minutes: 30})
	if len(res.Selected) != 0 || res.Minutes != 0 {
		t.Errorf("empty input should give an empty result, got %+v", res)
	}
}

// Nothing may be lost: every candidate ends up selected, overflowed, deferred or
// explicitly excluded.
func TestEveryItemIsAccountedFor(t *testing.T) {
	var items []*store.Item
	for i := 0; i < 10; i++ {
		items = append(items, item("a", "news", string(rune('a'+i)), 1000+i*900, float64(i)))
	}
	teaser := item("b", "tech", "teaser", 20, 1)
	teaser.Source = store.SourceSummary
	items = append(items, teaser)

	res := Select(items, nil, Params{Minutes: 20, MaxItemMinutes: 8, PerFeed: 3})

	var excluded int
	for _, n := range res.Excluded {
		excluded += n
	}
	total := len(res.Selected) + len(res.Overflow) + len(res.Deferred) + excluded
	if total != len(items) {
		t.Errorf("accounted for %d of %d items (selected %d, overflow %d, deferred %d, excluded %d)",
			total, len(items), len(res.Selected), len(res.Overflow), len(res.Deferred), excluded)
	}
}
