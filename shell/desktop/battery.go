//go:build !js && !linux && !windows && !darwin

// Fallback for platforms with no battery implementation. Linux reads sysfs
// (battery_linux.go), Windows calls GetSystemPowerStatus (battery_windows.go)
// and macOS goes through IOKit (battery_darwin.go); this covers the rest —
// the BSDs, mainly, where the interface is per-OS (sysctl hw.acpi on FreeBSD,
// apm on OpenBSD) and nobody has asked yet.
//
// Returning nil is the honest answer: ctx.Battery() is nil, so callers hide the
// affordance rather than rendering a fabricated 100%.

package desktop

import "github.com/doug/gophics/shell"

// Battery makes the desktop window a shell.BatteryWindow. It returns nil here.
// TODO(platform): the BSDs — FreeBSD sysctl hw.acpi.battery, OpenBSD apm.
func (w *window) Battery() shell.Battery { return nil }
