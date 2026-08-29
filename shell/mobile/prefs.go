package mobile

import (
	"path/filepath"

	"github.com/doug/gophics/internal/prefs"
	"github.com/doug/gophics/shell"
)

// Preferences makes the Bridge a shell.PreferencesWindow.
//
// This is the same JSON-file store the desktop uses (internal/prefs), not
// UserDefaults or SharedPreferences, and that is a deliberate choice rather
// than a shortcut. The capability is a small string map that never prompts;
// both platforms give the app a private writable directory, so one
// implementation covers desktop and mobile and the two cannot disagree about
// what "the same setting" means. Reaching for the native stores would buy
// nothing an app can observe through this interface, and would cost two more
// host round trips per read.
//
// It does need one thing only the host knows: where to write. Go cannot ask —
// os.UserConfigDir reads HOME, which iOS sets to the app sandbox but Android
// generally does not, so deriving the path in Go would work on one platform and
// silently fail on the other. So the host passes it in (SetFilesDir), and until
// it does, the capability is nil rather than a store that drops writes.
func (b *Bridge) Preferences() shell.Preferences {
	if b.prefs == nil {
		return nil // typed-nil guard: returning b.prefs directly would be non-nil
	}
	return b.prefs
}

// SetFilesDir gives the bridge the app's private writable directory, enabling
// the Preferences capability. Hosts call it once, before Start's bridge is
// driven: iOS passes NSDocumentDirectory, Android context.getFilesDir().
//
// Passing "" disables persistence again, which is what a host should do rather
// than guess a path it is not sure about — an app that can tell Preferences is
// nil hides the affordance, while a store that accepts writes and loses them is
// undetectable.
func (b *Bridge) SetFilesDir(dir string) {
	if dir == "" {
		b.prefs = nil
		b.capabilitiesChanged()
		return
	}
	b.prefs = prefs.New(filepath.Join(dir, "preferences.json"))
	b.capabilitiesChanged()
}
