//go:build darwin && !ios && !js

package desktop

import (
	"encoding/base64"
	"strings"
	"testing"
)

// The command line handed to security(1) is built from the caller's strings.
// Its tokenizer honours double quotes with backslash escapes, so those two
// characters are the whole quoting surface.
func TestQuoteSec(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"token", `"token"`},
		{"with space", `"with space"`},
		{`say "hi"`, `"say \"hi\""`},
		{`back\slash`, `"back\\slash"`},
		{`\"`, `"\\\""`},
		{"", `""`},
	} {
		if got := quoteSec(tc.in); got != tc.want {
			t.Errorf("quoteSec(%q) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

// Values are stored base64-encoded so arbitrary strings survive the CLI
// round trip. The alphabet must contain nothing quoteSec has to touch, or a
// value that quoted correctly on the way in would not decode on the way out.
func TestKeychainValueEncodingNeedsNoQuoting(t *testing.T) {
	for _, v := range []string{"", "plain", `"quoted"`, `back\slash`, "multi\nline", "ünïcödé ✓", "\x00\xff"} {
		enc := base64.StdEncoding.EncodeToString([]byte(v))
		if strings.ContainsAny(enc, `"\ `) {
			t.Errorf("encoding of %q is %q, which quoteSec would have to escape", v, enc)
		}
		if quoteSec(enc) != `"`+enc+`"` {
			t.Errorf("quoteSec changed the encoded value %q", enc)
		}
		dec, err := base64.StdEncoding.DecodeString(strings.TrimSpace(enc + "\n"))
		if err != nil || string(dec) != v {
			t.Errorf("round trip of %q gave %q, %v", v, dec, err)
		}
	}
}
