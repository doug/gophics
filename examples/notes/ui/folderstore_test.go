package ui

import (
	"errors"
	"sort"
	"strings"
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/shell"
)

// fakeFolder is shell.Folder over a map, with callbacks fired inline. The real
// capability posts them to the UI goroutine; a headless test has none, and
// inline is the stricter case anyway — it runs the whole chain before the test
// resumes, so a missing callback shows up as a missing result rather than a
// pass that raced.
type fakeFolder struct {
	name      string
	files     map[string][]byte
	readErr   map[string]error // names that fail to read
	writeErr  error            // every write fails
	removeErr error            // every removal fails
	listErr   error
	writes    []string // names written, in order
	removed   []string
	// late queues the outcome of every write and removal until flush. That
	// is the browser's order — the call returns, the app acts on it, the
	// bytes land or do not — and the tests of what the app does about a
	// write that fails after it was reported done need exactly that order.
	late    bool
	pending []func()
}

// settle delivers a write or removal's outcome: now, or at flush when late.
func (f *fakeFolder) settle(fn func()) {
	if f.late {
		f.pending = append(f.pending, fn)
		return
	}
	fn()
}

// flush delivers every queued outcome, in order.
func (f *fakeFolder) flush() {
	for len(f.pending) > 0 {
		fn := f.pending[0]
		f.pending = f.pending[1:]
		fn()
	}
}

func newFakeFolder(files map[string][]byte) *fakeFolder {
	return &fakeFolder{name: "vault", files: files, readErr: map[string]error{}}
}

func (f *fakeFolder) Name() string  { return f.name }
func (f *fakeFolder) Token() string { return "token:" + f.name }

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
	err := f.writeErr
	f.settle(func() {
		if err != nil {
			done(err)
			return
		}
		f.files[name] = data
		f.writes = append(f.writes, name)
		done(nil)
	})
}

