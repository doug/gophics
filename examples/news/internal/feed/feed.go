// Package feed parses syndication feeds: RSS 2.0, RSS 1.0/RDF and Atom 1.0.
//
// Real-world feeds are not well-formed XML as often as one would hope, so the
// parser is deliberately lenient: it tolerates undeclared entities, mismatched
// tags, byte-order marks, junk before the XML declaration and legacy charsets.
// A feed that yields zero items is not an error — daily-snapshot feeds such as
// arXiv's legitimately serve an empty channel.
package feed

import (
	"strings"
	"time"
)

// Feed is a parsed syndication document.
type Feed struct {
	Title       string
	Link        string
	Description string
	Updated     time.Time
	Format      string // "rss", "atom" or "rdf"
	Items       []Item
}

// Item is a single entry within a feed.
type Item struct {
	// GUID is the feed's own stable identifier for the entry. It is used for
	// deduplication and falls back to the link, then to a content hash.
	GUID       string
	Title      string
	Link       string
	Author     string
	Published  time.Time
	Updated    time.Time
	Summary    string // short teaser, plain-ish text
	Content    string // richest body HTML the feed offered
	Categories []string
	Enclosures []Enclosure
}

// Enclosure is an attached media file (podcast audio, lead image, ...).
type Enclosure struct {
	URL    string
	Type   string
	Length int64
}

// Date returns the best available timestamp for the item.
func (i Item) Date() time.Time {
	if !i.Published.IsZero() {
		return i.Published
	}
	return i.Updated
}

// TextLen reports the length of Content with HTML tags removed. It is the
// signal used to decide whether a feed shipped a real article or a teaser.
func (i Item) TextLen() int {
	return len(strings.TrimSpace(StripTags(i.Content)))
}
