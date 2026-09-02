//go:build !js

package desktop

import (
	"path/filepath"

	"github.com/doug/gophics/internal/prefs"
	"strings"
	"testing"
)

func TestAppDirName(t *testing.T) {
	// Dots survive because reverse-DNS identifiers need them; separators do not.
	cases := map[string]string{
		"com.example.tally": "com.example.tally",
		"My App":            "My-App",
		"weird/../name":     "weird..name", // separators stripped, dots kept
		"a b/c":             "a-bc",
	}
	for in, want := range cases {
		if got := prefs.AppDirName(in); got != want {
			t.Errorf("prefs.AppDirName(%q) = %q, want %q", in, got, want)
		}
	}

	// The property that actually matters: whatever an app passes, the result is a
	// single, non-traversing path element — never empty, never "." or "..", never
	// containing a separator. Otherwise an AppID could redirect where settings are
	// written.
	for _, in := range []string{
		"", ".", "..", "../..", "/", "/etc/passwd", `..\..\windows`,
		"...", "  ", "///", "a/../../b", "\x00nul",
	} {
		got := prefs.AppDirName(in)
		if got == "" || got == "." || got == ".." {
			t.Errorf("prefs.AppDirName(%q) = %q, want a usable name", in, got)
		}
		if filepath.Base(got) != got || filepath.IsAbs(got) {
			t.Errorf("prefs.AppDirName(%q) = %q, want a single relative path element", in, got)
		}
		if strings.ContainsAny(got, `/\`) {
			t.Errorf("prefs.AppDirName(%q) = %q, contains a path separator", in, got)
		}
	}
}

// TestPreferencesTypedNil guards the classic Go trap: returning a nil *filePrefs
// through the shell.Preferences interface would produce a non-nil interface, so
// `ctx.Preferences() != nil` would be true and callers would crash on first use.
func TestPreferencesTypedNil(t *testing.T) {
	w := &window{}
	w.prefsOnce.Do(func() {}) // consume the once so prefs stays nil
	if got := w.Preferences(); got != nil {
		t.Errorf("Preferences() = %v (typed nil leaked through the interface), want nil", got)
	}
}
