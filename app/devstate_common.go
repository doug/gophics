package app

import (
	"encoding/json"
	"log"
	"os"

	"github.com/doug/gophics/widget"
)

// The platform-independent half of state-preserving hot restart.
//
// What a hand-off is made of is the framework's business, not a platform's:
// the same widget.StateSnapshot, keyed by position in the widget tree, moves
// between a desktop process and its successor, between an app and its
// reinstalled self on a phone, and across a browser reload. Only three things
// differ per platform — where the snapshot is kept, who decides when to write
// it, and how the successor is told to look — so only those live in the
// devstate_<platform>.go files.

// restoreDevSnapshot reads a hand-off file into the live tree.
//
// A missing file is the ordinary first launch of a dev session, so it is
// silent. Anything else is logged: a file that exists and cannot be read or
// parsed is a bug in the hand-off, not a state the loop should swallow.
func restoreDevSnapshot(sh *shellHandler, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("gophics dev: read state: %v", err)
		}
		return
	}
	restoreDevSnapshotBytes(sh, data)
}

// restoreDevSnapshotBytes restores an already-loaded snapshot, for platforms
// whose hand-off is not a file (web uses sessionStorage).
func restoreDevSnapshotBytes(sh *shellHandler, data []byte) {
	var snap widget.StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("gophics dev: restore state: %v", err)
		return
	}
	sh.core.Owner.RestoreState(snap)
}
