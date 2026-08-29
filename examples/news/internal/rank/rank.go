// Package rank scores unread articles by how likely you are to want to read
// them, learning from what you actually do rather than from what you say.
//
// The model is a naive Bayes classifier over four kinds of evidence — the feed,
// the category, the author, and the words in the title and summary — combined
// in log-odds with a length preference and a recency term. It is deliberately
// small and inspectable: every score decomposes into named contributions, which
// is what Explain returns and what the UI shows when you ask why something is
// near the top.
//
// Two properties matter more than accuracy here:
//
// Cold start must be sensible. With no history the score falls back to the
// catalog's own editorial weight, so the first queue is ordered by must-read
// and freshness rather than by noise. Learned evidence displaces that prior
// gradually, in proportion to how much of it there is.
//
// Nothing may run away. Every feature's contribution is shrunk toward zero by
// how little evidence supports it and then clamped, so a single enthusiastic
// evening cannot convince the reader that one blog is the only thing worth
// reading.
package rank

import (
	"math"
	"sort"
	"strings"
	"time"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/store"
)

// Signal is something the reader observed you doing.
type Signal int

const (
	// Skipped: the article was on screen in the queue and you moved past it
	// repeatedly without opening it, or you swiped it away.
	Skipped Signal = iota
	// Opened: you tapped in. Weak evidence — a good headline earns this.
	Opened
	// Finished: you read to the end, or dwelled long enough that you must have.
	// This is the signal the model most wants.
	Finished
	// ThumbDown / ThumbUp: you said so directly.
	ThumbDown
	ThumbUp
)

// weight is how much each signal counts, and toward which class. Explicit
// judgements outweigh behaviour because they are rarer and unambiguous;
// finishing outweighs opening because a headline can win a tap on its own.
func (s Signal) weight() (pos, neg float64) {
	switch s {
	case Skipped:
		return 0, 1
	case Opened:
		return 1, 0
	case Finished:
		return 2.5, 0
	case ThumbDown:
		return 0, 6
	case ThumbUp:
		return 6, 0
	}
	return 0, 0
}

// counts is the positive/negative evidence tallied for one feature value.
type counts struct {
	Pos float64 `json:"p"`
	Neg float64 `json:"n"`
}

func (c counts) total() float64 { return c.Pos + c.Neg }

// Model holds everything learned. It is a plain value so it serialises to one
// small JSON file; there is no database and nothing leaves the phone.
type Model struct {
	Feeds      map[string]counts `json:"feeds"`
	Categories map[string]counts `json:"categories"`
	Authors    map[string]counts `json:"authors"`
	Tokens     map[string]counts `json:"tokens"`

	// TotalPos and TotalNeg are the class priors.
	TotalPos float64 `json:"total_pos"`
	TotalNeg float64 `json:"total_neg"`

	// Length evidence: a running mean and variance of log reading-minutes for
	// articles you finished, so the model can learn that you finish essays but
	// abandon 40-minute reports (or the reverse) without anyone declaring it.
	LenN    float64 `json:"len_n"`
	LenSum  float64 `json:"len_sum"`
	LenSum2 float64 `json:"len_sum2"`

	// Seen counts how many times each article has been shown without being
	// opened. It is what turns "kept scrolling past it" into a Skipped signal
	// exactly once, rather than every time the queue is rebuilt.
	Seen map[string]int `json:"seen,omitempty"`

	// Decided remembers articles already folded into the tallies, so reopening
	// something does not count it twice.
	Decided map[string]bool `json:"decided,omitempty"`
}

// New returns an empty model.
func New() *Model {
	return &Model{
		Feeds:      map[string]counts{},
		Categories: map[string]counts{},
		Authors:    map[string]counts{},
		Tokens:     map[string]counts{},
		Seen:       map[string]int{},
		Decided:    map[string]bool{},
	}
}

