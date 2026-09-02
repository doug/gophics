// Terminal implementation of the preferences capability (shell/prefs.go).
//
// It is the desktop implementation with the serial numbers intact: the same
// JSON file in the same per-app directory under the user's config dir, through
// the same internal/prefs store. Deliberately the same, not merely similar — a
// TUI build and a desktop build of one app are the same program to the user,
// and "the terminal version forgot my settings" is the bug this file exists to
// prevent. The shared AppDirName is what keeps the two from ever sanitizing an
// app identifier differently and splitting into two directories.
package terminal

import (
	"os"
	"path/filepath"

	"github.com/doug/gophics/internal/prefs"
	"github.com/doug/gophics/shell"
)

// Preferences publishes the capability, or nil when the platform gives us
// nowhere to write (os.UserConfigDir fails when HOME is unset — a stripped
// container, say), so callers see the usual "unsupported here" rather than a
// store that silently drops writes.
func (w *window) Preferences() shell.Preferences {
	w.ts.prefsOnce.Do(func() {
		dir, err := os.UserConfigDir()
		if err != nil {
			return
		}
		w.ts.prefs = prefs.New(filepath.Join(dir, prefs.AppDirName(w.ts.appID), "preferences.json"))
	})
	if w.ts.prefs == nil {
		return nil // typed-nil guard, same as the desktop shell
	}
	return w.ts.prefs
}
