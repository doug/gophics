//go:build darwin && !ios && !js

// macOS implementation of the notification capability (shell/notify.go), via
// osascript's `display notification`.
//
// Not UNUserNotificationCenter, deliberately: the real API only delivers for
// bundled, signed apps with a registered identifier, and a gophics binary run
// from a terminal or a build directory is neither — the notification would be
// accepted and never shown, with no error to say so. osascript posts through
// Script Editor's registration, which exists on every macOS install, so it
// works for exactly the unbundled binaries a developer actually runs. An app
// that ships as a signed .app can grow the real integration later without the
// contract changing.
//
// Notification.Tag is accepted and ignored: AppleScript's notification verb
// has no replace semantics, so tagged notifications stack instead of
// coalescing. That is a degradation the contract's caller survives — the tag
// is a de-duplication nicety — where refusing the capability entirely would
// not be.
package desktop

import (
	"os/exec"
	"strings"

	"github.com/doug/gophics/shell"
)

func (w *window) Notifier() shell.Notifier {
	if _, err := exec.LookPath("osascript"); err != nil {
		return nil
	}
	return macNotifier{}
}

type macNotifier struct{}

// Authorize reports granted: osascript needs no app-level grant (the user's
// Notification Center settings for Script Editor are the actual gate, and
// there is no API to query them from here).
func (macNotifier) Authorize(cb func(shell.Permission)) {
	if cb == nil {
		return
	}
	cb(shell.PermissionGranted)
}

func (macNotifier) Notify(n shell.Notification) {
	script := "display notification " + appleScriptString(n.Body) +
		" with title " + appleScriptString(n.Title)
	// Fire and forget on a goroutine: Notify is synchronous in the contract
	// but the subprocess is not worth blocking the UI goroutine for.
	go func() { _ = exec.Command("osascript", "-e", script).Run() }()
}

// appleScriptString quotes s as an AppleScript string literal. Backslash and
// double quote are the only escapes AppleScript recognizes; newlines are legal
// inside its string literals as-is.
func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
