package rank

import (
	"encoding/json"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/store"
)

// Meta supplies the catalog facts about a feed that the cold-start prior needs.
// It is a function rather than a catalog pointer so the ranker stays testable
// and does not care whether a feed came from the suggestions or was added by
// hand.
type Meta func(feedID string) (catalog.Priority, catalog.Kind)

// Scored is an article with its predicted likelihood, in 0..1.
type Scored struct {
	Item  *store.Item
	Score float64
}

// Ranker is the concurrency-safe, persistent front end to Model. The UI holds
// one and calls it from the frame goroutine, while refreshes touch it from
// background goroutines.
type Ranker struct {
	mu    sync.Mutex
	m     *Model
	path  string
	dirty bool
}

// Open loads the model from path, starting fresh if it is missing or corrupt.
// A damaged ranking file costs some learning; it must never stop the reader
// from opening, which is why there is no error to handle here.
func Open(path string) *Ranker {
	r := &Ranker{m: New(), path: path}
	if data, err := os.ReadFile(path); err == nil {
		var m Model
		if json.Unmarshal(data, &m) == nil {
			m.ensure()
			r.m = &m
		}
	}
	return r
}

// Save writes the model if anything changed.
func (r *Ranker) Save() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.dirty || r.path == "" {
		return nil
	}
	data, err := json.Marshal(r.m)
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(tmp, r.path); err != nil {
		return err
	}
	r.dirty = false
	return nil
}

// Observe records a signal and marks the model for saving.
func (r *Ranker) Observe(it *store.Item, s Signal) {
	r.mu.Lock()
	r.m.Observe(it, s)
	r.dirty = true
	r.mu.Unlock()
}

// Impression records that an article was shown; see Model.Impression.
// The running count has to be persisted, not only the skip it eventually
// produces. Marking the model dirty just when the threshold was crossed left
// the tally in memory, so every run of the app started it from zero and an
// article could be ignored indefinitely without the model ever noticing — which
// is the entire purpose of counting impressions.
func (r *Ranker) Impression(it *store.Item) {
	r.mu.Lock()
	if !r.m.decided(it.ID) {
		r.m.Impression(it)
		r.dirty = true
	}
	r.mu.Unlock()
}

// Score is the likelihood for one article.
func (r *Ranker) Score(it *store.Item, meta Meta, now time.Time) float64 {
	pri, kind := lookup(meta, it.FeedID)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m.Score(it, pri, kind, now)
}

// Explain returns the terms behind an article's score, largest first.
func (r *Ranker) Explain(it *store.Item, meta Meta, now time.Time) []Contribution {
	pri, kind := lookup(meta, it.FeedID)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m.Explain(it, pri, kind, now)
}

// Rank scores every article and returns them best first. Ties break on
// publication date so the order is stable between rebuilds of the queue —
// nothing is more disorienting than a list that reshuffles under your thumb.
func (r *Ranker) Rank(items []*store.Item, meta Meta, now time.Time) []Scored {
	out := make([]Scored, 0, len(items))
	r.mu.Lock()
	for _, it := range items {
		pri, kind := lookup(meta, it.FeedID)
		out = append(out, Scored{Item: it, Score: r.m.Score(it, pri, kind, now)})
	}
	r.mu.Unlock()

	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].Item.Published.After(out[j].Item.Published)
	})
	return out
}

// Trained reports how much evidence the model holds, so the UI can be honest
// about a ranking that is still mostly the catalog's opinion.
func (r *Ranker) Trained() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.m.evidence()
}

// Reset forgets everything learned. Offered in settings because a model that
// has drifted somewhere wrong is otherwise very hard to argue with.
func (r *Ranker) Reset() {
	r.mu.Lock()
	r.m = New()
	r.dirty = true
	r.mu.Unlock()
	r.Save()
}

func lookup(meta Meta, feedID string) (catalog.Priority, catalog.Kind) {
	if meta == nil {
		return 0, ""
	}
	return meta(feedID)
}
