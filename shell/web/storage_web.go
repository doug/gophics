//go:build js && wasm

// Web implementation of the shell key-value storage capability (shell/storage.go)
// using localStorage. NOTE: localStorage is origin-scoped but NOT encrypted — the
// "secure" in the interface is honoured by the native shells' keychain/keystore,
// not here; the shell/storage.go doc calls this out.

package web

import (
	"fmt"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// SecureStorage returns a store only when localStorage is available (it isn't in
// some private-mode / sandboxed contexts).
func (w *window) SecureStorage() shell.SecureStorage {
	ls := js.Global().Get("localStorage")
	if ls.IsUndefined() || ls.IsNull() {
		return nil
	}
	return &webStorage{ls: ls}
}

type webStorage struct{ ls js.Value }

func (s *webStorage) Get(key string) (string, bool) {
	v := s.ls.Call("getItem", key)
	if v.IsNull() {
		return "", false
	}
	return v.String(), true
}

func (s *webStorage) Set(key, value string) (err error) {
	// setItem throws on quota-exceeded / disabled storage; a JS throw surfaces as
	// a Go panic, so recover it into an error rather than crashing the app.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("web: storage set %q: %v", key, r)
		}
	}()
	s.ls.Call("setItem", key, value)
	return nil
}

func (s *webStorage) Delete(key string) error {
	s.ls.Call("removeItem", key)
	return nil
}
