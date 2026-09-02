//go:build !js

// Desktop implementation of the preferences capability (shell/prefs.go): a JSON
// file under the user's config directory. No native API is needed — every desktop
// OS gives us a filesystem and a per-user config location — so this one
// implementation serves macOS, Linux and Windows alike.
package desktop

import (
	"os"
	"path/filepath"

	"github.com/doug/gophics/internal/prefs"
	"github.com/doug/gophics/shell"
)

// Preferences publishes the capability, or nil if the platform gives us nowhere
// to write (os.UserConfigDir fails when HOME/APPDATA is unset), so callers see the
// usual "unsupported here" rather than a store that silently drops writes.
func (w *window) Preferences() shell.Preferences {
	w.prefsOnce.Do(func() {
		dir, err := os.UserConfigDir()
		if err != nil {
			return
		}
		w.prefs = prefs.New(filepath.Join(dir, prefs.AppDirName(w.appID), "preferences.json"))
	})
	if w.prefs == nil {
		return nil // typed-nil guard: returning w.prefs directly would be non-nil
	}
	return w.prefs
}
