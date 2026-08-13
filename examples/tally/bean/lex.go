package bean

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// The scanner is line-oriented because beancount is: a directive starts in
// column 0, and its postings and metadata are the indented lines that follow.
// Splitting the file into lines first, then tokenizing each line, keeps that
// structure explicit and makes every error report a real line number.

// tokKind classifies a token within a line.
type tokKind uint8

const (
	tokEOL tokKind = iota
	tokDate
	tokNumber
	tokString // a quoted string, unescaped
	tokWord   // bare word: directive keyword, currency, flag-less identifier
	tokAccount
	tokTag   // #tag
	tokLink  // ^link
	tokKey   // identifier followed by ':' — a metadata key
	tokPunct // one of { } { { } } , @ @@ * ! etc.
)

// token is one lexical item on a line.
type token struct {
	kind tokKind
	text string // decoded text (strings unescaped, punctuation as written)
	raw  string // exactly as it appeared, for round-tripping numbers
	col  int
}

// line is one physical source line, split into its indent and tokens.
type line struct {
	num    int
	indent int // leading spaces/tabs; >0 means a continuation of the directive above
	toks   []token
	text   string // the raw line, comment stripped, for error messages
	blank  bool   // nothing but whitespace or a comment
}

// scan splits src into lines and tokenizes each.
func scan(src string) ([]line, error) {
	var out []line
	for i, raw := range strings.Split(src, "\n") {
		raw = strings.TrimRight(raw, "\r")
		ln := line{num: i + 1}
		body, indent := stripIndent(raw)
		ln.indent = indent

		// A ';' starts a comment, except inside a string. Comment-only lines
		// (and beancount's org-style '*' headings in column 0) are skipped.
		body = stripComment(body)
		ln.text = body
		if strings.TrimSpace(body) == "" {
			ln.blank = true
			out = append(out, ln)
			continue
		}
		toks, err := tokenize(body, indent, i+1)
		if err != nil {
			return nil, err
		}
		ln.toks = toks
		ln.blank = len(toks) == 0
		out = append(out, ln)
	}
	return out, nil
}

// stripIndent splits leading whitespace from a line, counting a tab as one.
func stripIndent(s string) (body string, indent int) {
	i := 0
	for i < len(s) && (s[i] == ' ' || s[i] == '\t') {
		i++
	}
	return s[i:], i
}

// stripComment removes a trailing ';' comment, respecting quoted strings so a
// semicolon inside a payee ("Sam; Co") is not mistaken for one.
func stripComment(s string) string {
	inStr := false
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '"':
			// A quote escaped with a backslash stays inside the string.
			if i > 0 && s[i-1] == '\\' {
				continue
			}
			inStr = !inStr
		case ';':
			if !inStr {
				return strings.TrimRight(s[:i], " \t")
			}
		}
	}
	return s
}

// tokenize splits one line's body into tokens.
func tokenize(s string, indent, lineNum int) ([]token, error) {
	var toks []token
	i := 0
	for i < len(s) {
		if s[i] == ' ' || s[i] == '\t' {
			i++
			continue
		}
		start := i
		col := indent + start

		switch c := s[i]; {
		case c == '"':
			text, n, err := scanString(s[i:])
			if err != nil {
				return nil, &SyntaxError{Line: lineNum, Col: col, Msg: err.Error()}
			}
			toks = append(toks, token{kind: tokString, text: text, raw: s[i : i+n], col: col})
			i += n

		case c == '#':
			i++
			j := scanBareWord(s, i)
			toks = append(toks, token{kind: tokTag, text: s[i:j], raw: s[start:j], col: col})
			i = j

		case c == '^':
			i++
			j := scanBareWord(s, i)
			toks = append(toks, token{kind: tokLink, text: s[i:j], raw: s[start:j], col: col})
			i = j

		case c == '{':
			if strings.HasPrefix(s[i:], "{{") {
				toks = append(toks, token{kind: tokPunct, text: "{{", raw: "{{", col: col})
				i += 2
			} else {
				toks = append(toks, token{kind: tokPunct, text: "{", raw: "{", col: col})
				i++
			}

		case c == '}':
			if strings.HasPrefix(s[i:], "}}") {
				toks = append(toks, token{kind: tokPunct, text: "}}", raw: "}}", col: col})
				i += 2
			} else {
				toks = append(toks, token{kind: tokPunct, text: "}", raw: "}", col: col})
				i++
			}

		case c == '@':
			if strings.HasPrefix(s[i:], "@@") {
				toks = append(toks, token{kind: tokPunct, text: "@@", raw: "@@", col: col})
				i += 2
			} else {
				toks = append(toks, token{kind: tokPunct, text: "@", raw: "@", col: col})
				i++
			}

		case c == ',' || c == '(' || c == ')' || c == '~' ||
			c == '*' || c == '!' || c == '&' || c == '?' || c == '%' ||
			c == '+' || c == '-' && !startsNumber(s[i:]):
			toks = append(toks, token{kind: tokPunct, text: string(c), raw: string(c), col: col})
			i++

		case isDigit(c) || (c == '-' && startsNumber(s[i:])):
			j, isDate := scanNumberOrDate(s, i)
			raw := s[i:j]
			kind := tokNumber
			if isDate {
				kind = tokDate
			}
			toks = append(toks, token{kind: kind, text: raw, raw: raw, col: col})
			i = j

		default:
			j := scanBareWord(s, i)
			if j == i {
				// An unrecognized byte: report it rather than looping forever.
				_, sz := utf8.DecodeRuneInString(s[i:])
				return nil, &SyntaxError{Line: lineNum, Col: col,
					Msg: "unexpected character " + strconvQuote(s[i:i+sz])}
			}
			word := s[i:j]
			kind := tokWord
			// scanBareWord accepts ':' because accounts contain it, so a metadata
			// key arrives with its colon attached ("name:"). Strip it back off.
			switch {
			case strings.HasSuffix(word, ":") && isMetaKey(strings.TrimSuffix(word, ":")):
				kind = tokKey
				word = strings.TrimSuffix(word, ":")
			case isAccountName(word):
				kind = tokAccount
			}
			toks = append(toks, token{kind: kind, text: word, raw: s[i:j], col: col})
			i = j
		}
	}
	return toks, nil
}