// ensure fills in maps on a model that came from JSON written by an older
// version, or from an empty file.
func (m *Model) ensure() {
	if m.Feeds == nil {
		m.Feeds = map[string]counts{}
	}
	if m.Categories == nil {
		m.Categories = map[string]counts{}
	}
	if m.Authors == nil {
		m.Authors = map[string]counts{}
	}
	if m.Tokens == nil {
		m.Tokens = map[string]counts{}
	}
	if m.Seen == nil {
		m.Seen = map[string]int{}
	}
	if m.Decided == nil {
		m.Decided = map[string]bool{}
	}
}

// Observe folds one signal about one article into the model.
func (m *Model) Observe(it *store.Item, s Signal) {
	m.ensure()
	pos, neg := s.weight()
	if pos == 0 && neg == 0 {
		return
	}

	add := func(mp map[string]counts, key string) {
		if key == "" {
			return
		}
		c := mp[key]
		c.Pos += pos
		c.Neg += neg
		mp[key] = c
	}

	add(m.Feeds, it.FeedID)
	add(m.Categories, it.Category)
	add(m.Authors, normalizeAuthor(it.Author))
	for _, t := range Tokenize(it.Title + " " + it.Summary) {
		add(m.Tokens, t)
	}
	m.TotalPos += pos
	m.TotalNeg += neg

	// Only a completed read tells us anything about length: an article you
	// abandoned says nothing about how long you like them, since you never
	// found out how long it was.
	if s == Finished || s == ThumbUp {
		if mins := float64(minutes(it)); mins > 0 {
			l := math.Log(mins)
			m.LenN++
			m.LenSum += l
			m.LenSum2 += l * l
		}
	}
	m.Decided[it.ID] = true
	delete(m.Seen, it.ID)
}

// Impression records that an article was displayed in the queue. After enough
// impressions with no open, the reader treats that as a skip — the passive
// signal that keeps a feed you never actually read from sitting near the top
// forever. It returns true when a Skipped signal was recorded.
func (m *Model) Impression(it *store.Item) bool {
	m.ensure()
	if m.Decided[it.ID] {
		return false
	}
	m.Seen[it.ID]++
	if m.Seen[it.ID] < skipAfterImpressions {
		return false
	}
	m.Observe(it, Skipped)
	return true
}

// decided reports whether an article has already been folded into the tallies,
// so a caller can tell an impression that will change something from one that
// will not.
func (m *Model) decided(id string) bool { return m.Decided[id] }

// skipAfterImpressions is how many times an article may be scrolled past before
// that counts against it. Three is enough to survive a distracted glance and
// short enough to learn from a week of reading.
const skipAfterImpressions = 3

// Score returns the probability, in 0..1, that this is an article you want to
// read next.
func (m *Model) Score(it *store.Item, pri catalog.Priority, kind catalog.Kind, now time.Time) float64 {
	return sigmoid(m.logOdds(it, pri, kind, now).total())
}

// Contribution is one named term of a score, in log-odds. Positive pushes the
// article up the queue.
type Contribution struct {
	Label  string
	Detail string
	Weight float64
}

type breakdown []Contribution

func (b breakdown) total() float64 {
	var s float64
	for _, c := range b {
		s += c.Weight
	}
	return s
}

// Explain returns the score's terms, largest magnitude first. This is what the
// UI shows behind "why is this here", and what makes a wrong ranking something
// you can see the cause of rather than something you have to fight.
func (m *Model) Explain(it *store.Item, pri catalog.Priority, kind catalog.Kind, now time.Time) []Contribution {
	b := m.logOdds(it, pri, kind, now)
	sort.SliceStable(b, func(i, j int) bool {
		return math.Abs(b[i].Weight) > math.Abs(b[j].Weight)
	})
	return b
}

