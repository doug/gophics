// Package catalog loads and queries the curated feed catalog.
package catalog

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Fulltext classifies how much article body a feed ships of its own accord.
type Fulltext string

const (
	FullText Fulltext = "full"    // >= 4000 chars of body text
	Partial  Fulltext = "partial" // 800-3999
	Teaser   Fulltext = "teaser"  // < 800, extraction strongly advised
	Unknown  Fulltext = "unknown"
)

// Feed is one catalog entry.
type Feed struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	URL      string   `json:"url"`
	Category string   `json:"category"`
	Tags     []string `json:"tags,omitempty"`
	Fulltext Fulltext `json:"fulltext,omitempty"`
	Notes    string   `json:"notes,omitempty"`

	// Priority is the editorial weight used when an issue has less room than
	// there is material. Zero means unrated and is treated as Normal.
	Priority Priority `json:"priority,omitempty"`

	// Kind is what sort of value this source delivers: compounding, decision,
	// current or leisure. It is what makes it possible to build an issue out of
	// material worth the time rather than material that merely arrived.
	Kind Kind `json:"kind,omitempty"`

	// Enabled defaults to true. Use a pointer so an absent field differs from
	// an explicit false.
	Enabled *bool `json:"enabled,omitempty"`

	// Extract overrides whether the fetcher should follow the link and pull the
	// article body. When nil the decision is made from Fulltext: teaser and
	// unknown feeds are extracted, full ones are not. Set false for feeds where
	// the summary *is* the content (arXiv abstracts, bioRxiv).
	Extract *bool `json:"extract,omitempty"`

	// UserAgent overrides the default UA for hosts that reject browser-like
	// strings.
	UserAgent string `json:"user_agent,omitempty"`

	// MinIntervalSeconds overrides the per-host delay for sources that demand a
	// slower pace than the default. The arXiv API asks for three seconds.
	MinIntervalSeconds int `json:"min_interval_seconds,omitempty"`

	// CookieFile points at a Netscape cookies.txt export. It lets a paid
	// subscription be used to retrieve article bodies from publishers that put
	// only teasers in their feed, such as The Economist.
	CookieFile string `json:"cookie_file,omitempty"`

	// ArticleUserAgent overrides the UA used when fetching a linked article, as
	// distinct from the feed itself.
	ArticleUserAgent string `json:"article_user_agent,omitempty"`
}

// MinInterval returns the configured per-host delay, or zero for the default.
func (f Feed) MinInterval() time.Duration {
	if f.MinIntervalSeconds <= 0 {
		return 0
	}
	return time.Duration(f.MinIntervalSeconds) * time.Second
}

// IsEnabled reports whether the feed should be polled.
func (f Feed) IsEnabled() bool { return f.Enabled == nil || *f.Enabled }

// ShouldExtract reports whether the fetcher should fetch and parse the linked
// article rather than trusting the feed body.
func (f Feed) ShouldExtract() bool {
	if f.Extract != nil {
		return *f.Extract
	}
	switch f.Fulltext {
	case FullText:
		return false
	default:
		return true
	}
}

// Unavailable records a source we looked for and could not use, so the
// knowledge is not lost and can be revisited.
type Unavailable struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

// Catalog is the whole curated collection.
type Catalog struct {
	Version  int       `json:"version"`
	Comment  string    `json:"comment,omitempty"`
	Feeds    []Feed    `json:"feeds"`
	Editions []Edition `json:"editions,omitempty"`

	// SkipTitles are case-insensitive patterns matching recurring posts that are
	// not articles: open threads, link round-ups, site notices. They come from
	// sources worth reading, which is why they cannot simply be deprioritised.
	SkipTitles  []string      `json:"skip_titles,omitempty"`
	Unavailable []Unavailable `json:"unavailable,omitempty"`
}