func (f *fakeFolder) Remove(name string, done func(error)) {
	err := f.removeErr
	f.settle(func() {
		if err != nil {
			done(err)
			return
		}
		delete(f.files, name)
		f.removed = append(f.removed, name)
		done(nil)
	})
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

	n, err := s.Create("Alpha", "# Alpha\n")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if n.Path != "Alpha.md" || n.Name != "Alpha" {
		t.Errorf("Create returned %+v, want Path Alpha.md and Name Alpha", n)
	}
	if string(f.files["Alpha.md"]) != "# Alpha\n" {
		t.Errorf("folder holds %q, want the body", f.files["Alpha.md"])
	}
	// An existing note is addressed by its own Path, whatever case the
	// extension was found in — not by rebuilding the file name.
	if err := s.Write(Note{Path: "Beta.MD", Name: "Beta"}, "edited"); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if string(f.files["Beta.MD"]) != "edited" {
		t.Errorf("Write went to %v, want Beta.MD", f.files)
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
	var got []lateResult
	s := newFolderStore(f, func(r lateResult) { got = append(got, r) })

	if _, err := s.Create("Alpha", "body"); err != nil {
		t.Fatalf("Create returned %v; it reports success and surfaces failures through done", err)
	}
	if len(got) != 1 || got[0].Err == nil {
		t.Fatalf("a failed write was reported as %+v", got)
	}
	if r := got[0]; !strings.Contains(r.Err.Error(), "disk full") || r.Note.Name != "Alpha" || r.Body != "body" || r.Removed {
		t.Errorf("reported %+v, want the underlying failure with the note and body it was for", r)
	}

	// Success is reported too — it is what clears a standing message.
	f.writeErr = nil
	if err := s.Write(Note{Path: "Alpha.md", Name: "Alpha"}, "better"); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[1].Err != nil || got[1].Body != "better" {
		t.Errorf("a write that landed was reported as %+v", got[1:])
	}
	if err := s.Remove(Note{Path: "Alpha.md", Name: "Alpha"}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || !got[2].Removed || got[2].Note.Name != "Alpha" {
		t.Errorf("a removal was reported as %+v", got[2:])
	}
}

// A name the folder refuses is refused by Create, not reported late: the
// capability would have said so inline, through done, before the vault had
// added a note the folder was never going to hold.
func TestFolderStoreRefusesABadNameUpFront(t *testing.T) {
	f := newFakeFolder(map[string][]byte{})
	reported := 0
	s := newFolderStore(f, func(lateResult) { reported++ })

	if _, err := s.Create("meeting: notes", "body"); err == nil {
		t.Fatal("a name with a colon was accepted")
	}
	if reported != 0 || len(f.writes) != 0 {
		t.Errorf("the refused name was still written (%v) or reported (%d)", f.writes, reported)
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

// fakePicker is shell.FolderPicker over a map of tokens, with a per-token
// outcome so a test can stage "granted", "needs permission", and "gone".
type fakePicker struct {
	folders map[string]shell.Folder
	errs    map[string]error
	opened  shell.Folder
	openErr error
	asked   int // Restore calls, to check the retry actually re-asks
}

func (p *fakePicker) Open(done func(shell.Folder, error)) { done(p.opened, p.openErr) }

func (p *fakePicker) Restore(token string, done func(shell.Folder, error)) {
	p.asked++
	if err := p.errs[token]; err != nil {
		done(nil, err)
		return
	}
	done(p.folders[token], nil) // nil for an unknown token: gone, not an error
}

// A folder that is still granted comes back on its own, with no prompt.
func TestRestoreReopensRememberedFolder(t *testing.T) {
	_, st := mountNotes(t, t.TempDir())
	f := newFakeFolder(map[string][]byte{"Kept.md": []byte("# Kept")})
	p := &fakePicker{folders: map[string]shell.Folder{"token:vault": f}}
	prefs := apptest.NewPrefs(nil)
	_ = prefs.Set(prefFolderToken, "token:vault")

	restoreFolder(p, prefs, st)

	v := st.W().Vault
	if len(v.Notes) != 1 || v.Notes[0].Name != "Kept" {
		t.Fatalf("restored vault holds %v, want the remembered folder's note", v.Notes)
	}
	if st.reopen {
		t.Error("a folder that restored cleanly still asked to be reopened")
	}
}

// A lapsed grant is not a dead end: the app offers a button, and pressing it
// asks again. Without the reopen state the user sees a bare "Open folder…" and
// has to find the vault themselves, which is the whole thing this avoids.
func TestRestoreOffersReopenWhenPermissionLapsed(t *testing.T) {
	_, st := mountNotes(t, t.TempDir())
	p := &fakePicker{errs: map[string]error{"token:vault": shell.ErrFolderPermission}}
	prefs := apptest.NewPrefs(nil)
	_ = prefs.Set(prefFolderToken, "token:vault")

	restoreFolder(p, prefs, st)
	if !st.reopen {
		t.Fatal("a lapsed permission left no way to reopen the folder")
	}
	if _, ok := prefs.Get(prefFolderToken); !ok {
		t.Error("the token was forgotten; a lapsed grant means ask again, not give up")
	}

	restoreFolder(p, prefs, st) // what the reopen button does
	if p.asked != 2 {
		t.Errorf("Restore was called %d times, want a second ask from the retry", p.asked)
	}
}

// A vault that has moved or gone is forgotten, so the app stops offering to
// reopen something that is not there.
func TestRestoreForgetsAMissingFolder(t *testing.T) {
	_, st := mountNotes(t, t.TempDir())
	p := &fakePicker{} // no folder under any token
	prefs := apptest.NewPrefs(nil)
	_ = prefs.Set(prefFolderToken, "token:gone")

	restoreFolder(p, prefs, st)

	if _, ok := prefs.Get(prefFolderToken); ok {
		t.Error("a folder that is gone is still remembered")
	}
	if st.reopen {
		t.Error("offered to reopen a folder that no longer exists")
	}
}

// Nothing remembered means nothing happens — no error, no prompt, no call.
func TestRestoreWithNoRememberedFolderIsQuiet(t *testing.T) {
	_, st := mountNotes(t, t.TempDir())
	p := &fakePicker{}

	restoreFolder(p, apptest.NewPrefs(nil), st)

	if p.asked != 0 {
		t.Errorf("Restore was called %d times with nothing remembered", p.asked)
	}
	if st.storeErr != "" || st.reopen {
		t.Errorf("a first run reported storeErr=%q reopen=%v, want silence", st.storeErr, st.reopen)
	}
}

// remember stores the token the picker will be given back next launch.
func TestRememberStoresTheToken(t *testing.T) {
	prefs := apptest.NewPrefs(nil)
	f := newFakeFolder(nil)

	remember(prefs, f)

	got, ok := prefs.Get(prefFolderToken)
	if !ok || got != f.Token() {
		t.Errorf("remembered %q (present=%v), want %q", got, ok, f.Token())
	}
}
