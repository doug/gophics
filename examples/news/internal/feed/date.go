package feed

import (
	"strings"
	"time"
)

// dateLayouts covers the formats feeds actually emit, which is a considerably
// wider set than the RSS and Atom specifications permit.
var dateLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC3339,
	time.RFC3339Nano,
	time.RFC822Z,
	time.RFC822,
	time.ANSIC,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"Mon, 2 Jan 2006 15:04 -0700",
	"Mon, 2 Jan 2006 15:04:05",
	"Mon, 2 Jan 2006",
	"Mon, 02 Jan 2006 15:04:05 -0700",
	"Mon, 02 Jan 2006 15:04:05 MST",
	"Mon, 02 Jan 2006 15:04:05 -07:00",
	"2 Jan 2006 15:04:05 -0700",
	"2 Jan 2006 15:04:05 MST",
	"02 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05-07:00",
	"2006-01-02T15:04:05Z",
	"2006-01-02T15:04:05.000Z",
	"2006-01-02T15:04:05",
	"2006-01-02T15:04Z",
	"2006-01-02 15:04:05 -0700",
	"2006-01-02 15:04:05",
	"2006-01-02 15:04",
	"2006-01-02",
	"2006/01/02",
	"January 2, 2006",
	"January 2, 2006 15:04",
	"Jan 2, 2006",
	"20060102",
}

// ParseDate parses a feed timestamp. It reports whether parsing succeeded so
// callers can distinguish "no date" from "the zero time".
func ParseDate(s string) (time.Time, bool) {
	s = cleanText(s)
	if s == "" {
		return time.Time{}, false
	}
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	// Some feeds append a redundant parenthesised zone: "... -0700 (PDT)".
	if i := strings.IndexByte(s, '('); i > 0 {
		if t, ok := ParseDate(strings.TrimSpace(s[:i])); ok {
			return t, true
		}
	}
	// Others use a bare "GMT+00:00"-style suffix or stray trailing text; try
	// progressively shorter prefixes at word boundaries.
	fields := strings.Fields(s)
	for n := len(fields) - 1; n >= 3; n-- {
		if t, ok := parseExact(strings.Join(fields[:n], " ")); ok {
			return t, true
		}
	}
	return time.Time{}, false
}

func parseExact(s string) (time.Time, bool) {
	for _, layout := range dateLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
