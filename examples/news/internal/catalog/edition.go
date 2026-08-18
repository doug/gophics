package catalog

import (
	"strings"
	"time"
)

// Priority is a feed's editorial weight, and the main lever over what reaches an
// issue. Selection is otherwise driven by whatever happened to publish most,
// which is not the same as what is worth reading.
type Priority int

const (
	Filler   Priority = 1 // high volume or low signal: include only to fill space
	Normal   Priority = 2 // the default
	MustRead Priority = 3 // read nearly everything this source publishes
)

// Kind is what sort of value a source delivers, which is a different question
// from how much the source is trusted. Priority decides who wins a contested
// issue; Kind decides which issue they belong in at all.
//
// The distinction exists because "important" and "improving" are not the same.
// A world news item can be both urgent and inert: reading it changes nothing the
// reader will do or understand. An essay on why a field is stuck is neither
// urgent nor perishable, and is the reason to read at all.
type Kind string

const (
	// Compounding builds durable models or skills: technical depth, methods,
	// analysis, essays that change how something is understood. Long half-life.
	Compounding Kind = "compounding"

	// Decision changes something the reader actually does — a zoning vote, a
	// school board decision, a gear or tool choice. Perishable but actionable.
	Decision Kind = "decision"

	// Current is awareness only. Worth skimming, rarely worth reading, and the
	// easiest category to spend a year of attention on by accident.
	Current Kind = "current"

	// Leisure is read for pleasure, which is a legitimate reason. Labelling it
	// honestly is better than dressing it up as improvement.
	Leisure Kind = "leisure"
)

// KnownKinds lists the valid values.
func KnownKinds() []Kind { return []Kind{Compounding, Decision, Current, Leisure} }

// Layout selects how an edition presents its items.
type Layout string

const (
	// Articles renders one chapter per item, for things that are read.
	Articles Layout = "articles"
	// Scan renders a single chapter listing every item, for things that are
	// skimmed. Sixty arXiv abstracts are one scan list, not sixty chapters.
	Scan Layout = "scan"
)

// Edition is a named, right-sized issue: which sources, how far back, and how
// much reading time. Editions exist because a catalog of 146 feeds is not one
// publication but several with different rhythms, and a single file mixing a
// two-minute link post with a two-hour essay fits no reading session.
type Edition struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`

	// Source selection. Empty means no constraint on that axis.
	Categories   []string `json:"categories,omitempty"`
	Tags         []string `json:"tags,omitempty"`
	Feeds        []string `json:"feeds,omitempty"`
	ExcludeFeeds []string `json:"exclude_feeds,omitempty"`

	// Kinds restricts the edition to sources delivering a given sort of value.
	// An edition of compounding sources is the one built to be worth the time.
	Kinds []Kind `json:"kinds,omitempty"`

	// ExcludeTags drops sources carrying any of these tags. Preprint abstracts
	// are compounding material, but they belong in a scan list rather than
	// interleaved with essays as though they were something to sit down with.
	ExcludeTags []string `json:"exclude_tags,omitempty"`

	// Since is how far back to look, in the same spelling the command line uses
	// ("1d", "7d", "36h").
	Since string `json:"since,omitempty"`

	// Minutes is the reading-time budget. It replaces an item count because word
	// counts in practice range from 11 to nearly 30,000.
	Minutes int `json:"minutes,omitempty"`

	// MaxItemMinutes routes anything longer into the overflow issue, so a single
	// very long essay cannot consume a whole edition.
	MaxItemMinutes int `json:"max_item_minutes,omitempty"`

	// PerFeed caps how many items one feed may contribute.
	PerFeed int `json:"per_feed,omitempty"`

	// MaxCategoryShare caps any one category's share of the budget, between 0
	// and 1. Without it the longest-winded category dominates every issue.
	MaxCategoryShare float64 `json:"max_category_share,omitempty"`

	// MinPriority drops feeds below this weight.
	MinPriority Priority `json:"min_priority,omitempty"`

	// MinWords drops items shorter than this. Excellent sources publish short
	// housekeeping posts, and because a short item is cheap against a minute
	// budget it is exactly what gets picked to fill the last few minutes.
	MinWords int `json:"min_words,omitempty"`

	// AllowSummaryOnly keeps items whose body is just a teaser. Off by default:
	// they read as a headline and a dead end.
	AllowSummaryOnly bool `json:"allow_summary_only,omitempty"`

	Layout  Layout `json:"layout,omitempty"`
	GroupBy string `json:"group_by,omitempty"`
}

// SinceDuration parses the edition's window, returning zero when unset.
func (e Edition) SinceDuration() (time.Duration, error) {
	if e.Since == "" {
		return 0, nil
	}
	return ParseWindow(e.Since)
}

// EffectiveLayout defaults to one chapter per article.
func (e Edition) EffectiveLayout() Layout {
	if e.Layout == "" {
		return Articles
	}
	return e.Layout
}

// EditionByID looks up an edition.
func (c *Catalog) EditionByID(id string) (Edition, bool) {
	for _, e := range c.Editions {
		if equalFold(e.ID, id) {
			return e, true
		}
	}
	return Edition{}, false
}

// EditionIDs lists the defined editions in catalog order.
func (c *Catalog) EditionIDs() []string {
	out := make([]string, 0, len(c.Editions))
	for _, e := range c.Editions {
		out = append(out, e.ID)
	}
	return out
}

// PriorityOf returns a feed's weight, defaulting to Normal for feeds that have
// not been rated.
func (c *Catalog) PriorityOf(feedID string) Priority {
	if f, ok := c.ByID(feedID); ok && f.Priority != 0 {
		return f.Priority
	}
	return Normal
}

// FeedsOfKind returns the ids of enabled feeds delivering any of the given kinds.
// Feeds with no kind recorded are omitted, so an unclassified source cannot
// quietly appear in an edition that was defined by value.
func (c *Catalog) FeedsOfKind(kinds []Kind) []string {
	if len(kinds) == 0 {
		return nil
	}
	want := make(map[Kind]bool, len(kinds))
	for _, k := range kinds {
		want[k] = true
	}
	var out []string
	for _, f := range c.Feeds {
		if !f.IsEnabled() || f.Kind == "" {
			continue
		}
		if want[f.Kind] {
			out = append(out, f.ID)
		}
	}
	return out
}

// ResolveFeeds returns the feed ids an edition should draw from: its explicit
// feeds plus everything matching its kinds, minus its exclusions. An empty
// result means "no feed-level restriction", leaving category and tag filters to
// do the work.
func (c *Catalog) ResolveFeeds(e Edition) []string {
	ids := append([]string{}, e.Feeds...)
	ids = append(ids, c.FeedsOfKind(e.Kinds)...)
	if len(ids) == 0 {
		return nil
	}

	drop := make(map[string]bool, len(e.ExcludeFeeds))
	for _, id := range e.ExcludeFeeds {
		drop[strings.ToLower(id)] = true
	}
	if len(e.ExcludeTags) > 0 {
		for _, f := range c.Feeds {
			if anyFold(e.ExcludeTags, f.Tags) {
				drop[strings.ToLower(f.ID)] = true
			}
		}
	}

	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		low := strings.ToLower(id)
		if drop[low] || seen[low] {
			continue
		}
		seen[low] = true
		out = append(out, id)
	}
	return out
}

// KindCounts summarises how the catalog is classified, which is how you notice
// that most of it is current-awareness.
func (c *Catalog) KindCounts() map[Kind]int {
	out := map[Kind]int{}
	for _, f := range c.Feeds {
		out[f.Kind]++
	}
	return out
}
