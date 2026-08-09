//go:build !js

package desktop

import "github.com/doug/gophics/shell"

// Geolocation satisfies shell.GeolocationWindow. There is no desktop location
// backend yet, so the capability is honestly absent — Geolocation returns nil and
// callers hide the affordance (ctx.Geolocation() == nil).
func (w *window) Geolocation() shell.Geolocation {
	// TODO(platform): CoreLocation/geoclue/Win32/FusedLocation
	return nil
}
