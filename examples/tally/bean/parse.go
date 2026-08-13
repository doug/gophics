package bean

import (
	"strings"

	"github.com/shopspring/decimal"
)

// Parse reads beancount source into a File.
//
// Parsing is error-tolerant: a line it cannot understand is reported and skipped,
// and the rest of the file still parses. A ledger is a user's data — one bad line
// should not make the whole thing unopenable.
func Parse(path, src string) (*File, error) {
	lines, err := scan(src)
	if err != nil {
		return nil, err
	}
	p := &parser{path: path, lines: lines}
	f := p.file()
	return f, p.errs.Err()
}

type parser struct {
	path  string
	lines []line
	i     int
	errs  ErrorList

	// tags and meta pushed by pushtag/pushmeta apply to every following
	// transaction until popped.
	pushedTags []string
	pushedMeta Meta
}

func (p *parser) file() *File {
	f := &File{Path: p.path}
	for p.i < len(p.lines) {
		ln := p.lines[p.i]
		if ln.blank || ln.indent > 0 {
			// A stray indented line with no directive above it is not fatal.
			p.i++
			continue
		}
		before := p.i
		if d := p.directive(); d != nil {
			// Record the directive's span, ignoring blank lines the block loop
			// consumed past its last real content.
			last := p.i - 1
			for last > before && p.lines[last].blank {
				last--
			}
			if es, ok := d.(interface{ setEnd(int) }); ok {
				es.setEnd(p.lines[last].num)
			}
			f.Directives = append(f.Directives, d)
		}
		if p.i == before {
			p.i++ // guarantee progress even if a branch forgot to advance
		}
	}
	return f
}

