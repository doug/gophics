//go:build !js

package desktop

import "testing"

// Desktop has no geolocation backend, and the capability must say so by being
// nil rather than by returning a Geolocation that never reports a fix.
//
// The distinction matters to an app: nil means "hide the affordance", while a
// working-looking capability that never calls back is indistinguishable from a
// user who has not answered the permission prompt yet. This test exists so that
// when a backend does land, the change is deliberate.
func TestGeolocationAbsentOnDesktop(t *testing.T) {
	if got := (&window{}).Geolocation(); got != nil {
		t.Errorf("Geolocation() = %v, want nil until a real backend exists", got)
	}
}
