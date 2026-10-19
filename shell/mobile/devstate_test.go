package mobile

import (
	"path/filepath"
	"testing"

	"github.com/doug/gophics/shell"
)

// devRecorder is a handler that can snapshot, and remembers being asked.
type devRecorder struct {
	eventRecorder
	restored []string
	saved    []string
}

func (r *devRecorder) DevRestoreState(path string)  { r.restored = append(r.restored, path) }
func (r *devRecorder) DevSnapshotState(path string) { r.saved = append(r.saved, path) }

// withDevRestart turns the link-time gate on for one test and puts it back.
func withDevRestart(t *testing.T) {
	t.Helper()
	prev := devRestart
	devRestart = "1"
	t.Cleanup(func() { devRestart = prev })
}

// A shipped build must not write a dev-state file.
//
// This is the whole of the gate, so it is worth asserting directly rather than
// trusting that nobody adds a second way in. `gophics build` and `gophics run`
// never pass the linker flag, so an app on a user's phone goes to the
// background and leaves nothing behind.
func TestDevStateOffWithoutTheLinkerFlag(t *testing.T) {
	rec := &devRecorder{}
	b := NewBridge(rec)
	b.SetFilesDir(t.TempDir())
	b.SetAppState(int(shell.StateBackground))
	if len(rec.restored) != 0 || len(rec.saved) != 0 {
		t.Errorf("restored %v, saved %v in a non-dev build; both must be empty",
			rec.restored, rec.saved)
	}
}

// The dev build restores when the host says where to write, and snapshots on
// the way to the background — which is the whole hand-off, in the order the
// loop depends on.
func TestDevStateRestoresAndSnapshots(t *testing.T) {
	withDevRestart(t)
	dir := t.TempDir()
	want := filepath.Join(dir, devStateFile)

	rec := &devRecorder{}
	b := NewBridge(rec)
	b.SetFilesDir(dir)
	if len(rec.restored) != 1 || rec.restored[0] != want {
		t.Fatalf("restored %v, want one call for %s — the restore has to happen "+
			"when the path arrives, which is before the first frame", rec.restored, want)
	}

	b.SetAppState(int(shell.StateInactive))
	if len(rec.saved) != 0 {
		t.Errorf("snapshotted on inactive (%v); a dialog over the app is not a "+
			"restart, and writing then would overwrite good state with a frame "+
			"the developer never saw", rec.saved)
	}
	b.SetAppState(int(shell.StateBackground))
	if len(rec.saved) != 1 || rec.saved[0] != want {
		t.Fatalf("saved %v, want one call for %s", rec.saved, want)
	}
}

// Without a writable directory there is no hand-off, and that must be silent
// rather than a guessed path: HOME is the app sandbox on iOS and generally
// unset on Android, so deriving one in Go would work on one platform only.
func TestDevStateWithoutFilesDir(t *testing.T) {
	withDevRestart(t)
	rec := &devRecorder{}
	b := NewBridge(rec)
	b.SetAppState(int(shell.StateBackground))
	if len(rec.saved) != 0 {
		t.Errorf("saved %v with no files dir", rec.saved)
	}
}

// A handler that cannot snapshot opts out by not having the methods. The
// Bridge is driven by test fakes and by one real app handler, and the fakes
// must not have to care that this feature exists.
func TestDevStateIgnoresAHandlerThatCannotSnapshot(t *testing.T) {
	withDevRestart(t)
	b := NewBridge(&eventRecorder{})
	b.SetFilesDir(t.TempDir())
	if got := b.devStatePath(); got != "" {
		t.Errorf("devStatePath = %q for a handler with no snapshot methods", got)
	}
	b.SetAppState(int(shell.StateBackground)) // must not panic
}
