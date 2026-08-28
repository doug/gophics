package mobile

import (
	"errors"

	"github.com/doug/gophics/shell"
)

// SecureStorage over the Keychain / Keystore.
//
// This is the one capability here whose contract is synchronous: Get returns a
// value, it does not take a callback, so the drain-and-answer shape the media
// capabilities use cannot serve it. gomobile does bind host methods that return
// values, so the host is called straight through and blocks the UI goroutine
// for the length of a keychain lookup.
//
// That is acceptable *here* and nowhere else on this bridge: keychain reads are
// local and sub-millisecond, an app reads a token once at startup rather than
// per frame, and the alternative — caching every secret in Go memory so reads
// can be synchronous — is exactly what a keychain exists to avoid.

// SecureHost is the platform keychain, implemented by the host.
//
// All three run on the UI thread and must return promptly. A host that needs to
// prompt (biometric-gated items) should fail the call rather than block: an app
// can retry, but a frozen UI is not recoverable.
type SecureHost interface {
	// SecureGet returns the value for key, or "" when absent.
	SecureGet(key string) string
	// SecureHas reports whether key exists, distinguishing a stored empty
	// string from a missing entry — which SecureGet alone cannot.
	SecureHas(key string) bool
	// SecureSet stores value under key, returning "" or an error message.
	SecureSet(key, value string) string
	// SecureDelete removes key, returning "" or an error message. Removing an
	// absent key is not an error.
	SecureDelete(key string) string
}

// SetSecureHost registers the keychain backend, enabling ctx.SecureStorage().
func (b *Bridge) SetSecureHost(h SecureHost) { b.secureHost = h }

// SecureStorage makes the Bridge a shell.StorageWindow.
func (b *Bridge) SecureStorage() shell.SecureStorage {
	if b.secureHost == nil {
		return nil
	}
	return mobileSecure{b}
}

type mobileSecure struct{ b *Bridge }

func (s mobileSecure) Get(key string) (string, bool) {
	if !s.b.secureHost.SecureHas(key) {
		return "", false
	}
	return s.b.secureHost.SecureGet(key), true
}

func (s mobileSecure) Set(key, value string) error {
	if msg := s.b.secureHost.SecureSet(key, value); msg != "" {
		return errors.New(msg)
	}
	return nil
}

func (s mobileSecure) Delete(key string) error {
	if msg := s.b.secureHost.SecureDelete(key); msg != "" {
		return errors.New(msg)
	}
	return nil
}
