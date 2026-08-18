package rank

import (
	"math"
	"testing"
	"time"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/store"
)

var now = time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)

func item(id, feed, cat, title string, ageHours int, words int) *store.Item {
	return &store.Item{
		ID:        id,
		FeedID:    feed,
		FeedName:  feed,
		Category:  cat,
		Title:     title,
		WordCount: words,
		Published: now.Add(-time.Duration(ageHours) * time.Hour),
	}
}

func meta(pri catalog.Priority, kind catalog.Kind) Meta {
	return func(string) (catalog.Priority, catalog.Kind) { return pri, kind }
}

// With nothing learned the queue must still be ordered sensibly, or the app is
// useless until it has been used.
func TestColdStartPrefersMustReadAndFresh(t *testing.T) {
	m := New()
	fresh := item("a", "f1", "ai", "A compiler that proves its own passes", 2, 2000)
	stale := item("b", "f2", "ai", "A compiler that proves its own passes", 200, 2000)

	if got, want := m.Score(fresh, catalog.MustRead, catalog.Compounding, now),
		m.Score(stale, catalog.MustRead, catalog.Compounding, now); got <= want {
		t.Errorf("fresh %.3f should outrank stale %.3f", got, want)
	}
	must := m.Score(fresh, catalog.MustRead, catalog.Compounding, now)
	filler := m.Score(fresh, catalog.Filler, catalog.Current, now)
	if must <= filler {
		t.Errorf("must-read %.3f should outrank filler %.3f", must, filler)
	}
}

func TestThumbUpRaisesTheFeed(t *testing.T) {
	m := New()
	liked := item("a", "good", "tech", "Notes on database indexes", 5, 1500)
	other := item("b", "meh", "tech", "Notes on database indexes", 5, 1500)

	before := m.Score(liked, 0, "", now)
	for i := range 3 {
		m.Observe(item(string(rune('x'+i)), "good", "tech", "Something else entirely", 5, 1500), ThumbUp)
	}
	after := m.Score(liked, 0, "", now)
	if after <= before {
		t.Errorf("liking the feed should raise its articles: %.3f -> %.3f", before, after)
	}
	if m.Score(other, 0, "", now) >= after {
		t.Error("the unliked feed should not have risen with it")
	}
}

func TestSkipsPushDown(t *testing.T) {
	m := New()
	for i := range 4 {
		m.Observe(item(string(rune('p'+i)), "noisy", "news", "Daily briefing roundup", 1, 300), Skipped)
	}
	it := item("z", "noisy", "news", "Daily briefing roundup", 1, 300)
	if s := m.Score(it, 0, "", now); s >= 0.5 {
		t.Errorf("a feed skipped four times should score below even: %.3f", s)
	}
}

// The passive signal: scrolling past something repeatedly is the only evidence
// most articles will ever generate, so it has to be recorded exactly once.
func TestImpressionsBecomeASkip(t *testing.T) {
	m := New()
	it := item("a", "f", "tech", "Some headline", 1, 500)

	for i := 1; i < skipAfterImpressions; i++ {
		if m.Impression(it) {
			t.Fatalf("impression %d should not yet count as a skip", i)
		}
	}
	if !m.Impression(it) {
		t.Fatalf("impression %d should have counted as a skip", skipAfterImpressions)
	}
	if m.TotalNeg != 1 {
		t.Errorf("expected one negative signal, got %v", m.TotalNeg)
	}
	// Further impressions must not keep accumulating against it.
	m.Impression(it)
	m.Impression(it)
	if m.TotalNeg != 1 {
		t.Errorf("skip counted more than once: %v", m.TotalNeg)
	}
}

func TestOpeningAnArticleStopsItBeingSkipped(t *testing.T) {
	m := New()
	it := item("a", "f", "tech", "Some headline", 1, 500)
	m.Impression(it)
	m.Observe(it, Finished)
	for range 5 {
		if m.Impression(it) {
			t.Fatal("an article already read must never be recorded as skipped")
		}
	}
}

