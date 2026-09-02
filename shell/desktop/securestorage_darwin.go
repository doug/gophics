//go:build darwin && !ios && !js

// macOS implementation of the secure-storage capability (shell/storage.go),
// backed by the login keychain through security(1).
//
// The subprocess, not the Security.framework API, for the same reason the
// Linux file chooser shells out to zenity: the framework route is a pile of
// CoreFoundation object plumbing through dlsym for three operations a token
// store performs a handful of times per session. security(1) ships with every
// macOS and talks to the same keychain.
//
// Commands go through `security -i`, which reads them from stdin — never argv,
// where the secret would be readable in the process table for the life of the
// call. Values are base64-encoded before storing so arbitrary strings survive
// the CLI round-trip byte-for-byte; the cost is that entries read through
// Keychain Access show the encoding, which for a machine-written token store
// is acceptable.
package desktop

import (
	"encoding/base64"
	"fmt"
	"os/exec"
	"strings"

	"github.com/doug/gophics/internal/prefs"
	"github.com/doug/gophics/shell"
)

func (w *window) SecureStorage() shell.SecureStorage {
	if _, err := exec.LookPath("security"); err != nil {
		return nil
	}
	return macKeychain{service: "gophics." + prefs.AppDirName(w.appID)}
}

type macKeychain struct{ service string }

// quoteSec quotes s for security(1)'s own command tokenizer.
func quoteSec(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// checkKey rejects keys the CLI transport cannot carry. Keys are app-chosen
// names, not user data, so refusing the exotic ones is a compile-adjacent
// error rather than a runtime hazard.
func checkKey(key string) error {
	if key == "" || strings.ContainsAny(key, "\n\r\x00") {
		return fmt.Errorf("desktop: secure storage key %q is empty or contains a control character", key)
	}
	return nil
}

func (k macKeychain) run(cmd string) (string, error) {
	c := exec.Command("security", "-i")
	c.Stdin = strings.NewReader(cmd + "\n")
	out, err := c.CombinedOutput()
	return string(out), err
}

func (k macKeychain) Get(key string) (string, bool) {
	if checkKey(key) != nil {
		return "", false
	}
	// -w prints only the password. A missing item exits non-zero (errSecItemNotFound).
	out, err := k.run(fmt.Sprintf("find-generic-password -s %s -a %s -w",
		quoteSec(k.service), quoteSec(key)))
	if err != nil {
		return "", false
	}
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(out))
	if err != nil {
		return "", false // not one of ours; treat as absent rather than corrupt
	}
	return string(b), true
}

func (k macKeychain) Set(key, value string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	enc := base64.StdEncoding.EncodeToString([]byte(value))
	// -U updates in place instead of failing on a duplicate item.
	out, err := k.run(fmt.Sprintf("add-generic-password -U -s %s -a %s -w %s",
		quoteSec(k.service), quoteSec(key), quoteSec(enc)))
	if err != nil {
		return fmt.Errorf("desktop: keychain set %q: %v: %s", key, err, strings.TrimSpace(out))
	}
	return nil
}

func (k macKeychain) Delete(key string) error {
	if err := checkKey(key); err != nil {
		return err
	}
	// Deleting an absent key is not an error, matching the contract.
	_, _ = k.run(fmt.Sprintf("delete-generic-password -s %s -a %s",
		quoteSec(k.service), quoteSec(key)))
	return nil
}