// directive parses one top-level entry and everything indented under it.
func (p *parser) directive() Directive {
	ln := p.lines[p.i]
	toks := ln.toks
	if len(toks) == 0 {
		p.i++
		return nil
	}
	pos := Position{File: p.path, Line: ln.num}

	// Undated forms come first: they start with a keyword, not a date.
	if toks[0].kind == tokWord {
		switch toks[0].text {
		case "option":
			p.i++
			if len(toks) < 3 {
				return p.fail(ln, "option needs a name and a value")
			}
			return &Option{base: base{Pos: pos}, Name: toks[1].text, Value: toks[2].text}
		case "plugin":
			p.i++
			if len(toks) < 2 {
				return p.fail(ln, "plugin needs a name")
			}
			cfg := ""
			if len(toks) > 2 {
				cfg = toks[2].text
			}
			return &Plugin{base: base{Pos: pos}, Name: toks[1].text, Config: cfg}
		case "include":
			p.i++
			if len(toks) < 2 {
				return p.fail(ln, "include needs a path")
			}
			return &Include{base: base{Pos: pos}, Path: toks[1].text}
		case "pushtag":
			p.i++
			if len(toks) > 1 && toks[1].kind == tokTag {
				p.pushedTags = append(p.pushedTags, toks[1].text)
			}
			return nil
		case "poptag":
			p.i++
			if len(toks) > 1 && toks[1].kind == tokTag {
				p.popTag(toks[1].text)
			}
			return nil
		case "pushmeta":
			p.i++
			if len(toks) > 1 && toks[1].kind == tokKey {
				p.pushedMeta = append(p.pushedMeta, p.metaValue(toks[1].text, toks[2:]))
			}
			return nil
		case "popmeta":
			p.i++
			if len(toks) > 1 && toks[1].kind == tokKey {
				p.popMeta(toks[1].text)
			}
			return nil
		}
	}

	if toks[0].kind != tokDate {
		p.i++
		// Beancount files may carry org-mode section headings ("* Assets"), which
		// are presentation, not data.
		if toks[0].kind == tokPunct && toks[0].text == "*" {
			return nil
		}
		return p.fail(ln, "expected a date or a directive keyword, got "+strconvQuote(toks[0].text))
	}
	date, ok := parseDate(toks[0].text)
	if !ok {
		p.i++
		return p.fail(ln, "invalid date "+strconvQuote(toks[0].text))
	}
	if len(toks) < 2 {
		p.i++
		return p.fail(ln, "a date must be followed by a directive")
	}

	kw := toks[1]
	b := base{Date: date, Pos: pos}

	// A transaction's second token is a flag (* or !) or a quoted string, not a
	// keyword; everything else dispatches on the keyword.
	if kw.kind == tokWord {
		switch kw.text {
		case "open":
			p.i++
			return p.open(b, toks, ln)
		case "close":
			p.i++
			return p.simpleAccount(b, toks, ln, func(a Account) Directive {
				return &Close{base: p.withMeta(b), Account: a}
			})
		case "commodity":
			p.i++
			if len(toks) < 3 {
				return p.fail(ln, "commodity needs a currency")
			}
			d := &Commodity{base: b, Currency: toks[2].text}
			d.Meta = p.indentedMeta()
			return d
		case "balance":
			p.i++
			return p.balance(b, toks, ln)
		case "pad":
			p.i++
			if len(toks) < 4 {
				return p.fail(ln, "pad needs an account and a source account")
			}
			d := &Pad{base: b, Account: Account(toks[2].text), Source: Account(toks[3].text)}
			d.Meta = p.indentedMeta()
			return d
		case "price":
			p.i++
			return p.price(b, toks, ln)
		case "note":
			p.i++
			if len(toks) < 4 {
				return p.fail(ln, "note needs an account and a comment")
			}
			d := &Note{base: b, Account: Account(toks[2].text), Comment: toks[3].text}
			d.Meta = p.indentedMeta()
			return d
		case "document":
			p.i++
			if len(toks) < 4 {
				return p.fail(ln, "document needs an account and a path")
			}
			d := &Document{base: b, Account: Account(toks[2].text), Path: toks[3].text}
			d.Meta = p.indentedMeta()
			return d
		case "event":
			p.i++
			if len(toks) < 4 {
				return p.fail(ln, "event needs a type and a description")
			}
			d := &Event{base: b, Type: toks[2].text, Description: toks[3].text}
			d.Meta = p.indentedMeta()
			return d
		case "query":
			p.i++
			if len(toks) < 4 {
				return p.fail(ln, "query needs a name and a query string")
			}
			d := &Query{base: b, Name: toks[2].text, SQL: toks[3].text}
			d.Meta = p.indentedMeta()
			return d
		case "custom":
			p.i++
			d := &Custom{base: b}
			if len(toks) > 2 {
				d.Type = toks[2].text
			}
			for _, t := range toks[3:] {
				d.Values = append(d.Values, t.text)
			}
			d.Meta = p.indentedMeta()
			return d
		case "txn":
			p.i++
			return p.transaction(b, toks[2:], "*")
		}
	}

	// Otherwise: a transaction. Its header is a flag, or a bare string when the
	// flag is omitted. A multi-character word here is an unknown directive, not a
	// transaction — reporting it beats silently inventing an empty transaction.
	p.i++
	flag := "*"
	rest := toks[1:]
	switch {
	case kw.kind == tokPunct && isFlag(kw.text):
		flag, rest = kw.text, toks[2:]
	case kw.kind == tokWord && len(kw.text) == 1:
		flag, rest = kw.text, toks[2:]
	case kw.kind == tokString:
		// bare transaction: 2021-01-01 "Payee" "Narration"
	default:
		p.skipIndented()
		return p.fail(ln, "unknown directive "+strconvQuote(kw.text))
	}
	return p.transaction(b, rest, flag)
}

