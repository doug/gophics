package shell

import "testing"

// A name is a file in the folder, never a path out of it. The desktop backend
// joins these onto a real directory, so a name that escapes is arbitrary
// filesystem access with the user's own permissions.
func TestCheckFolderNameRejectsEscapes(t *testing.T) {
	bad := []struct{ name, why string }{
		{"", "empty"},
		{".", "the folder itself"},
		{"..", "the parent"},
		{"../secrets", "a relative escape"},
		{"a/b.md", "a subpath"},
		{"/etc/passwd", "an absolute path"},
		{`..\secrets`, "a Windows relative escape"},
		{`sub\note.md`, "a Windows subpath"},
		{"C:note.md", "a Windows drive-relative path"},
		{"note.md:stream", "an NTFS alternate data stream"},
		{"note\x00.md", "a NUL truncation"},
	}
	for _, c := range bad {
		if err := CheckFolderName(c.name); err == nil {
			t.Errorf("CheckFolderName(%q) accepted %s", c.name, c.why)
		}
	}
}

// Backslash and colon are refused on every platform, not only Windows. A vault
// is a folder of real files that gets synced and opened elsewhere, so a name
// that is harmless on a Mac and a path on Windows should never be creatable.
func TestCheckFolderNameIsPlatformIndependent(t *testing.T) {
	for _, name := range []string{`a\b`, "a:b"} {
		if err := CheckFolderName(name); err == nil {
			t.Errorf("CheckFolderName(%q) accepted a name that is a path on Windows", name)
		}
	}
}

func TestCheckFolderNameAcceptsOrdinaryNames(t *testing.T) {
	for _, name := range []string{"note.md", "a note with spaces.md", ".hidden", "ünïcode.md", "no-extension"} {
		if err := CheckFolderName(name); err != nil {
			t.Errorf("CheckFolderName(%q) = %v, want nil", name, err)
		}
	}
}

// The filter lives on the options type so every backend applies the same one.
// A backend matching case-sensitively would show "NOTE.MD" on one platform and
// hide it on another, from the same folder.
func TestFolderListOptionsAccepts(t *testing.T) {
	cases := []struct {
		accept []string
		name   string
		want   bool
	}{
		{nil, "anything.bin", true},
		{[]string{}, "anything.bin", true},
		{[]string{".md"}, "note.md", true},
		{[]string{".md"}, "NOTE.MD", true},
		{[]string{".md"}, "Note.Md", true},
		{[]string{".md"}, "note.txt", false},
		{[]string{"md"}, "note.md", true},  // a missing dot is tolerated
		{[]string{".md"}, "readme", false}, // no extension at all
		{[]string{".md", ".txt"}, "note.txt", true},
		{[]string{""}, "note.md", false}, // an empty entry matches nothing
		{[]string{".md"}, ".md", true},   // a file actually named ".md"
	}
	for _, c := range cases {
		o := FolderListOptions{Accept: c.accept}
		if got := o.Accepts(c.name); got != c.want {
			t.Errorf("FolderListOptions{%v}.Accepts(%q) = %v, want %v", c.accept, c.name, got, c.want)
		}
	}
}
