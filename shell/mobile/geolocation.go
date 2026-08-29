package mobile

import (
	"errors"

	"github.com/doug/gophics/shell"
)

// LocationHost is the platform location service, implemented by the host
// (iOS CoreLocation, Android FusedLocationProviderClient).
//
// Permission is the host's business: both platforms prompt on first use and
// remember the answer, so a denied request simply fails the pending call rather
// than needing a separate Authorize round trip.
type LocationHost interface {
	// StartLocation begins delivering fixes for reqID. watch distinguishes a
	// one-shot read from a subscription: a one-shot may stop itself after the
	// first fix, a watch runs until StopLocation.
	//
	// Answer with DeliverLocation(reqID, ...) — repeatedly for a watch — or
	// FailLocation(reqID, msg).
	StartLocation(reqID int, watch bool)
	// StopLocation ends a watch and releases the platform's location hardware.
	StopLocation(reqID int)
}

// SetLocationHost registers the location backend, enabling ctx.Geolocation().
func (b *Bridge) SetLocationHost(h LocationHost) { b.locHost = h; b.capabilitiesChanged() }

// Geolocation makes the Bridge a shell.GeolocationWindow.
func (b *Bridge) Geolocation() shell.Geolocation {
	if b.locHost == nil {
		return nil
	}
	return mobileGeo{b}
}

type mobileGeo struct{ b *Bridge }

func (g mobileGeo) Current(done func(lat, lon, accuracy float64, err error)) {
	b := g.b
	id := b.newReq()
	if done != nil {
		if b.locOnce == nil {
			b.locOnce = map[int]func(float64, float64, float64, error){}
		}
		b.locOnce[id] = done
	}
	b.locHost.StartLocation(id, false)
}

func (g mobileGeo) Watch(fn func(lat, lon, accuracy float64)) (cancel func()) {
	b := g.b
	id := b.newReq()
	if fn != nil {
		if b.locWatch == nil {
			b.locWatch = map[int]func(float64, float64, float64){}
		}
		b.locWatch[id] = fn
	}
	b.locHost.StartLocation(id, true)
	return func() {
		if _, live := b.locWatch[id]; !live {
			return // already cancelled; stopping twice would unbalance the host
		}
		delete(b.locWatch, id)
		b.locHost.StopLocation(id)
	}
}

// DeliverLocation reports a fix. A one-shot request is completed by the first
// one; a watch keeps receiving them until it is cancelled.
func (b *Bridge) DeliverLocation(reqID int, lat, lon, accuracy float64) {
	if cb := b.locOnce[reqID]; cb != nil {
		delete(b.locOnce, reqID)
		cb(lat, lon, accuracy, nil)
		// A one-shot host may keep the hardware running; tell it to stop.
		b.locHost.StopLocation(reqID)
		return
	}
	if fn := b.locWatch[reqID]; fn != nil {
		fn(lat, lon, accuracy)
	}
}

// FailLocation reports that a location request could not be served — permission
// denied, location services off, or no fix available.
func (b *Bridge) FailLocation(reqID int, msg string) {
	if cb := b.locOnce[reqID]; cb != nil {
		delete(b.locOnce, reqID)
		cb(0, 0, 0, errors.New(msg))
		return
	}
	// A watch has nowhere to report an error — its callback carries no error —
	// so a failure ends the subscription rather than silently delivering
	// nothing forever.
	delete(b.locWatch, reqID)
}
