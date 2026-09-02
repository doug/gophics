package ui

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/doug/gophics/shell"
)

// fakeFolder is shell.Folder over a map, with callbacks fired inline. The real
// capability posts them to the UI goroutine; a headless test has none, and
// inline is the stricter case anyway — it runs the whole chain before the test
// resumes, so a missing callback shows up as a missing result rather than a
// pass that raced.
type fakeFolder struct {
	name     string
	files    map[string][]byte
	readErr  map[string]error // names that fail to read
	writeErr error            // every write fails
	listErr  error
	writes   []string // names written, in order
	removed  []string
}

func newFakeFolder(files map[string][]byte) *fakeFolder {
	return &fakeFolder{name: "vault", files: files, readErr: map[string]error{}}
}

func (f *fakeFolder) Name() string { return f.name }

func (f *fakeFolder) List(opts shell.FolderListOptions, done func([]shell.FolderEntry, error)) {
	if f.listErr != nil {
		done(nil, f.listErr)
		return
	}
	var out []shell.FolderEntry
	for name, b := range f.files {
		if opts.Accepts(name) {
			out = append(out, shell.FolderEntry{Name: name, Size: int64(len(b))})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	done(out, nil)
}

func (f *fakeFolder) Read(name string, done func([]byte, error)) {
	if err := f.readErr[name]; err != nil {
		done(nil, err)
		return
	}
	b, ok := f.files[name]
	if !ok {
		done(nil, errors.New("not found"))
		return
	}
	done(b, nil)
}

func (f *fakeFolder) Write(name string, data []byte, done func(error)) {
	if f.writeErr != nil {
		done(f.writeErr)
		return
	}
	f.files[name] = data
	f.writes = append(f.writes, name)
	done(nil)
}

func (f *fakeFolder) Remove(name string, done func(error)) {
	delete(f.files, name)
	f.removed = append(f.removed, name)
	done(nil)
}

// Opening a folder loads its .md files as notes, named without the extension.
func TestLoadFolderAdoptsNotes(t *testing.T) {
	_, st := mountNotes(t, t.TempDir())

	f := newFakeFolder(map[string][]byte{
		"Beta.md":   []byte("# Beta"),
		"Alpha.md":  []byte("# Alpha"),
		"notes.txt": []byte("not a note"),
	})
	loadFolder(st, f)

	v := st.W().Vault
	if !v.HasStore() {
		t.Fatal("vault has no store after loading a folder")
	}
	if v.Label() != "vault" {
		t.Errorf("Label() = %q, want the folder's name", v.Label())
	}
	if len(v.Notes) != 2 {
		t.Fatalf("loaded %d notes, want 2 — the .txt must not be one: %v", len(v.Notes), v.Notes)
	}
	if v.Notes[0].Name != "Alpha" || v.Notes[1].Name != "Beta" {
		t.Errorf("loaded %q and %q, want them sorted and without .md",
			v.Notes[0].Name, v.Notes[1].Name)
	}
	if v.Notes[0].Body != "# Alpha" {
		t.Errorf("Alpha's body is %q, want its file contents", v.Notes[0].Body)
	}
}

// One unreadable file costs the user that note, not the whole vault.
func TestLoadFolderSkipsUnreadableFiles(t *testing.T) {
	_, st := mountNotes(t, t.TempDir())

	f := newFakeFolder(map[string][]byte{
		"Alpha.md":  []byte("# Alpha"),
		"Broken.md": []byte("x"),
		"Gamma.md":  []byte("# Gamma"),
	})
	f.readErr["Broken.md"] = errors.New("permission denied")
	loadFolder(st, f)

	v := st.W().Vault
	if len(v.Notes) != 2 {
		t.Fatalf("loaded %d notes, want the 2 readable ones: %v", len(v.Notes), v.Notes)
	}
	for _, n := range v.Notes {
		if n.Name == "Broken" {
			t.Error("the unreadable note was loaded anyway")
		}
	}
}

// A folder that cannot be listed leaves the prompt up with a reason, rather
// than an empty vault that looks like an empty folder.
func TestLoadFolderReportsListFailure(t *testing.T) {
	dir := t.TempDir()
	_, st := mountNotes(t, dir)
	before := st.W().Vault.Label()

	f := newFakeFolder(nil)
	f.listErr = errors.New("nope")
	loadFolder(st, f)

	if st.storeErr == "" {
		t.Error("a folder that could not be listed reported no error to the user")
	}
	if got := st.W().Vault.Label(); got != before {
		t.Errorf("vault now points at %q, want the previous folder %q — a folder that "+
			"could not be read must not replace the one that was open", got, before)
	}
}

// Saving writes the .md file through to the folder.
func TestFolderStoreWritesThrough(t *testing.T) {
	f := newFakeFolder(map[string][]byte{})
	s := newFolderStore(f, nil)

	n, err := s.Write("Alpha", "# Alpha\n")
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if n.Path != "Alpha.md" || n.Name != "Alpha" {
		t.Errorf("Write returned %+v, want Path Alpha.md and Name Alpha", n)
	}
	if string(f.files["Alpha.md"]) != "# Alpha\n" {
		t.Errorf("folder holds %q, want the body", f.files["Alpha.md"])
	}
}

// Remove addresses the file by the note's identity, which for this store is the
// file name — deleting "Alpha" instead of "Alpha.md" silently removes nothing.
func TestFolderStoreRemovesTheFile(t *testing.T) {
	f := newFakeFolder(map[string][]byte{"Alpha.md": []byte("x")})
	s := newFolderStore(f, nil)

	if err := s.Remove(Note{Path: "Alpha.md", Name: "Alpha"}); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if len(f.removed) != 1 || f.removed[0] != "Alpha.md" {
		t.Errorf("removed %v, want [Alpha.md]", f.removed)
	}
}

// A write that fails after the fact still reaches the user. The store reports
// success immediately — it has to, the editor cannot wait for a round trip —
// so onErr is the only way a failed save is ever visible.
func TestFolderStoreReportsLateWriteFailure(t *testing.T) {
	f := newFakeFolder(map[string][]byte{})
	f.writeErr = errors.New("disk full")
	var got error
	s := newFolderStore(f, func(err error) { got = err })

	if _, err := s.Write("Alpha", "body"); err != nil {
		t.Fatalf("Write returned %v; it reports success and surfaces failures through onErr", err)
	}
	if got == nil {
		t.Fatal("a failed write was never reported")
	}
	if !strings.Contains(got.Error(), "disk full") {
		t.Errorf("reported %v, want the underlying failure", got)
	}
}

func TestNoteNameStripsExtension(t *testing.T) {
	cases := map[string]string{
		"Alpha.md":     "Alpha",
		"Alpha.MD":     "Alpha",
		"a.b.md":       "a.b",
		"README":       "README",
		"Mixed.Md":     "Mixed",
		"trailing.mdx": "trailing.mdx",
	}
	for file, want := range cases {
		if got := noteName(file); got != want {
			t.Errorf("noteName(%q) = %q, want %q", file, got, want)
		}
	}
}