func (m *Model) logOdds(it *store.Item, pri catalog.Priority, kind catalog.Kind, now time.Time) breakdown {
	m.ensure()
	var b breakdown

	// The editorial prior. Its influence fades as real evidence accumulates, so
	// day one is ordered by the catalog and month two is ordered by you.
	w := priorWeight(m.evidence())
	if p := editorialPrior(pri, kind); p != 0 && w > 0 {
		b = append(b, Contribution{"Source rating", editorialLabel(pri, kind), p * w})
	}

	if c, ok := m.Feeds[it.FeedID]; ok {
		if v := llr(c, m.TotalPos, m.TotalNeg); v != 0 {
			b = append(b, Contribution{"Feed", it.FeedName, clamp(v, 2.0)})
		}
	}
	if c, ok := m.Categories[it.Category]; ok {
		if v := llr(c, m.TotalPos, m.TotalNeg); v != 0 {
			b = append(b, Contribution{"Category", it.Category, clamp(v, 1.5)})
		}
	}
	if a := normalizeAuthor(it.Author); a != "" {
		if c, ok := m.Authors[a]; ok {
			if v := llr(c, m.TotalPos, m.TotalNeg); v != 0 {
				b = append(b, Contribution{"Author", it.Author, clamp(v, 1.5)})
			}
		}
	}

	if v, words := m.tokenLLR(it); v != 0 {
		b = append(b, Contribution{"Topic", strings.Join(words, ", "), clamp(v, 2.5)})
	}
	if v, label := m.lengthLLR(it); v != 0 {
		b = append(b, Contribution{"Length", label, v})
	}
	if v, label := recency(it, now); v != 0 {
		b = append(b, Contribution{"Freshness", label, v})
	}
	return b
}

// evidence is how much the model has learned, in signal weight.
func (m *Model) evidence() float64 { return m.TotalPos + m.TotalNeg }

// priorWeight fades the catalog's editorial rating out as behaviour arrives.
// At zero evidence it is 1; by roughly forty signals it is a third.
func priorWeight(evidence float64) float64 {
	return 20 / (20 + evidence)
}

func editorialPrior(pri catalog.Priority, kind catalog.Kind) float64 {
	var v float64
	switch pri {
	case catalog.MustRead:
		v += 0.9
	case catalog.Filler:
		v -= 0.7
	}
	switch kind {
	case catalog.Compounding:
		v += 0.45
	case catalog.Decision:
		v += 0.3
	case catalog.Leisure:
		v += 0.1
	}
	return v
}

func editorialLabel(pri catalog.Priority, kind catalog.Kind) string {
	var parts []string
	switch pri {
	case catalog.MustRead:
		parts = append(parts, "must-read")
	case catalog.Filler:
		parts = append(parts, "filler")
	}
	if kind != "" {
		parts = append(parts, string(kind))
	}
	if len(parts) == 0 {
		return "unrated"
	}
	return strings.Join(parts, ", ")
}

// tokenLLR scores the title and summary words, and names the few that mattered
// most so the explanation can say "because: kubernetes, latency" rather than
// producing a number nobody can check.
func (m *Model) tokenLLR(it *store.Item) (float64, []string) {
	toks := Tokenize(it.Title + " " + it.Summary)
	if len(toks) == 0 {
		return 0, nil
	}
	type tv struct {
		tok string
		v   float64
	}
	var seen []tv
	var sum float64
	for _, t := range toks {
		c, ok := m.Tokens[t]
		if !ok {
			continue
		}
		v := llr(c, m.TotalPos, m.TotalNeg)
		if v == 0 {
			continue
		}
		sum += v
		seen = append(seen, tv{t, v})
	}
	if len(seen) == 0 {
		return 0, nil
	}
	// Normalise by the square root of how many words carried evidence. Dividing
	// by the count would make a single strong word beat a headline full of
	// moderately good ones; not dividing at all would let long summaries
	// outscore short ones purely by length.
	sum /= math.Sqrt(float64(len(seen)))

	sort.SliceStable(seen, func(i, j int) bool {
		return math.Abs(seen[i].v) > math.Abs(seen[j].v)
	})
	words := make([]string, 0, 3)
	for _, s := range seen[:min(3, len(seen))] {
		words = append(words, s.tok)
	}
	return sum, words
}

