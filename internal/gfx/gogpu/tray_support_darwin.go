//go:build darwin && !ios

package gogpu

// TraySupported reports whether this build has a tray backend.
func TraySupported() bool { return true }
