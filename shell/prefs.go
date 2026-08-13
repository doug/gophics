package shell

// Preferences is the capability for ordinary, non-secret application settings —
// window state, the last opened document, a chosen theme. A Window opts in by
// implementing PreferencesWindow; widgets reach it via ctx.Preferences(), nil
// where unsupported.
//
// Why this is separate from SecureStorage: they look alike but are backed very
// differently. SecureStorage means the OS keychain/keystore, which exists to guard
// secrets and can prompt the user for access; routing a window size or a file path
// through it is both wrong and user-hostile. Preferences is plain, never prompts,
// and is always available where the platform can persist anything at all:
//
//	Preferences    settings you'd be happy to see in a plain-text file
//	SecureStorage  tokens, passwords, keys
//
// Desktop backs this with a JSON file under the user's config directory; the web
// shell uses localStorage; mobile will use UserDefaults / SharedPreferences.
//
// All methods are synchronous — reads come from an in-memory cache, so they are
// cheap enough to call from Build. Keep values small; this is not a database.

// PreferencesWindow is implemented by a Window that can persist settings. The app
// runner type-asserts it and publishes Preferences() to the widget tree.
type PreferencesWindow interface {
	Preferences() Preferences
}

// Preferences is a small persistent string key-value store.
type Preferences interface {
	// Get returns the value for key and whether it was present.
	Get(key string) (string, bool)
	// Set stores value under key, persisting it.
	Set(key, value string) error
	// Delete removes key (no error if absent).
	Delete(key string) error
	// Keys returns the stored keys, sorted, so a caller can enumerate or clear
	// its own namespace without tracking what it wrote.
	Keys() []string
}
