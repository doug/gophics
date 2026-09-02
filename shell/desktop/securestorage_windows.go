//go:build windows

package desktop

import "github.com/doug/gophics/shell"

// SecureStorage is nil on Windows for now. The Credential Manager
// (CredWrite/CredRead) is the right backing and reachable without CGo through
// windows.NewLazyDLL — the same route the GPU dialog code already takes — but
// it has not been built, and nil is the honest answer that lets an app hide
// the affordance rather than offer a store that errors.
func (w *window) SecureStorage() shell.SecureStorage { return nil }
