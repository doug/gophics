package shell

// Key-value storage capability for small secrets and preferences (auth tokens,
// settings). A Window opts in by implementing StorageWindow; widgets reach it via
// ctx.SecureStorage(), nil where unsupported.
//
// "Secure" describes intent, not a universal guarantee: the desktop and mobile
// shells back this with the OS keychain/keystore, but the web shell can only use
// localStorage (origin-scoped, NOT encrypted) — don't store high-value secrets in
// a web build. All methods are synchronous; keep values small.

// StorageWindow is implemented by a Window that can persist key-value data. The
// app runner type-asserts it and publishes SecureStorage() to the widget tree.
type StorageWindow interface {
	SecureStorage() SecureStorage
}

// SecureStorage is a small persistent key-value store.
type SecureStorage interface {
	// Get returns the value for key and whether it was present.
	Get(key string) (string, bool)
	// Set stores value under key.
	Set(key, value string) error
	// Delete removes key (no error if absent).
	Delete(key string) error
}
