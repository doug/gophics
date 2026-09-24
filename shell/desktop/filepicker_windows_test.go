//go:build windows

package desktop

import (
	"strings"
	"testing"
)

// The dialog script is assembled from the caller's strings, so the quoting is
// the injection surface. Inside a PowerShell single-quoted string the only
// metacharacter is the quote itself; a newline ends the statement.
func TestPsEscape(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"plain", "plain"},
		{"O'Brien", "O''Brien"},
		{"a'b'c", "a''b''c"},
		{"line\nbreak", "line break"},
		{"cr\r\nlf", "cr  lf"},
		{"'; Remove-Item -Recurse C:\\ #", "''; Remove-Item -Recurse C:\\ #"},
		{"$env:USERPROFILE", "$env:USERPROFILE"}, // no expansion in single quotes, so left alone
		{"", ""},
	} {
		if got := psEscape(tc.in); got != tc.want {
			t.Errorf("psEscape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
	// Whatever goes in, the output must never contain a bare quote or a
	// newline: either would end the string literal.
	for _, s := range []string{"'", "''", "'\n'", "a\r'"} {
		got := psEscape(s)
		if strings.ContainsAny(got, "\n\r") {
			t.Errorf("psEscape(%q) = %q still holds a line break", s, got)
		}
		if strings.Count(got, "'")%2 != 0 {
			t.Errorf("psEscape(%q) = %q has an odd number of quotes", s, got)
		}
	}
}

// The filter is what the dialog shows; a wrong one hides the user's file.
func TestWinFilter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		accept []string
		want   string
	}{
		{"none", nil, "All files|*.*"},
		{"empty entries", []string{"", " "}, "All files|*.*"},
		{"dotted extensions", []string{".png", ".jpg"}, "Supported files|*.png;*.jpg|All files|*.*"},
		{"bare extensions", []string{"png"}, "Supported files|*.png|All files|*.*"},
		{"mime types widen to everything", []string{"image/png"}, "All files|*.*"},
		{"mime beside an extension", []string{"image/*", ".txt"}, "Supported files|*.txt|All files|*.*"},
		{"whitespace trimmed", []string{" .md "}, "Supported files|*.md|All files|*.*"},
	} {
		if got := winFilter(tc.accept); got != tc.want {
			t.Errorf("%s: winFilter(%q) = %q, want %q", tc.name, tc.accept, got, tc.want)
		}
	}
}
