//go:build !js && !android && !ios && !gophics_term

package app

import (
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/doug/gophics/widget"
)

// devStateEnv names the file `gophics dev` uses to hand UI state from an
// exiting process to its relaunched successor. Empty/unset disables the
// feature — a shipped binary never sees it. The CLI (internal/cli) sets the
// same variable when launching the app under `gophics dev -p desktop`.
const devStateEnv = "GOPHICS_DEV_STATE"

// setupDevState enables state-preserving hot-restart when the process runs
// under `gophics dev` (which sets devStateEnv to a snapshot file path). It is
// a no-op in a shipped binary, where the variable is unset.
//
// On boot it restores any snapshot the previous process left behind, so the
// window comes back at the same page/scroll/field contents. It then installs a
// restart-signal handler: on SIGTERM/SIGINT it flags the handler and wakes a
// frame, and the next Frame snapshots current state and closes the window (see
// shellHandler.Frame). Catching the signal also keeps the default terminate
// from killing us before the snapshot is written.
func setupDevState(sh *shellHandler) {
	path := os.Getenv(devStateEnv)
	if path == "" {
		return
	}
	sh.devStatePath = path
	restoreDevState(sh, path)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, os.Interrupt)
	go func() {
		<-sig
		sh.devQuit.Store(true)
		sh.core.Owner.RequestFrameThreadSafe() // wake a frame to run the snapshot
	}()
}

func restoreDevState(sh *shellHandler, path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return // no prior state — first launch of the dev session
	}
	var snap widget.StateSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		log.Printf("gophics dev: restore state: %v", err)
		return
	}
	sh.core.Owner.RestoreState(snap)
}
