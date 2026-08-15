//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && !android && !js

package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/doug/gophics/shell"
)

func TestGlobFor(t *testing.T) {
	cases := map[string]string{
		".epub":                "*.epub",
		"csv":                  "*.csv",
		"image/*":              "", // no glob equivalent: accept everything
		"application/epub+zip": "", // no reliable extension mapping
		"":                     "",
		"   .png   ":           "*.png",
	}
	for in, want := range cases {
		if got := globFor(in); got != want {
			t.Errorf("globFor(%q) = %q, want %q", in, got, want)
		}
	}
}

// A filter that hides the file the user came to open is worse than no filter,
// so an Accept list that maps to nothing must widen rather than narrow.
func TestFilterWidensWhenNothingMaps(t *testing.T) {
	if got := zenityFilter([]string{"image/*"}); got != "" {
		t.Errorf("zenityFilter = %q, want empty (no --file-filter passed)", got)
	}
	if got := kdialogFilter([]string{"image/*"}); got != "*|All files" {
		t.Errorf("kdialogFilter = %q, want the all-files filter", got)
	}
}

func TestSplitSelection(t *testing.T) {
	cases := []struct {
		name     string
		out      string
		multiple bool
		want     []string
	}{
		{"single", "/home/a/one.txt\n", false, []string{"/home/a/one.txt"}},
		{"single ignores pipes", "/home/a/one|two.txt", false, []string{"/home/a/one|two.txt"}},
		{"zenity multiple", "/a/one.txt|/a/two.txt", true, []string{"/a/one.txt", "/a/two.txt"}},
		{"kdialog multiple", "/a/one.txt\n/a/two.txt\n", true, []string{"/a/one.txt", "/a/two.txt"}},
		{"cancelled", "", true, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := splitSelection(c.out, c.multiple)
			if len(got) != len(c.want) {
				t.Fatalf("got %q, want %q", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Errorf("got %q, want %q", got, c.want)
				}
			}
		})
	}
}

func TestZenityOpenArgs(t *testing.T) {
	args := zenityOpenArgs(shell.OpenOptions{Accept: []string{".epub"}, Multiple: true})
	var haveSep, haveFilter bool
	for _, a := range args {
		if a == "--separator=|" {
			haveSep = true
		}
		if a == "--file-filter=*.epub" {
			haveFilter = true
		}
	}
	// Without an explicit separator zenity's default is also "|", but relying
	// on a default that a fork may change is how multi-select silently breaks.
	if !haveSep {
		t.Errorf("multiple select passed no separator: %q", args)
	}
	if !haveFilter {
		t.Errorf("accept list produced no filter: %q", args)
	}
}

// A save that fails halfway must not leave the user's existing file truncated.
func TestWriteAtomicReplacesWholeFile(t *testing.T) {
	dir := t.TempDir()
	name := filepath.Join(dir, "ledger.beancount")
	if err := os.WriteFile(name, []byte("old contents, longer than the new"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeAtomic(name, []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("contents = %q, want %q", got, "new")
	}
	// The temp file must not survive.
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Errorf("directory has %d entries, want 1 (a temp file leaked)", len(ents))
	}
}
