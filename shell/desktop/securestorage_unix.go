//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && !android && !js

// Linux/BSD implementation of the secure-storage capability (shell/storage.go)
// over secret-tool(1), libsecret's CLI to the session's Secret Service (GNOME
// Keyring, KWallet). Shelling out rather than speaking the D-Bus Secret
// Service protocol directly: the protocol requires a session-encryption
// handshake that is a project in itself, and secret-tool reads the secret from
// stdin — never argv — which is the property that matters.
//
// Published only when secret-tool is installed and nil otherwise, the same
// bargain as the file chooser: callers use nil to hide the affordance, which
// beats a store that errors on every call.
package desktop

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/doug/gophics/internal/prefs"
	"github.com/doug/gophics/shell"
)

func (w *window) SecureStorage() shell.SecureStorage {
	if _, err := exec.LookPath("secret-tool"); err != nil {
		return nil
	}
	return unixKeyring{service: "gophics." + prefs.AppDirName(w.appID)}
}

type unixKeyring struct{ service string }

func (k unixKeyring) Get(key string) (string, bool) {
	if checkKey(key) != nil {
		return "", false
	}
	out, err := exec.Command("secret-tool", "lookup", "service", k.service, "key", key).Output()
	if err != nil {
		return "", false // exit 1 is "not found"
	}
	return string(out), true
}

func (k unixKeyring) Set(key, value string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	c := exec.Command("secret-tool", "store",
		"--label", k.service+"/"+key, "service", k.service, "key", key)
	c.Stdin = strings.NewReader(value)
	if out, err := c.CombinedOutput(); err != nil {
		return fmt.Errorf("desktop: keyring set %q: %v: %s", key, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (k unixKeyring) Delete(key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	// Clearing an absent key exits non-zero; the contract says that is fine.
	_ = exec.Command("secret-tool", "clear", "service", k.service, "key", key).Run()
	return nil
}

// checkKey rejects keys the CLI transport cannot carry; shared shape with the
// darwin backend, duplicated because the two files never build together.
func checkKey(key string) error {
	if key == "" || strings.ContainsAny(key, "\n\r\x00") {
		return fmt.Errorf("desktop: secure storage key %q is empty or contains a control character", key)
	}
	return nil
}
