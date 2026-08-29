// Package pick chooses which stored articles go into an issue.
//
// Selection is a budget problem, not a counting problem. Word counts in a real
// catalog range from a dozen to nearly thirty thousand, so "the newest twelve
// articles" produces an issue somewhere between five minutes and five hours
// long. Everything here works in reading minutes instead, and spends that budget
// according to editorial weight rather than publication volume.
package pick

import (
	"sort"
	"time"

	"github.com/doug/gophics/examples/news/internal/catalog"
	"github.com/doug/gophics/examples/news/internal/store"
)

// WordsPerMinute is the prose reading pace used to convert length into time.
const WordsPerMinute = 230

// Minutes returns an item's reading time, rounded up so nothing costs zero.
func Minutes(it *store.Item) int {
	if it.WordCount <= 0 {
		return 1
	}
	return (it.WordCount + WordsPerMinute - 1) / WordsPerMinute
}

// Params are the selection rules, normally taken from an edition.
type Params struct {
	// Minutes is the reading budget. Zero means unbounded.
	Minutes int

	// MaxItemMinutes diverts anything longer to Overflow, so one very long
	// essay cannot swallow an entire issue.
	MaxItemMinutes int

	// PerFeed caps items from any one feed. Zero means unlimited.
	PerFeed int

	// MaxCategoryShare caps a category's share of the budget, 0 to 1. Zero
	// means unlimited. Without it, whichever category writes at greatest length
	// dominates every issue.
	MaxCategoryShare float64

	// MinPriority drops feeds rated below this.
	MinPriority catalog.Priority

	// AllowSummaryOnly keeps items whose stored body is only a teaser.
	AllowSummaryOnly bool

	// SkipTitle rejects titles that are not articles at all. Nil skips nothing.
	SkipTitle func(string) bool
}

// Result is the outcome of a selection.
type Result struct {
	// Selected is the issue, in the order it should be read.
	Selected []*store.Item

	// Overflow holds items too long for this issue, in the same newest-first
	// order as the input. They are not discarded: the caller publishes them
	// separately, normally one at a time, so a very long essay becomes a
	// deliberate single read rather than an entire issue.
	Overflow []*store.Item

	// Deferred are items that fit the rules but not the remaining budget or a
	// quota. They stay unread and will be offered again next time.
	Deferred []*store.Item

	// Excluded are items the rules reject outright, with the reason.
	Excluded map[string]int

	Minutes int
}

// PriorityFunc reports a feed's editorial weight.
type PriorityFunc func(feedID string) catalog.Priority

// Select applies the rules to a candidate set. Items are assumed to be sorted
// newest first; the returned issue preserves the chosen reading order.
func Select(items []*store.Item, priority PriorityFunc, p Params) Result {
	res := Result{Excluded: map[string]int{}}
	if priority == nil {
		priority = func(string) catalog.Priority { return catalog.Normal }
	}

	type scored struct {
		item  *store.Item
		prio  catalog.Priority
		mins  int
		score float64
	}

	var eligible []scored
	for _, it := range items {
		prio := priority(it.FeedID)

		if p.MinPriority != 0 && prio < p.MinPriority {
			res.Excluded["below minimum priority"]++
			continue
		}
		// A teaser is a headline and a dead end; it reads as a defect in an issue.
		if !p.AllowSummaryOnly && it.Source == store.SourceSummary {
			res.Excluded["summary only, no article body"]++
			continue
		}
		if p.SkipTitle != nil && p.SkipTitle(it.Title) {
			res.Excluded["not an article (open thread, link round-up, notice)"]++
			continue
		}

		mins := Minutes(it)
		if p.MaxItemMinutes > 0 && mins > p.MaxItemMinutes {
			res.Overflow = append(res.Overflow, it)
			continue
		}

		eligible = append(eligible, scored{item: it, prio: prio, mins: mins, score: score(it, prio)})
	}

	sort.SliceStable(eligible, func(i, j int) bool { return eligible[i].score > eligible[j].score })

	var (
		spent     int
		perFeed   = map[string]int{}
		perCat    = map[string]int{}
		catBudget = 0
	)
	if p.Minutes > 0 && p.MaxCategoryShare > 0 {
		catBudget = max(int(float64(p.Minutes)*p.MaxCategoryShare), 1)
	}

	for _, e := range eligible {
		switch {
		case p.PerFeed > 0 && perFeed[e.item.FeedID] >= p.PerFeed:
			res.Deferred = append(res.Deferred, e.item)
			continue
		case catBudget > 0 && perCat[e.item.Category]+e.mins > catBudget &&
			perCat[e.item.Category] > 0:
			// Allow a category's first item through regardless, so a quota can
			// never silence a category completely.
			res.Deferred = append(res.Deferred, e.item)
			continue
		case p.Minutes > 0 && spent+e.mins > p.Minutes && spent > 0:
			// Keep looking: a shorter item may still fit the remaining budget.
			res.Deferred = append(res.Deferred, e.item)
			continue
		}

		res.Selected = append(res.Selected, e.item)
		spent += e.mins
		perFeed[e.item.FeedID]++
		perCat[e.item.Category] += e.mins
	}

	res.Minutes = spent
	return res
}

// score ranks a candidate. Priority dominates, and recency orders items of equal
// weight — so a must-read source outranks a filler one, but a stale must-read
// eventually loses to something fresh.
func score(it *store.Item, prio catalog.Priority) float64 {
	const priorityWeight = 100
	days := time.Since(it.Published).Hours() / 24
	if days < 0 {
		days = 0 // a feed dated slightly in the future is not extra fresh
	}
	return float64(int(prio)*priorityWeight) - days
}

// ByCategoryMinutes summarises where an issue's reading time went, which is how
// you tell whether the quotas are doing anything useful.
func ByCategoryMinutes(items []*store.Item) map[string]int {
	out := map[string]int{}
	for _, it := range items {
		out[it.Category] += Minutes(it)
	}
	return out
}
