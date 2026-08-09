package mobile

import "github.com/doug/gophics/shell"

// Geolocation makes the Bridge a shell.GeolocationWindow. There is no mobile
// location backend yet, so the capability is honestly absent — Geolocation
// returns nil and callers hide the affordance (ctx.Geolocation() == nil).
func (b *Bridge) Geolocation() shell.Geolocation {
	// TODO(platform): CoreLocation / FusedLocationProvider, driven from the host
	// over the Bridge like the media capability.
	return nil
}
