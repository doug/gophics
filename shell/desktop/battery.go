//go:build !js

// Desktop stub for the shell battery capability (shell/battery.go). Battery()
// returns nil, leaving ctx.Battery() nil so callers hide any battery affordance
// — the correct nil-where-unsupported signal until the OS power APIs are wired.

package desktop

import "github.com/doug/gophics/shell"

// Battery makes the desktop window a shell.BatteryWindow. It returns nil today.
// TODO(platform): read OS battery (IOKit IOPowerSources on macOS, upower/sysfs
// on Linux, Win32 GetSystemPowerStatus on Windows).
func (w *window) Battery() shell.Battery { return nil }
