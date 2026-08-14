package bean

import (
	"strconv"
	"strings"

	"github.com/doug/tally/decimal"
)

// Format renders directives as beancount source.
//
// A deliberate limit: this formats *entries*, not files. It is never used to
// rewrite a ledger wholesale, because a beancount file is a hand-maintained text
// document — section headings, comments explaining an odd transaction, blank lines
// grouping a month — and an app that reformatted all of it would destroy work the
// user did by hand and never asked to have touched. Editing goes through Source
// (edit.go), which splices formatted entries into the original text and leaves
// every other byte alone.

// FormatOptions controls the rendered layout.
type FormatOptions struct {
	// Indent prefixes postings and metadata (default two spaces).
	Indent string
	// AmountColumn is the column the amount's last digit aligns to, counting from
	// the start of the line. Numbers are right-aligned to it so decimal points
	// line up down the entry. Zero uses a width derived from the content.
	AmountColumn int
}

func (o FormatOptions) indent() string {
	if o.Indent == "" {
		return "  "
	}
	return o.Indent
}

// FormatTransaction renders one transaction, with its postings aligned.
func FormatTransaction(t *Transaction, opts FormatOptions) string {
	var b strings.Builder
	ind := opts.indent()

	b.WriteString(t.Date.String())
	b.WriteByte(' ')
	b.WriteString(flagOrDefault(t.Flag))
	if t.Payee != "" {
		b.WriteByte(' ')
		b.WriteString(quote(t.Payee))
	}
	if t.Narration != "" || t.Payee != "" {
		b.WriteByte(' ')
		b.WriteString(quote(t.Narration))
	}
	for _, tag := range t.Tags {
		b.WriteString(" #")
		b.WriteString(tag)
	}
	for _, link := range t.Links {
		b.WriteString(" ^")
		b.WriteString(link)
	}
	b.WriteByte('\n')

	for _, m := range t.Meta {
		b.WriteString(ind)
		b.WriteString(formatMeta(m))
		b.WriteByte('\n')
	}

	// Align the amounts: find the widest "account" prefix and the widest number,
	// then right-align every number to the same column so decimal points stack.
	col := opts.AmountColumn
	if col == 0 {
		col = amountColumn(t, ind)
	}
	for _, p := range t.Postings {
		b.WriteString(formatPosting(p, ind, col))
		b.WriteByte('\n')
		for _, m := range p.Meta {
			b.WriteString(ind)
			b.WriteString(ind)
			b.WriteString(formatMeta(m))
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// amountColumn computes where numbers should end so they align.
func amountColumn(t *Transaction, ind string) int {
	widestLeft, widestNum := 0, 0
	for _, p := range t.Postings {
		left := len(ind) + len(p.Account)
		if p.Flag != "" {
			left += len(p.Flag) + 1
		}
		if left > widestLeft {
			widestLeft = left
		}
		if p.Amount != nil {
			if n := len(formatNumber(p.Amount)); n > widestNum {
				widestNum = n
			}
		}
	}
	// Two spaces of breathing room between the longest account and the numbers.
	return widestLeft + 2 + widestNum
}

// formatPosting renders one posting with its number right-aligned to col.
func formatPosting(p *Posting, ind string, col int) string {
	var b strings.Builder
	b.WriteString(ind)
	if p.Flag != "" {
		b.WriteString(p.Flag)
		b.WriteByte(' ')
	}
	b.WriteString(string(p.Account))
	if p.Amount == nil {
		return b.String()
	}

	num := formatNumber(p.Amount)
	pad := col - b.Len() - len(num)
	if pad < 2 {
		pad = 2 // never let an amount touch the account name
	}
	b.WriteString(strings.Repeat(" ", pad))
	b.WriteString(num)
	if p.Amount.Currency != "" {
		b.WriteByte(' ')
		b.WriteString(p.Amount.Currency)
	}

	if p.Cost != nil {
		b.WriteByte(' ')
		b.WriteString(formatCost(p.Cost))
	}
	if p.Price != nil {
		if p.PriceTotal {
			b.WriteString(" @@ ")
		} else {
			b.WriteString(" @ ")
		}
		b.WriteString(formatAmount(p.Price))
	}
	return b.String()
}

func formatCost(c *Cost) string {
	var parts []string
	switch {
	case c.Amount != nil:
		parts = append(parts, formatAmount(c.Amount))
	case c.Total != nil:
		parts = append(parts, formatAmount(c.Total))
	}
	if c.Date != nil {
		parts = append(parts, c.Date.String())
	}
	if c.Label != "" {
		parts = append(parts, quote(c.Label))
	}
	inner := strings.Join(parts, ", ")
	if c.Total != nil {
		return "{{" + inner + "}}"
	}
	return "{" + inner + "}"
}

func formatAmount(a *Amount) string {
	if a == nil {
		return ""
	}
	if a.Currency == "" {
		return formatNumber(a)
	}
	return formatNumber(a) + " " + a.Currency
}

// formatNumber prefers the source spelling. Preserving what the author wrote —
// trailing zeros, grouping — means an entry the app did not edit comes back out
// exactly as it went in, and only genuinely new numbers get the app's formatting.
func formatNumber(a *Amount) string {
	if a.Raw != "" {
		return a.Raw
	}
	return a.Number.String()
}

func formatMeta(m MetaItem) string {
	return m.Key + ": " + formatMetaValue(m)
}

func formatMetaValue(m MetaItem) string {
	if m.Raw != "" {
		return m.Raw
	}
	switch v := m.Value.(type) {
	case nil:
		return ""
	case string:
		return quote(v)
	case Account:
		return string(v)
	case Date:
		return v.String()
	case bool:
		if v {
			return "TRUE"
		}
		return "FALSE"
	case decimal.Decimal:
		return v.String()
	case *Amount:
		return formatAmount(v)
	}
	return ""
}

func flagOrDefault(f string) string {
	if f == "" {
		return "*"
	}
	return f
}

// quote renders a beancount string literal.
func quote(s string) string {
	if !strings.ContainsAny(s, "\"\\\n\t") {
		return `"` + s + `"`
	}
	return strconv.Quote(s)
}