// Load reads a catalog from disk and validates it.
func Load(path string) (*Catalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Catalog
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

func (c *Catalog) validate() error {
	seen := make(map[string]bool, len(c.Feeds))
	for i, f := range c.Feeds {
		switch {
		case f.ID == "":
			return fmt.Errorf("feed %d: missing id", i)
		case f.URL == "":
			return fmt.Errorf("feed %q: missing url", f.ID)
		case f.Category == "":
			return fmt.Errorf("feed %q: missing category", f.ID)
		case seen[f.ID]:
			return fmt.Errorf("duplicate feed id %q", f.ID)
		}
		seen[f.ID] = true
		if f.Priority != 0 && (f.Priority < Filler || f.Priority > MustRead) {
			return fmt.Errorf("feed %q: priority %d out of range (1-3)", f.ID, f.Priority)
		}
		if f.Kind != "" {
			var ok bool
			if slices.Contains(KnownKinds(), f.Kind) {
				ok = true
			}
			if !ok {
				return fmt.Errorf("feed %q: unknown kind %q", f.ID, f.Kind)
			}
		}
	}

	for _, pat := range c.SkipTitles {
		if _, err := regexp.Compile("(?i)" + pat); err != nil {
			return fmt.Errorf("skip_titles: invalid pattern %q: %w", pat, err)
		}
	}

	editions := make(map[string]bool, len(c.Editions))
	for i, e := range c.Editions {
		switch {
		case e.ID == "":
			return fmt.Errorf("edition %d: missing id", i)
		case editions[e.ID]:
			return fmt.Errorf("duplicate edition id %q", e.ID)
		case e.Minutes < 0:
			return fmt.Errorf("edition %q: negative minutes", e.ID)
		case e.MaxCategoryShare < 0 || e.MaxCategoryShare > 1:
			return fmt.Errorf("edition %q: max_category_share %v must be between 0 and 1",
				e.ID, e.MaxCategoryShare)
		}
		editions[e.ID] = true

		switch e.EffectiveLayout() {
		case Articles, Scan:
		default:
			return fmt.Errorf("edition %q: unknown layout %q (want articles or scan)", e.ID, e.Layout)
		}
		if e.Since != "" {
			if _, err := ParseWindow(e.Since); err != nil {
				return fmt.Errorf("edition %q: %w", e.ID, err)
			}
		}
		// A misspelled feed id in an edition would silently narrow the issue.
		for _, id := range append(append([]string{}, e.Feeds...), e.ExcludeFeeds...) {
			if _, ok := c.ByID(id); !ok {
				return fmt.Errorf("edition %q references unknown feed %q", e.ID, id)
			}
		}
		for _, cat := range e.Categories {
			if !containsFold(c.Categories(), cat) {
				return fmt.Errorf("edition %q references unknown category %q", e.ID, cat)
			}
		}
		for _, k := range e.Kinds {
			var ok bool
			if slices.Contains(KnownKinds(), k) {
				ok = true
			}
			if !ok {
				return fmt.Errorf("edition %q references unknown kind %q", e.ID, k)
			}
		}
	}
	return nil
}

// TitleSkipper compiles SkipTitles into a predicate. It reports true for titles
// that should never reach an issue. A catalog with no patterns yields nil, which
// callers treat as "skip nothing".
func (c *Catalog) TitleSkipper() func(string) bool {
	if len(c.SkipTitles) == 0 {
		return nil
	}
	res := make([]*regexp.Regexp, 0, len(c.SkipTitles))
	for _, pat := range c.SkipTitles {
		if re, err := regexp.Compile("(?i)" + pat); err == nil {
			res = append(res, re)
		}
	}
	return func(title string) bool {
		title = strings.TrimSpace(title)
		for _, re := range res {
			if re.MatchString(title) {
				return true
			}
		}
		return false
	}
}

// equalFold is a spelling of case-insensitive comparison local to this package.
func equalFold(a, b string) bool { return strings.EqualFold(a, b) }

// ParseWindow parses a lookback window: Go durations plus day, week and year
// suffixes. It lives here so the catalog can validate what it stores without
// depending on the command-line package.
func ParseWindow(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	var mult time.Duration
	switch s[len(s)-1] {
	case 'd':
		mult = 24 * time.Hour
	case 'w':
		mult = 7 * 24 * time.Hour
	case 'y':
		mult = 365 * 24 * time.Hour
	}
	if mult > 0 {
		n, err := strconv.ParseFloat(s[:len(s)-1], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid window %q (try 7d, 12h or 2w)", s)
		}
		return time.Duration(n * float64(mult)), nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid window %q (try 7d, 12h or 2w)", s)
	}
	return d, nil
}

// Selector filters the catalog.
type Selector struct {
	Categories []string // match any
	Tags       []string // match any
	IDs        []string // match any; exact
	All        bool     // include disabled feeds
}

// Select returns the feeds matching the selector, in catalog order.
func (c *Catalog) Select(s Selector) []Feed {
	var out []Feed
	for _, f := range c.Feeds {
		if !s.All && !f.IsEnabled() {
			continue
		}
		if len(s.IDs) > 0 && !containsFold(s.IDs, f.ID) {
			continue
		}
		if len(s.Categories) > 0 && !containsFold(s.Categories, f.Category) {
			continue
		}
		if len(s.Tags) > 0 && !anyFold(s.Tags, f.Tags) {
			continue
		}
		out = append(out, f)
	}
	return out
}

// Categories returns the distinct categories present, sorted.
func (c *Catalog) Categories() []string {
	set := map[string]bool{}
	for _, f := range c.Feeds {
		set[f.Category] = true
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ByID returns the feed with the given id.
func (c *Catalog) ByID(id string) (Feed, bool) {
	for _, f := range c.Feeds {
		if strings.EqualFold(f.ID, id) {
			return f, true
		}
	}
	return Feed{}, false
}

func containsFold(hay []string, needle string) bool {
	for _, h := range hay {
		if strings.EqualFold(h, needle) {
			return true
		}
	}
	return false
}

func anyFold(want, have []string) bool {
	for _, w := range want {
		if containsFold(have, w) {
			return true
		}
	}
	return false
}

// SplitList splits a comma-separated flag value, dropping empties.
func SplitList(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