// transaction parses the header remainder plus the indented postings/metadata.
func (p *parser) transaction(b base, rest []token, flag string) Directive {
	t := &Transaction{base: b, Flag: flag}

	// The header carries up to two strings: narration alone, or payee then
	// narration. Tags and links may follow in any order.
	var strs []string
	for _, tok := range rest {
		switch tok.kind {
		case tokString:
			strs = append(strs, tok.text)
		case tokTag:
			t.Tags = append(t.Tags, tok.text)
		case tokLink:
			t.Links = append(t.Links, tok.text)
		}
	}
	switch len(strs) {
	case 0:
	case 1:
		t.Narration = strs[0]
	default:
		t.Payee, t.Narration = strs[0], strs[1]
	}
	t.Tags = append(t.Tags, p.pushedTags...)
	t.Meta = append(t.Meta, p.pushedMeta...)

	// Indented lines below are either metadata or postings.
	for p.i < len(p.lines) {
		ln := p.lines[p.i]
		if ln.blank {
			p.i++
			continue
		}
		if ln.indent == 0 {
			break
		}
		toks := ln.toks
		if len(toks) == 0 {
			p.i++
			continue
		}
		if toks[0].kind == tokKey {
			item := p.metaValue(toks[0].text, toks[1:])
			if len(t.Postings) > 0 {
				last := t.Postings[len(t.Postings)-1]
				last.Meta = append(last.Meta, item)
			} else {
				t.Meta = append(t.Meta, item)
			}
			p.i++
			continue
		}
		post := p.posting(ln)
		p.i++
		if post != nil {
			t.Postings = append(t.Postings, post)
		}
	}
	return t
}

// posting parses one indented posting line:
//
//	[flag] Account [amount [{cost}] [@ price]]
func (p *parser) posting(ln line) *Posting {
	toks := ln.toks
	pos := Position{File: p.path, Line: ln.num}
	i := 0

	post := &Posting{Pos: pos}
	if toks[i].kind == tokPunct && (toks[i].text == "*" || toks[i].text == "!") {
		post.Flag = toks[i].text
		i++
	}
	if i >= len(toks) {
		p.errs = append(p.errs, &SyntaxError{File: p.path, Line: ln.num, Msg: "posting has no account"})
		return nil
	}
	if toks[i].kind != tokAccount && toks[i].kind != tokWord {
		p.errs = append(p.errs, &SyntaxError{File: p.path, Line: ln.num,
			Msg: "expected an account, got " + strconvQuote(toks[i].text)})
		return nil
	}
	post.Account = Account(toks[i].text)
	i++

	// An elided amount is legal — exactly one per transaction — and is inferred
	// during processing.
	if i >= len(toks) {
		return post
	}
	amt, n := parseAmount(toks[i:])
	if n == 0 {
		p.errs = append(p.errs, &SyntaxError{File: p.path, Line: ln.num,
			Msg: "expected an amount after the account, got " + strconvQuote(toks[i].text)})
		return post
	}
	post.Amount = amt
	i += n

	// Optional cost basis, then optional price.
	if i < len(toks) && toks[i].kind == tokPunct && (toks[i].text == "{" || toks[i].text == "{{") {
		cost, n := p.parseCost(toks[i:], ln)
		post.Cost = cost
		i += n
	}
	if i < len(toks) && toks[i].kind == tokPunct && (toks[i].text == "@" || toks[i].text == "@@") {
		post.PriceTotal = toks[i].text == "@@"
		i++
		if price, n := parseAmount(toks[i:]); n > 0 {
			post.Price = price
			i += n
		}
	}
	return post
}

