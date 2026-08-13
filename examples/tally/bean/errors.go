package bean

import (
	"errors"
	"fmt"
	"strconv"
)

// SyntaxError reports a problem reading the source text.
type SyntaxError struct {
	File string
	Line int
	Col  int
	Msg  string
}

func (e *SyntaxError) Error() string {
	where := ""
	if e.File != "" {
		where = e.File + ":"
	}
	return fmt.Sprintf("%s%d:%d: %s", where, e.Line, e.Col, e.Msg)
}

var errUnterminatedString = errors.New("unterminated string")

// strconvQuote is strconv.Quote, wrapped so lex.go needn't import strconv.
func strconvQuote(s string) string { return strconv.Quote(s) }

// ErrorList collects problems without abandoning the parse: a ledger with one bad
// line should still open and show everything else, the way an editor keeps working
// on a file with a syntax error.
type ErrorList []error

func (l ErrorList) Error() string {
	switch len(l) {
	case 0:
		return "no errors"
	case 1:
		return l[0].Error()
	}
	return fmt.Sprintf("%s (and %d more)", l[0], len(l)-1)
}

// Err returns the list as an error, or nil when empty.
func (l ErrorList) Err() error {
	if len(l) == 0 {
		return nil
	}
	return l
}
