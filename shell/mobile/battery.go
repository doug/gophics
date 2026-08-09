package mobile

import "github.com/doug/gophics/shell"

// Battery makes the Bridge a shell.BatteryWindow. It returns nil today, leaving
// ctx.Battery() nil so callers hide any battery affordance.
// TODO(platform): read OS battery over the host bridge (Android BatteryManager,
// iOS UIDevice.batteryLevel/batteryState) via the poll/drain pattern used by
// TakeHaptic.
func (b *Bridge) Battery() shell.Battery { return nil }