// A single enthusiastic evening must not convince the model that one blog is
// the only thing worth reading.
func TestContributionsCannotRunAway(t *testing.T) {
	m := New()
	for i := range 200 {
		m.Observe(item(string(rune(i)), "obsession", "ai", "Transformers scaling laws", 1, 1200), ThumbUp)
	}
	it := item("z", "obsession", "ai", "Transformers scaling laws", 1, 1200)
	for _, c := range m.Explain(it, 0, "", now) {
		if math.Abs(c.Weight) > 3 {
			t.Errorf("contribution %q ran away to %.2f", c.Label, c.Weight)
		}
	}
	if s := m.Score(it, 0, "", now); s >= 1 {
		t.Errorf("score saturated to %v", s)
	}
}

func TestExplanationNamesTheEvidence(t *testing.T) {
	m := New()
	for i := range 5 {
		m.Observe(item(string(rune('a'+i)), "jvns", "tech", "How DNS resolution actually works", 3, 900), Finished)
	}
	it := item("z", "jvns", "tech", "How DNS caching actually works", 3, 900)
	cs := m.Explain(it, catalog.MustRead, catalog.Compounding, now)
	if len(cs) == 0 {
		t.Fatal("no explanation produced")
	}
	labels := map[string]bool{}
	for _, c := range cs {
		labels[c.Label] = true
	}
	for _, want := range []string{"Feed", "Topic", "Freshness"} {
		if !labels[want] {
			t.Errorf("explanation is missing a %q term: %+v", want, cs)
		}
	}
	// Sorted by magnitude, largest first.
	for i := 1; i < len(cs); i++ {
		if math.Abs(cs[i-1].Weight) < math.Abs(cs[i].Weight) {
			t.Fatalf("explanation not sorted by magnitude: %+v", cs)
		}
	}
}

// The editorial prior should hand over to learned behaviour, not compete with
// it forever.
func TestEditorialPriorFadesAsEvidenceArrives(t *testing.T) {
	m := New()
	it := item("z", "f", "tech", "A headline", 5, 800)

	cold := weightOf(m.Explain(it, catalog.MustRead, catalog.Compounding, now), "Source rating")
	for i := range 40 {
		m.Observe(item(string(rune(i)), "other", "econ", "Unrelated things", 5, 800), Finished)
	}
	warm := weightOf(m.Explain(it, catalog.MustRead, catalog.Compounding, now), "Source rating")
	if !(warm < cold*0.6) {
		t.Errorf("prior should have faded: cold %.3f, warm %.3f", cold, warm)
	}
}

func TestLengthPreferenceNeedsEvidenceThenApplies(t *testing.T) {
	m := New()
	short := item("s", "f", "tech", "A note", 5, 400)   // ~2 min
	long := item("l", "f", "tech", "A report", 5, 9000) // ~41 min

	if w := weightOf(m.Explain(long, 0, "", now), "Length"); w != 0 {
		t.Errorf("length should say nothing before evidence, got %.3f", w)
	}
	for i := range 8 {
		m.Observe(item(string(rune(i)), "f", "tech", "A note", 5, 400), Finished)
	}
	if weightOf(m.Explain(short, 0, "", now), "Length") <= weightOf(m.Explain(long, 0, "", now), "Length") {
		t.Error("after finishing only short pieces, short should score better on length")
	}
}

func TestRankIsStableAndOrdered(t *testing.T) {
	r := &Ranker{m: New()}
	items := []*store.Item{
		item("a", "f", "tech", "One", 100, 800),
		item("b", "f", "tech", "Two", 1, 800),
		item("c", "f", "tech", "Three", 50, 800),
	}
	got := r.Rank(items, meta(catalog.Normal, catalog.Current), now)
	if len(got) != 3 {
		t.Fatalf("expected 3 results, got %d", len(got))
	}
	for i := 1; i < len(got); i++ {
		if got[i-1].Score < got[i].Score {
			t.Fatalf("results not ordered by score: %+v", got)
		}
	}
	if got[0].Item.ID != "b" {
		t.Errorf("freshest should lead with no learning, got %q", got[0].Item.ID)
	}
}

func TestTokenize(t *testing.T) {
	got := Tokenize("The 2008 Financial Crisis, revisited -- and the 42 lessons THE")
	want := map[string]bool{"2008": true, "financial": true, "crisis": true, "revisited": true, "lessons": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want keys %v", got, want)
	}
	for _, g := range got {
		if !want[g] {
			t.Errorf("unexpected token %q in %v", g, got)
		}
	}
}

func weightOf(cs []Contribution, label string) float64 {
	for _, c := range cs {
		if c.Label == label {
			return c.Weight
		}
	}
	return 0
}
