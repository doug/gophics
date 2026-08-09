package shell

// Geolocation capability. A Window exposes it by implementing GeolocationWindow;
// callers reach it through the widget layer (ctx.Geolocation()), which returns
// nil when the running platform can't provide it. Only the web shell implements
// it today; the same interface is the contract the mobile (CoreLocation /
// FusedLocationProvider) and desktop (CoreLocation / geoclue / Win32) shells
// will satisfy. All callbacks fire on the UI goroutine.
//
// A location fix may prompt the user for permission the first time it is
// requested; the platform owns that flow, and a denial surfaces as an error to
// Current (or as Watch simply never firing).

// GeolocationWindow is implemented by a Window that can report the device
// location. The app runner type-asserts the Window to it and, when present,
// publishes Geolocation() to the widget tree.
type GeolocationWindow interface {
	// Geolocation returns the location capability, or nil if unavailable.
	Geolocation() Geolocation
}

// Geolocation reports the device's position. Coordinates are WGS-84 degrees;
// accuracy is the radius of the 95% confidence circle in meters.
type Geolocation interface {
	// Current requests a single position fix. On success err is nil and the
	// callback carries the coordinates; on failure (permission denied,
	// unavailable, timeout) lat/lon/accuracy are zero and err is set.
	Current(func(lat, lon, accuracy float64, err error))
	// Watch subscribes to position updates, invoking the callback each time the
	// fix changes, and returns a cancel func that stops the subscription (and
	// releases any platform resources). Errors during a watch are not reported;
	// the callback simply doesn't fire.
	Watch(func(lat, lon, accuracy float64)) (cancel func())
}
