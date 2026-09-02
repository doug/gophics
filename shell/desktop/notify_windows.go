//go:build windows

package desktop

import "github.com/doug/gophics/shell"

// Notifier is nil on Windows for now. Toast notifications require a WinRT
// activation path (an AppUserModelID and the ToastNotificationManager) that
// the PowerShell subprocess route can technically reach but only with a
// registered AUMID — unbundled binaries get a notification that is accepted
// and never shown, the silent failure the macOS backend's comment describes.
// nil is the detectable answer until the real integration exists.
func (w *window) Notifier() shell.Notifier { return nil }