// parseCost reads {…} or {{…}}, returning the cost and tokens consumed.
func (p *parser) parseCost(toks []token, ln line) (*Cost, int) {
	open := toks[0].text
	total := open == "{{"
	closeTok := "}"
	if total {
		closeTok = "}}"
	}

	cost := &Cost{}
	i := 1
	for i < len(toks) {
		t := toks[i]
		if t.kind == tokPunct && t.text == closeTok {
			i++
			break
		}
		switch {
		case t.kind == tokPunct && t.text == ",":
			i++
		case t.kind == tokDate:
			if d, ok := parseDate(t.text); ok {
				cost.Date = &d
			}
			i++
		case t.kind == tokString:
			cost.Label = t.text
			i++
		case t.kind == tokNumber:
			if amt, n := parseAmount(toks[i:]); n > 0 {
				if total {
					cost.Total = amt
				} else {
					cost.Amount = amt
				}
				i += n
				continue
			}
			i++
		default:
			i++
		}
	}
	return cost, i
}

func (p *parser) open(b base, toks []token, ln line) Directive {
	if len(toks) < 3 {
		return p.fail(ln, "open needs an account")
	}
	d := &Open{base: b, Account: Account(toks[2].text)}
	for _, t := range toks[3:] {
		switch t.kind {
		case tokWord:
			// Currencies are upper-case; a quoted word is the booking method.
			d.Currencies = append(d.Currencies, t.text)
		case tokString:
			d.Booking = t.text
		}
	}
	d.Meta = p.indentedMeta()
	return d
}

func (p *parser) balance(b base, toks []token, ln line) Directive {
	if len(toks) < 4 {
		return p.fail(ln, "balance needs an account and an amount")
	}
	d := &Assertion{base: b, Account: Account(toks[2].text)}
	rest := toks[3:]

	// An optional tolerance is written "amount ~ tolerance currency".
	if amt, n := parseAmount(rest); n > 0 {
		d.Amount = *amt
		rest = rest[n:]
	} else {
		return p.fail(ln, "balance needs an amount")
	}
	if len(rest) >= 2 && rest[0].kind == tokPunct && rest[0].text == "~" {
		if tol, err := decimal.NewFromString(cleanNumber(rest[1].raw)); err == nil {
			d.Tolerance = &tol
		}
	}
	d.Meta = p.indentedMeta()
	return d
}

func (p *parser) price(b base, toks []token, ln line) Directive {
	if len(toks) < 4 {
		return p.fail(ln, "price needs a commodity and an amount")
	}
	d := &Price{base: b, Currency: toks[2].text}
	if amt, n := parseAmount(toks[3:]); n > 0 {
		d.Amount = *amt
	} else {
		return p.fail(ln, "price needs an amount")
	}
	d.Meta = p.indentedMeta()
	return d
}

func (p *parser) simpleAccount(b base, toks []token, ln line, make func(Account) Directive) Directive {
	if len(toks) < 3 {
		return p.fail(ln, "directive needs an account")
	}
	d := make(Account(toks[2].text))
	return d
}

// withMeta attaches any indented metadata following the current directive.
func (p *parser) withMeta(b base) base {
	b.Meta = p.indentedMeta()
	return b
}

// indentedMeta consumes the indented metadata lines below a directive.
func (p *parser) indentedMeta() Meta {
	var m Meta
	for p.i < len(p.lines) {
		ln := p.lines[p.i]
		if ln.blank {
			p.i++
			continue
		}
		if ln.indent == 0 || len(ln.toks) == 0 || ln.toks[0].kind != tokKey {
			break
		}
		m = append(m, p.metaValue(ln.toks[0].text, ln.toks[1:]))
		p.i++
	}
	return m
}

// metaValue decodes one metadata value from the tokens after its key.
func (p *parser) metaValue(key string, toks []token) MetaItem {
	item := MetaItem{Key: key}
	if len(toks) == 0 {
		return item
	}
	item.Raw = joinRaw(toks)
	switch t := toks[0]; t.kind {
	case tokString:
		item.Value = t.text
	case tokDate:
		if d, ok := parseDate(t.text); ok {
			item.Value = d
		}
	case tokAccount:
		item.Value = Account(t.text)
	case tokNumber:
		if amt, n := parseAmount(toks); n > 1 {
			item.Value = amt
		} else if d, err := decimal.NewFromString(cleanNumber(t.raw)); err == nil {
			item.Value = d
		}
	case tokWord:
		switch t.text {
		case "TRUE":
			item.Value = true
		case "FALSE":
			item.Value = false
		default:
			item.Value = t.text
		}
	default:
		item.Value = t.text
	}
	return item
}

