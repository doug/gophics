//go:build !js

package desktop

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/doug/gophics/shell"
)

// await1 runs one callback-shaped folder call and waits for it, so a test reads
// top to bottom. Real callers never do this — the callback arrives on the UI
// goroutine — but a test has no UI goroutine to arrive on.
func await1[T any](t *testing.T, call func(done func(T, error))) (T, error) {
	t.Helper()
	type res struct {
		v   T
		err error
	}
	ch := make(chan res, 1)
	call(func(v T, err error) { ch <- res{v, err} })
	select {
	case r := <-ch:
		return r.v, r.err
	case <-time.After(10 * time.Second):
		t.Fatal("folder call never called back")
		panic("unreachable")
	}
}

func syncErr(t *testing.T, call func(done func(error))) error {
	t.Helper()
	_, err := await1(t, func(done func(struct{}, error)) {
		call(func(err error) { done(struct{}{}, err) })
	})
	return err
}

func tempFolder(t *testing.T) osFolder {
	t.Helper()
	return osFolder{path: t.TempDir()}
}

func TestFolderWriteReadRoundTrip(t *testing.T) {
	f := tempFolder(t)
	if err := syncErr(t, func(d func(error)) { f.Write("note.md", []byte("hello"), d) }); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := await1(t, func(d func([]byte, error)) { f.Read("note.md", d) })
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != "hello" {
		t.Errorf("Read returned %q, want %q", got, "hello")
	}
}

// Bytes, not text. A vault holds whatever the user put in it, and a backend
// that round-trips through a string corrupts everything that is not UTF-8.
func TestFolderCarriesBinary(t *testing.T) {
	f := tempFolder(t)
	want := []byte{0x00, 0xff, 0xfe, 0x80, 0x01}
	if err := syncErr(t, func(d func(error)) { f.Write("blob.bin", want, d) }); err != nil {
		t.Fatalf("Write: %v", err)
	}
	got, err := await1(t, func(d func([]byte, error)) { f.Read("blob.bin", d) })
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("Read returned % x, want % x", got, want)
	}
}

// List reports files sorted, skips directories, and applies the filter.
func TestFolderList(t *testing.T) {
	f := tempFolder(t)
	for _, n := range []string{"b.md", "a.md", "c.txt"} {
		if err := os.WriteFile(filepath.Join(f.path, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(f.path, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}

	all, err := await1(t, func(d func([]shell.FolderEntry, error)) {
		f.List(shell.FolderListOptions{}, d)
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List returned %d entries, want 3 files (the directory must not be listed): %v", len(all), all)
	}
	if all[0].Name != "a.md" || all[1].Name != "b.md" || all[2].Name != "c.txt" {
		t.Errorf("List returned %v, want them sorted by name", all)
	}
	if all[0].Size != 1 {
		t.Errorf("entry size is %d, want 1", all[0].Size)
	}

	md, err := await1(t, func(d func([]shell.FolderEntry, error)) {
		f.List(shell.FolderListOptions{Accept: []string{".md"}}, d)
	})
	if err != nil {
		t.Fatalf("filtered List: %v", err)
	}
	if len(md) != 2 {
		t.Errorf("filtered List returned %v, want only the .md files", md)
	}
}

// Removing what is not there is not an error — shell.Folder documents it, and
// it matches Preferences.Delete and SecureStorage.Delete.
func TestFolderRemoveIsIdempotent(t *testing.T) {
	f := tempFolder(t)
	if err := syncErr(t, func(d func(error)) { f.Write("gone.md", []byte("x"), d) }); err != nil {
		t.Fatal(err)
	}
	if err := syncErr(t, func(d func(error)) { f.Remove("gone.md", d) }); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := syncErr(t, func(d func(error)) { f.Remove("gone.md", d) }); err != nil {
		t.Errorf("removing an absent file reported %v, want nil", err)
	}
}

// The name check is enforced at the backend, not merely documented: this is the
// difference between a folder the user granted and the disk around it.
func TestFolderRejectsEscapingNames(t *testing.T) {
	f := tempFolder(t)
	outside := filepath.Join(filepath.Dir(f.path), "escaped.md")

	if err := syncErr(t, func(d func(error)) { f.Write("../escaped.md", []byte("x"), d) }); err == nil {
		t.Error("Write accepted a name escaping the folder")
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatalf("Write created %s outside the folder", outside)
	}
	if _, err := await1(t, func(d func([]byte, error)) { f.Read("../escaped.md", d) }); err == nil {
		t.Error("Read accepted a name escaping the folder")
	}
	if err := syncErr(t, func(d func(error)) { f.Remove("../escaped.md", d) }); err == nil {
		t.Error("Remove accepted a name escaping the folder")
	}
}

// A failed write must not destroy what was there. os.WriteFile truncates before
// it writes, which is why this goes through a temporary file and a rename.
func TestFolderWriteReplacesAtomically(t *testing.T) {
	f := tempFolder(t)
	path := filepath.Join(f.path, "note.md")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := syncErr(t, func(d func(error)) { f.Write("note.md", []byte("replaced"), d) }); err != nil {
		t.Fatalf("Write: %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "replaced" {
		t.Errorf("file holds %q, want %q", got, "replaced")
	}
	// The temporary file is the mechanism, not a leftover.
	ents, err := os.ReadDir(f.path)
	if err != nil {
		t.Fatal(err)
	}
	if len(ents) != 1 {
		t.Errorf("folder holds %d entries after one write, want 1 — a temporary file was left behind", len(ents))
	}
}

func TestFolderNameIsTheDirectoryName(t *testing.T) {
	f := osFolder{path: filepath.Join("home", "doug", "vault")}
	if got := f.Name(); got != "vault" {
		t.Errorf("Name() = %q, want the last path element", got)
	}
}
