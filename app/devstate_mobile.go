//go:build android || ios

package app

import (
	"log"
	"os"
)

// The app's half of mobile hot restart. shell/mobile's Bridge finds these two
// methods by type assertion (see shell/mobile/devstate.go for why the loop is
// shaped this way) and calls them from the host UI thread — the same thread
// that drives frames, so the tree is never mid-build here and no signal
// handler or frame hook is involved.
//
// Exported only because the interface naming them lives in another package.
// They are not API an app calls, and nothing happens unless the binary was
// linked by `gophics dev`: the Bridge will not have a path to hand over.

// DevRestoreState adopts the state the previous process left behind, so the
// relaunched app comes back on the same screen.
func (h *shellHandler) DevRestoreState(path string) {
	restoreDevSnapshot(h, path)
	// A hand-off is read once. Leaving the file behind would make a crashing
	// app restore the same poisoned state on every relaunch, and would
	// resurrect a dev session's state in an app the developer later started by
	// tapping its icon — the file outlives the session, because the private
	// directory is what survives the reinstall that makes this work at all.
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		log.Printf("gophics dev: clear state: %v", err)
	}
}

// DevSnapshotState writes the current UI state out for the process that is
// about to replace this one.
//
// Nothing to say when the tree holds no serializable state: an empty snapshot
// would overwrite a good one with nothing if the app were backgrounded twice,
// and the restore side treats a missing file as a first launch anyway.
func (h *shellHandler) DevSnapshotState(path string) {
	snap := h.core.Owner.SnapshotState()
	if len(snap) == 0 {
		return
	}
	if err := writeDevSnapshot(path, snap); err != nil {
		log.Printf("gophics dev: snapshot state: %v", err)
	}
}
