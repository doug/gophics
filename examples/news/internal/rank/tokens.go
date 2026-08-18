package rank

import "strings"

// Tokenize reduces a headline and teaser to the words worth learning from:
// lowercase, no punctuation, no stopwords, nothing shorter than three
// characters, and each word counted once per article so a title that repeats a
// term does not get three votes.
//
// Numbers are dropped except for four-digit years, which carry real meaning in
// a headline ("the 2008 crisis") where a bare count does not.
func Tokenize(s string) []string {
	fields := strings.FieldsFunc(strings.ToLower(s), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '\'')
	})
	seen := make(map[string]bool, len(fields))
	out := make([]string, 0, len(fields))
	for _, f := range fields {
		f = strings.Trim(f, "'")
		if len(f) < 3 || stopwords[f] {
			continue
		}
		if isNumber(f) && len(f) != 4 {
			continue
		}
		if seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) >= maxTokensPerItem {
			break
		}
	}
	return out
}

// maxTokensPerItem bounds how much one article can contribute. Summaries run to
// 400 characters and an unbounded bag would let a verbose feed dominate the
// vocabulary.
const maxTokensPerItem = 40

func isNumber(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// stopwords are words that appear in everything and so distinguish nothing.
// The list is short on purpose: aggressive filtering throws away signal, and
// the shrinkage in llr already stops a common word from mattering much.
var stopwords = map[string]bool{
	"the": true, "and": true, "for": true, "are": true, "but": true,
	"not": true, "you": true, "all": true, "can": true, "her": true,
	"was": true, "one": true, "our": true, "out": true, "day": true,
	"get": true, "has": true, "him": true, "his": true, "how": true,
	"its": true, "new": true, "now": true, "old": true, "see": true,
	"two": true, "who": true, "did": true, "yes": true, "let": true,
	"put": true, "say": true, "she": true, "too": true, "use": true,
	"that": true, "with": true, "this": true, "from": true, "they": true,
	"will": true, "what": true, "when": true, "your": true, "them": true,
	"than": true, "then": true, "have": true, "been": true, "were": true,
	"into": true, "over": true, "just": true, "some": true, "more": true,
	"only": true, "also": true, "very": true, "most": true, "much": true,
	"such": true, "here": true, "there": true, "about": true, "would": true,
	"could": true, "their": true, "which": true, "these": true, "those": true,
	"other": true, "after": true, "before": true, "being": true, "where": true,
	"while": true, "still": true, "every": true, "should": true, "because": true,
	"through": true, "between": true, "against": true, "during": true,
	"says": true, "said": true, "make": true, "made": true, "like": true,
	"time": true, "year": true, "years": true, "back": true, "even": true,
	"want": true, "need": true, "does": true, "doing": true, "going": true,
	"got": true, "has_been": true, "may": true, "might": true, "must": true,
}
