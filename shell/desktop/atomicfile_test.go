//go:build !js

package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// A saved document must be readable by the same people a plain save would
// let read it. The temp file came from os.CreateTemp, which is 0600 by
// design, and that mode survived the rename — so every save was user-private
// regardless of the umask. The reference is a direct 0644 write in the same
// directory, which is what the umask leaves of 0644 on this machine.
func TestWriteFileAtomicNewFileModeMatchesDirectWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not bind here")
	}
	dir := t.TempDir()
	ref := filepath.Join(dir, "ref.txt")
	if err := os.WriteFile(ref, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "out.txt")
	if err := writeFileAtomic(path, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	want := mode(t, ref)
	if got := mode(t, path); got != want {
		t.Errorf("new file has mode %o, want %o (what a direct 0644 write gets here)", got, want)
	}
}

// Replacing a file keeps the mode the user gave it — a vault made
// group-readable stays that way, and one locked down stays locked down.
func TestWriteFileAtomicKeepsExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits do not bind here")
	}
	for _, perm := range []os.FileMode{0o600, 0o664} {
		path := filepath.Join(t.TempDir(), "out.txt")
		if err := os.WriteFile(path, []byte("old"), perm); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, perm); err != nil { // past the umask
			t.Fatal(err)
		}
		if err := writeFileAtomic(path, []byte("new")); err != nil {
			t.Fatal(err)
		}
		if got := mode(t, path); got != perm {
			t.Errorf("replaced file has mode %o, want the %o it had", got, perm)
		}
	}
}

func mode(t *testing.T, path string) os.FileMode {
	t.Helper()
	st, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return st.Mode().Perm()
}

func TestWriteFileAtomicRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.txt")
	if err := writeFileAtomic(path, []byte("hello")); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "hello" {
		t.Fatalf("read %q, %v", b, err)
	}
}

// Replacing an existing file must at no point leave the path without a file.
// This can only assert the end state; the no-gap property is the absence of
// the delete-then-rename the Windows path used to do, and the rename's
// replace semantics are Go's on every platform.
func TestWriteFileAtomicReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAtomic(path, []byte("new")); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != "new" {
		t.Errorf("file holds %q, want %q", b, "new")
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != 1 {
		t.Errorf("dir holds %d entries, want 1 — a temp file leaked", len(ents))
	}
}

// A directory that refuses a temp file still gets the direct-write fallback,
// and a directory that refuses everything reports the failure rather than
// pretending.
func TestWriteFileAtomicUnwritableDirFails(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("permission bits do not bind here")
	}
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })
	if err := writeFileAtomic(filepath.Join(dir, "out.txt"), []byte("x")); err == nil {
		t.Error("writing into an unwritable directory reported success")
	}
}
