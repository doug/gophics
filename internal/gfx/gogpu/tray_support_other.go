//go:build !(darwin && !ios)

package gogpu

// TraySupported reports whether this build has a tray backend. Windows
// (Shell_NotifyIcon) and Linux (StatusNotifierItem over D-Bus) are both
// reachable from pure Go and neither is written yet.
func TraySupported() bool { return false }
