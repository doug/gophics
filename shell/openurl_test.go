package shell

import "testing"

// The allow-list is the whole contract, so every entry and the shapes it
// refuses are pinned. Desktop used to accept only http(s) while web and
// mobile accepted anything; the rule now lives here and the shells ask it.
func TestCheckOpenURL(t *testing.T) {
	for _, tc := range []struct {
		url string
		ok  bool
	}{
		{"https://example.com/a?b=c", true},
		{"http://example.com", true},
		{"HTTPS://EXAMPLE.COM", true}, // url.Parse lower-cases the scheme
		{"mailto:someone@example.com?subject=hi", true},
		{"tel:+15555550100", true},
		{"file:///etc/passwd", false},
		{"javascript:alert(1)", false},
		{"myapp://open/42", false},
		{"ftp://example.com", false},
		{"example.com", false},             // no scheme
		{`C:\Users\doug\notes.txt`, false}, // a one-letter "scheme"
		{"", false},
		{"http://[::1", false}, // does not parse
	} {
		err := CheckOpenURL(tc.url)
		if tc.ok && err != nil {
			t.Errorf("CheckOpenURL(%q) = %v, want it to open", tc.url, err)
		}
		if !tc.ok && err == nil {
			t.Errorf("CheckOpenURL(%q) = nil, want a refusal", tc.url)
		}
	}
}