// lengthLLR rewards articles near the length you actually finish. It needs a
// handful of completed reads before it says anything at all.
func (m *Model) lengthLLR(it *store.Item) (float64, string) {
	const minReads = 5
	if m.LenN < minReads {
		return 0, ""
	}
	mins := float64(minutes(it))
	if mins <= 0 {
		return 0, ""
	}
	mean := m.LenSum / m.LenN
	varr := m.LenSum2/m.LenN - mean*mean
	if varr < 0.05 {
		varr = 0.05 // never become so confident that one length is the only one
	}
	z := (math.Log(mins) - mean) / math.Sqrt(varr)
	// A gentle bell: +0.5 at the preferred length, tapering to a small penalty
	// two standard deviations out.
	v := 0.5 * (1 - z*z/2)
	v = clamp(v, 0.6)
	label := "about the length you finish"
	if z > 1 {
		label = "longer than you usually finish"
	} else if z < -1 {
		label = "shorter than you usually finish"
	}
	return v, label
}

// recency keeps perishable news near the top and lets it fall away. Half-life
// is a day and a half, which is roughly how long a news item stays worth
// reading; the floor stops an old essay from being buried by arithmetic.
func recency(it *store.Item, now time.Time) (float64, string) {
	age := max(now.Sub(it.Published), 0)
	hours := age.Hours()
	v := 0.8*math.Exp(-hours/36) - 0.3
	switch {
	case hours < 3:
		return v, "just published"
	case hours < 24:
		return v, "today"
	case hours < 48:
		return v, "yesterday"
	default:
		return v, "older"
	}
}

// llr is how much better — or worse — one feature value does than the reader
// does on average, in log-odds.
//
// The textbook naive Bayes ratio, log(p(v|read) / p(v|skipped)), is wrong for
// this job, and wrong in a way that only shows up early. Both terms are
// normalised by their class total, so a feed that accounts for *all* of the
// positive evidence has p(v|read) = 1, and with no negative evidence anywhere
// p(v|skipped) smooths to 1 as well: liking three articles from one blog moves
// its score by exactly nothing. That is the state every new install is in.
//
// Comparing against the global rate instead is well behaved from the first
// signal. Each value's odds are smoothed toward the base rate rather than
// toward an even split, so an unseen value scores zero, a value that only ever
// gets read scores positive however lopsided the data, and a value merely rarer
// than average is not mistaken for one that is disliked.
func llr(c counts, totalPos, totalNeg float64) float64 {
	// alpha is how many pseudo-observations of average behaviour every value
	// starts with. Two is enough that a single click cannot swing a feed.
	const alpha = 2.0

	base := (totalPos + 1) / (totalPos + totalNeg + 2) // global read rate
	priorOdds := base / (1 - base)

	odds := (c.Pos + alpha*base) / (c.Neg + alpha*(1-base))
	if odds <= 0 || priorOdds <= 0 {
		return 0
	}
	v := math.Log(odds / priorOdds)
	n := c.total()
	return v * (n / (n + 3)) // shrink toward zero while evidence is thin
}

func clamp(v, lim float64) float64 {
	return math.Max(-lim, math.Min(lim, v))
}

func sigmoid(x float64) float64 { return 1 / (1 + math.Exp(-x)) }

// minutes is reading time at 220 words per minute, floored at one so every
// article has a length.
func minutes(it *store.Item) int {
	if it.WordCount <= 0 {
		return 0
	}
	m := max((it.WordCount+219)/220, 1)
	return m
}

func normalizeAuthor(a string) string {
	a = strings.ToLower(strings.TrimSpace(a))
	if len(a) > 60 {
		return ""
	}
	return a
}
