//go:build !js

// Desktop implementation of the preferences capability (shell/prefs.go): a JSON
// file under the user's config directory. No native API is needed — every desktop
// OS gives us a filesystem and a per-user config location — so this one
// implementation serves macOS, Linux and Windows alike.
package desktop

import (
	"os"
	"path/filepath"
	"strings"

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
		w.prefs = prefs.New(filepath.Join(dir, appDirName(w.appID), "preferences.json"))
	})
	if w.prefs == nil {
		return nil // typed-nil guard: returning w.prefs directly would be non-nil
	}
	return w.prefs
}

// appDirName sanitizes an app identifier into a single directory name, falling
// back to the executable's name so an app that sets no identifier still gets a
// stable, recognizable location instead of a shared one.
func appDirName(id string) string {
	if id == "" {
		if exe, err := os.Executable(); err == nil {
			id = filepath.Base(exe)
		}
	}
	if id == "" {
		id = "gophics-app"
	}
	// Keep it a plain single path element.
	id = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		case r == '-', r == '_', r == '.':
			return r
		case r == ' ':
			return '-'
		default:
			return -1
		}
	}, id)
	if id == "" || id == "." || id == ".." {
		id = "gophics-app"
	}
	return id
}