// scanString reads a quoted string, returning the decoded text and how many bytes
// it consumed.
func scanString(s string) (string, int, error) {
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			if i+1 < len(s) {
				i++
				switch s[i] {
				case 'n':
					b.WriteByte('\n')
				case 't':
					b.WriteByte('\t')
				default:
					b.WriteByte(s[i])
				}
				continue
			}
		case '"':
			return b.String(), i + 1, nil
		default:
			b.WriteByte(s[i])
		}
	}
	return "", 0, errUnterminatedString
}

// scanBareWord consumes an identifier-ish run: letters, digits, and the
// punctuation that appears inside accounts, currencies and keys.
func scanBareWord(s string, i int) int {
	j := i
	for j < len(s) {
		r, sz := utf8.DecodeRuneInString(s[j:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) ||
			r == '_' || r == '-' || r == ':' || r == '.' || r == '\'' {
			j += sz
			continue
		}
		break
	}
	return j
}

// scanNumberOrDate consumes a date or a number. Both begin with digits, so the
// exact YYYY-MM-DD shape is tested first; anything else scans as a number
// (optional sign, digits, grouping commas, decimal point).
func scanNumberOrDate(s string, i int) (end int, isDate bool) {
	if looksLikeDate(s[i:]) {
		return i + 10, true
	}
	j := i
	if j < len(s) && (s[j] == '-' || s[j] == '+') {
		j++
	}
	for j < len(s) && (isDigit(s[j]) || s[j] == ',' || s[j] == '.') {
		j++
	}
	return j, false
}

// looksLikeDate reports whether s begins with exactly YYYY-MM-DD.
func looksLikeDate(s string) bool {
	if len(s) < 10 || s[4] != '-' || s[7] != '-' {
		return false
	}
	for _, i := range [8]int{0, 1, 2, 3, 5, 6, 8, 9} {
		if !isDigit(s[i]) {
			return false
		}
	}
	// A date must not run straight into more digits or another dash.
	if len(s) > 10 && (isDigit(s[10]) || s[10] == '-') {
		return false
	}
	return true
}

// startsNumber reports whether a '-' or '+' begins a signed number rather than
// standing alone.
func startsNumber(s string) bool {
	return len(s) > 1 && isDigit(s[1])
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

// isAccountName reports the beancount account shape: a capitalized root type
// followed by at least one ':' component.
func isAccountName(w string) bool {
	i := strings.IndexByte(w, ':')
	if i <= 0 || i == len(w)-1 {
		return false
	}
	r, _ := utf8.DecodeRuneInString(w)
	return unicode.IsUpper(r)
}

// isMetaKey reports the metadata-key shape: lowercase-initial and free of ':'.
func isMetaKey(w string) bool {
	if w == "" || strings.ContainsRune(w, ':') {
		return false
	}
	r, _ := utf8.DecodeRuneInString(w)
	return unicode.IsLower(r) || r == '_'
}
