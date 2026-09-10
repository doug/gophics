//go:build !js

package desktop

import (
	"fmt"
	"strings"
)

// checkKey rejects secure-storage keys the CLI transports cannot carry. Keys
// are app-chosen names, not user data, so refusing the exotic ones is a
// compile-adjacent error rather than a runtime hazard. Shared by the keychain
// and keyring backends, which never build together but must agree.
func checkKey(key string) error {
	if key == "" || strings.ContainsAny(key, "\n\r\x00") {
		return fmt.Errorf("desktop: secure storage key %q is empty or contains a control character", key)
	}
	return nil
}