// isFlag reports the transaction flag characters beancount allows.
func isFlag(s string) bool {
	switch s {
	case "*", "!", "&", "?", "%":
		return true
	}
	return false
}

// skipIndented consumes the indented block under a directive we could not parse,
// so its postings are not mistaken for top-level entries.
func (p *parser) skipIndented() {
	for p.i < len(p.lines) {
		ln := p.lines[p.i]
		if !ln.blank && ln.indent == 0 {
			return
		}
		p.i++
	}
}

func (p *parser) fail(ln line, msg string) Directive {
	p.errs = append(p.errs, &SyntaxError{File: p.path, Line: ln.num, Msg: msg})
	return nil
}

func (p *parser) popTag(tag string) {
	for i := len(p.pushedTags) - 1; i >= 0; i-- {
		if p.pushedTags[i] == tag {
			p.pushedTags = append(p.pushedTags[:i], p.pushedTags[i+1:]...)
			return
		}
	}
}

func (p *parser) popMeta(key string) {
	for i := len(p.pushedMeta) - 1; i >= 0; i-- {
		if p.pushedMeta[i].Key == key {
			p.pushedMeta = append(p.pushedMeta[:i], p.pushedMeta[i+1:]...)
			return
		}
	}
}

// parseAmount reads "NUMBER CURRENCY", returning the amount and tokens consumed.
// It also accepts a bare number (no currency), which balance tolerances and some
// metadata use.
func parseAmount(toks []token) (*Amount, int) {
	if len(toks) == 0 || toks[0].kind != tokNumber {
		return nil, 0
	}
	num, err := decimal.NewFromString(cleanNumber(toks[0].raw))
	if err != nil {
		return nil, 0
	}
	a := &Amount{Number: num, Raw: toks[0].raw}
	if len(toks) > 1 && toks[1].kind == tokWord && isCurrency(toks[1].text) {
		a.Currency = toks[1].text
		return a, 2
	}
	return a, 1
}

// cleanNumber strips grouping commas and a leading '+' so decimal can read it.
func cleanNumber(s string) string {
	s = strings.ReplaceAll(s, ",", "")
	return strings.TrimPrefix(s, "+")
}

// isCurrency reports the commodity shape: upper-case initial, and no lower-case
// letters (USD, GLD, VBMPX, IRAUSD).
func isCurrency(w string) bool {
	if w == "" {
		return false
	}
	if w[0] < 'A' || w[0] > 'Z' {
		return false
	}
	for i := 0; i < len(w); i++ {
		if w[i] >= 'a' && w[i] <= 'z' {
			return false
		}
	}
	return true
}

// parseDate reads YYYY-MM-DD.
func parseDate(s string) (Date, bool) {
	if len(s) != 10 || s[4] != '-' || s[7] != '-' {
		return Date{}, false
	}
	y, ok1 := atoi(s[0:4])
	m, ok2 := atoi(s[5:7])
	d, ok3 := atoi(s[8:10])
	if !ok1 || !ok2 || !ok3 || m < 1 || m > 12 || d < 1 || d > 31 {
		return Date{}, false
	}
	return Date{Year: y, Month: monthOf(m), Day: d}, true
}

func atoi(s string) (int, bool) {
	n := 0
	for i := 0; i < len(s); i++ {
		if !isDigit(s[i]) {
			return 0, false
		}
		n = n*10 + int(s[i]-'0')
	}
	return n, true
}

func joinRaw(toks []token) string {
	parts := make([]string, len(toks))
	for i, t := range toks {
		parts[i] = t.raw
	}
	return strings.Join(parts, " ")
}
