package cookies

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "cookies.txt")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadNetscapeFormat(t *testing.T) {
	// Tab-separated, as every browser extension exports it.
	body := "# Netscape HTTP Cookie File\n" +
		"# This is a generated file!\n" +
		".economist.com\tTRUE\t/\tTRUE\t2000000000\tsession_id\tabc123\n" +
		"#HttpOnly_.economist.com\tTRUE\t/\tTRUE\t2000000000\tauth\tsecret-token\n" +
		"\n" +
		"www.economist.com\tFALSE\t/subscribe\tFALSE\t0\tprefs\tdark\n"

	cs, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 3 {
		t.Fatalf("loaded %d cookies, want 3", len(cs))
	}

	first := cs[0]
	if first.Name != "session_id" || first.Value != "abc123" {
		t.Errorf("first cookie = %+v", first)
	}
	// The leading dot is stripped, since net/http treats Domain as a suffix.
	if first.Domain != "economist.com" {
		t.Errorf("Domain = %q, want economist.com", first.Domain)
	}
	if !first.Secure {
		t.Error("Secure should be true")
	}
	if first.Expires.IsZero() {
		t.Error("Expires should be parsed")
	}

	if !cs[1].HttpOnly {
		t.Error("the #HttpOnly_ prefix should set HttpOnly, not be treated as a comment")
	}
	if cs[1].Name != "auth" {
		t.Errorf("HttpOnly cookie name = %q", cs[1].Name)
	}

	if cs[2].Path != "/subscribe" || cs[2].Secure {
		t.Errorf("third cookie = %+v", cs[2])
	}
	// A zero expiry means a session cookie, not 1970.
	if !cs[2].Expires.IsZero() {
		t.Errorf("expiry 0 should leave Expires zero, got %v", cs[2].Expires)
	}
}

func TestLoadSpaceSeparated(t *testing.T) {
	// Some exporters emit runs of spaces rather than tabs.
	body := ".x.test  TRUE  /  TRUE  2000000000  name  value\n"
	cs, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Name != "name" || cs[0].Value != "value" {
		t.Errorf("got %+v", cs)
	}
}

func TestLoadValueContainingTabs(t *testing.T) {
	// Everything after the name field is the value, even if it contains tabs.
	body := ".x.test\tTRUE\t/\tTRUE\t2000000000\tname\tpart1\tpart2\n"
	cs, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if cs[0].Value != "part1\tpart2" {
		t.Errorf("Value = %q", cs[0].Value)
	}
}

func TestLoadErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.txt")); err == nil {
		t.Error("expected an error for a missing file")
	}
	// A file with no cookie lines is a user mistake worth reporting, because the
	// alternative is silently continuing to fetch teasers.
	if _, err := Load(write(t, "# just a comment\n\n")); err == nil {
		t.Error("expected an error for a file with no cookies")
	}
	if _, err := Load(write(t, "not a cookie line at all\n")); err == nil {
		t.Error("expected an error when nothing parses")
	}
}

func TestExpired(t *testing.T) {
	now := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	body := ".x.test\tTRUE\t/\tTRUE\t1000000000\told\tv\n" + // 2001
		".x.test\tTRUE\t/\tTRUE\t4000000000\tfresh\tv\n" + // 2096
		".x.test\tTRUE\t/\tTRUE\t0\tsession\tv\n"
	cs, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	got := Expired(cs, now)
	if len(got) != 1 || got[0] != "old" {
		t.Errorf("Expired = %v, want [old]", got)
	}
}

// A raw DevTools "Cookie:" header is accepted, because copying one line from the
// network panel is much less work than installing an extension.
func TestLoadCookieHeaderFormat(t *testing.T) {
	for _, body := range []string{
		"Cookie: session=abc123; auth=tok-xyz; prefs=dark\n",
		"cookie: session=abc123; auth=tok-xyz; prefs=dark",
		"session=abc123; auth=tok-xyz; prefs=dark\n",
		"# exported from Chrome\nsession=abc123; auth=tok-xyz; prefs=dark\n",
	} {
		cs, err := Load(write(t, body))
		if err != nil {
			t.Fatalf("Load(%q): %v", body, err)
		}
		if len(cs) != 3 {
			t.Fatalf("Load(%q) got %d cookies, want 3: %+v", body, len(cs), cs)
		}
		if cs[0].Name != "session" || cs[0].Value != "abc123" {
			t.Errorf("first cookie = %+v", cs[0])
		}
		if cs[1].Name != "auth" || cs[1].Value != "tok-xyz" {
			t.Errorf("second cookie = %+v", cs[1])
		}
		// Header-form cookies must stay host-only so they can only reach the
		// site the feed belongs to.
		for _, c := range cs {
			if c.Domain != "" {
				t.Errorf("cookie %s has Domain %q; header form must be host-only", c.Name, c.Domain)
			}
		}
	}
}

func TestLoadHeaderWithBase64Value(t *testing.T) {
	// Session tokens routinely contain '=' padding, which must not be split.
	cs, err := Load(write(t, "Cookie: tok=YWJjZGVmZw==; x=1\n"))
	if err != nil {
		t.Fatal(err)
	}
	if cs[0].Value != "YWJjZGVmZw==" {
		t.Errorf("Value = %q, want the full base64 token", cs[0].Value)
	}
}

func TestLoadHeaderSpanningLines(t *testing.T) {
	cs, err := Load(write(t, "session=abc\nauth=xyz\n"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 2 {
		t.Errorf("got %d cookies, want 2: %+v", len(cs), cs)
	}
}

// Netscape format must still win when both could plausibly parse.
func TestNetscapePreferredOverHeader(t *testing.T) {
	body := ".economist.com\tTRUE\t/\tTRUE\t2000000000\tsession\tabc\n"
	cs, err := Load(write(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(cs) != 1 || cs[0].Domain != "economist.com" {
		t.Errorf("expected the Netscape parse to win, got %+v", cs[0])
	}
}
