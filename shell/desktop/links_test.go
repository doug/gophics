//go:build !js

package desktop

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The launch argument is the whole desktop deep-link story, and it had no
// test. url.Parse reads a Windows drive path as a URL with a one-letter
// scheme, so "C:\Users\doug\notes.txt" came back raw instead of as file://
// — or at all, when the file did not exist.
func TestInitialURL(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(existing, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "gone.txt")

	// What an existing path normalizes to. Checked for shape below, since the
	// temp dir's spelling is the platform's.
	fileURL := fileURL(existing)
	if !strings.HasPrefix(fileURL, "file:///") {
		t.Errorf("fileURL(%q) = %q, want a file:/// URL with an empty host", existing, fileURL)
	}
	if u, err := url.Parse(fileURL); err != nil || u.Host != "" || filepath.FromSlash(u.Path) != filepath.Clean("/"+strings.TrimPrefix(filepath.ToSlash(existing), "/")) {
		t.Errorf("fileURL(%q) = %q does not round-trip to the path (host %q, path %q, err %v)",
			existing, fileURL, u.Host, u.Path, err)
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no arguments", []string{"app"}, ""},
		{"nil", nil, ""},
		{"empty argument skipped", []string{"app", "", "https://x.test/a"}, "https://x.test/a"},
		{"custom scheme", []string{"app", "myapp://open/42"}, "myapp://open/42"},
		{"https", []string{"app", "https://x.test/a?b=c"}, "https://x.test/a?b=c"},
		{"existing file becomes file://", []string{"app", existing}, fileURL},
		{"missing file is nothing", []string{"app", missing}, ""},
		{"a flag is nothing", []string{"app", "-v"}, ""},
		{"a drive-letter path that does not exist is not a URL", []string{"app", `C:\Users\doug\notes.txt`}, ""},
		{"a forward-slash drive path that does not exist is not a URL", []string{"app", "C:/Users/doug/notes.txt"}, ""},
		{"file:// is not returned raw", []string{"app", "file:///nowhere"}, ""},
		{"first match wins", []string{"app", "-v", "myapp://a", "https://b"}, "myapp://a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := initialURL(tc.args); got != tc.want {
				t.Errorf("initialURL(%q) = %q, want %q", tc.args, got, tc.want)
			}
		})
	}

	// A relative path that exists resolves through the working directory.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	got := initialURL([]string{"app", "notes.txt"})
	if !strings.HasPrefix(got, "file://") || !strings.HasSuffix(got, "/notes.txt") {
		t.Errorf("relative existing path gave %q, want a file:// URL ending in /notes.txt", got)
	}
}

// The window's OpenURL refuses before it reaches the platform opener; the
// accepted schemes are shell.CheckOpenURL's and are tested there. Anything
// refused must come back as an error and not as a launched process.
func TestOpenURLRefusesWhatTheContractRefuses(t *testing.T) {
	w := &window{}
	for _, u := range []string{"file:///etc/passwd", "javascript:alert(1)", "myapp://x", "", "notaurl"} {
		if err := w.OpenURL(u); err == nil {
			t.Errorf("OpenURL(%q) = nil, want a refusal", u)
		}
	}
}
