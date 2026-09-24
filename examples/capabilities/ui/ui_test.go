package ui

import (
	"testing"
	"unicode/utf8"

	"github.com/doug/gophics/apptest"
)

// The IME card is where multi-byte text arrives, so its backspace has to
// remove a character, not a byte — a byte off "日本" leaves invalid UTF-8.
func TestBackspaceRemovesACharacter(t *testing.T) {
	cases := []struct{ in, want string }{
		{"日本", "日"},
		{"日", ""},
		{"ab", "a"},
		{"a", ""},
		{"é", ""},
		{"", ""},
	}
	for _, c := range cases {
		got := backspace(c.in)
		if got != c.want {
			t.Errorf("backspace(%q) = %q, want %q", c.in, got, c.want)
		}
		if !utf8.ValidString(got) {
			t.Errorf("backspace(%q) = %q is not valid UTF-8", c.in, got)
		}
	}
}

// The inspector renders headless with every capability absent, which is a
// platform it has to be honest about: each card says "unsupported here"
// rather than pretending, and the one thing every shell has (OpenURL) is
// still offered.
func TestInspectorRendersWithoutCapabilities(t *testing.T) {
	a := apptest.New(t, Root(), apptest.WithConfig(Config()), apptest.Tol(apptest.AntiAliased))
	a.Settle()
	if a.Render().Bounds().Empty() {
		t.Fatal("empty render")
	}
	a.AssertText("Capabilities")
	a.AssertText("Connectivity")
	a.AssertText("unsupported here")
	a.AssertLabel("Open the repo")
	a.AssertNoLabel("Raise keyboard")
}
