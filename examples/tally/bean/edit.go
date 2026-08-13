package bean

import (
	"strings"
)

// Source is a ledger file's text together with its parsed directives, edited by
// splicing lines.
//
// The guiding rule is that everything the user did not change comes back
// byte-for-byte. A beancount file is a hand-maintained document: section
// headings, a comment explaining why a transaction looks odd, blank lines
// grouping a month, the author's own alignment. Re-rendering the whole file from
// the AST would silently discard all of it. So an edit rewrites only the line
// span of the entry it touches, and inserts new entries between existing lines.
//
// That also keeps edits legible in version control, which is much of the reason
// people keep their books in a text file to begin with.
type Source struct {
	Path  string
	lines []string
	file  *File
}

// NewSource parses text and returns it ready for editing.
func NewSource(path, src string) (*Source, error) {
	f, err := Parse(path, src)
	if f == nil {
		return nil, err
	}
	return &Source{Path: path, lines: splitLines(src), file: f}, err
}

// Directives returns the parsed entries, in source order.
func (s *Source) Directives() []Directive { return s.file.Directives }

// String renders the current text.
func (s *Source) String() string { return strings.Join(s.lines, "\n") }

// Bytes renders the current text as bytes.
func (s *Source) Bytes() []byte { return []byte(s.String()) }

// Ledger processes the current text, so a caller can check an edit before saving.
func (s *Source) Ledger() (*Ledger, error) { return LoadString(s.Path, s.String()) }

// Insert places a transaction in date order and returns the line it landed on.
//
// It goes after the last entry dated on or before it, which keeps a
// chronologically-kept file chronological, and appends at the end of a file whose
// entries are all earlier. Insertion never reflows neighbouring entries.
func (s *Source) Insert(t *Transaction, opts FormatOptions) int {
	text := FormatTransaction(t, opts)
	at := s.insertLine(t.Date)

	block := splitLines(strings.TrimRight(text, "\n"))
	// Keep a blank line between entries, matching how ledgers are normally kept.
	if at > 0 && strings.TrimSpace(s.lines[at-1]) != "" {
		block = append([]string{""}, block...)
	}
	s.lines = append(s.lines[:at], append(block, s.lines[at:]...)...)
	s.reparse()
	return at + 1
}

// insertLine finds the line index a new entry dated d should be written at.
func (s *Source) insertLine(d Date) int {
	last := -1
	for _, dir := range s.file.Directives {
		if dir.When().IsZero() || dir.When().After(d) {
			continue
		}
		_, end := dir.Where().Lines()
		if end > last {
			last = end
		}
	}
	if last < 0 {
		return len(s.lines) // nothing earlier: append
	}
	return last // 0-based index just past the last earlier entry's final line
}

// Replace rewrites an existing directive's lines with a formatted transaction.
// Only that entry's span changes; the rest of the file is untouched.
func (s *Source) Replace(old Directive, t *Transaction, opts FormatOptions) bool {
	start, end := old.Where().Lines()
	if start <= 0 || end > len(s.lines) {
		return false
	}
	block := splitLines(strings.TrimRight(FormatTransaction(t, opts), "\n"))
	s.lines = append(s.lines[:start-1], append(block, s.lines[end:]...)...)
	s.reparse()
	return true
}

// Delete removes a directive's lines, along with one blank line left behind so
// deleting an entry does not leave a growing gap.
func (s *Source) Delete(d Directive) bool {
	start, end := d.Where().Lines()
	if start <= 0 || end > len(s.lines) {
		return false
	}
	from, to := start-1, end
	if to < len(s.lines) && strings.TrimSpace(s.lines[to]) == "" {
		to++
	} else if from > 0 && strings.TrimSpace(s.lines[from-1]) == "" {
		from--
	}
	s.lines = append(s.lines[:from], s.lines[to:]...)
	s.reparse()
	return true
}

// reparse refreshes the parsed view after an edit, so line spans stay accurate.
func (s *Source) reparse() {
	if f, _ := Parse(s.Path, s.String()); f != nil {
		s.file = f
	}
}

func splitLines(s string) []string { return strings.Split(s, "\n") }
