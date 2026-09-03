package shell

// GestureTuning is the platform's thresholds for what counts as a tap, a long
// press, and a double tap — the scalars a user's thumb has been trained on by
// every other app on the device.
//
// Measured, with tools/native-twin, not quoted: Android's ViewConfiguration
// on an emulator reports a touch slop of 8dp and a long press of 400ms; UIKit's
// headers document 10pt and 0.5s; macOS's double-click interval is 0.5s by
// default and is a system setting the user can change. gophics carried one
// set of constants — iOS's — and a hardcoded 0.3s double tap that ignored
// the Mac setting. The zero value still resolves to those, so nothing that did
// not ask has changed; a shell that knows better implements
// GestureTuningProvider, and app.Config.Gestures pins values for an app that
// wants its own.
//
// Not a capability, for the reasons ScrollPhysics is not: every platform has
// an answer and nothing calls back. The provider's name keeps it out of
// capgen's <X>Window convention.
type GestureTuning struct {
	// TouchSlop is how far a finger may move, in logical px, and still be a
	// tap or a long press. Zero means 10 — UIKit's allowableMovement.
	TouchSlop float32
	// LongPress is how long a still press is held before it is a long press,
	// in seconds. Zero means 0.5 — UIKit's minimumPressDuration.
	LongPress float64
	// DoubleTap is the longest gap between two taps that makes a double tap,
	// in seconds. Zero means 0.3 — Android's DOUBLE_TAP_TIMEOUT, which is
	// also about what iOS does in practice (UIKit does not publish one).
	DoubleTap float64
}

// IOSGestureTuning is UIKit's documented defaults.
func IOSGestureTuning() GestureTuning {
	return GestureTuning{TouchSlop: 10, LongPress: 0.5, DoubleTap: 0.3}
}

// AndroidGestureTuning is ViewConfiguration's defaults, as measured.
func AndroidGestureTuning() GestureTuning {
	return GestureTuning{TouchSlop: 8, LongPress: 0.4, DoubleTap: 0.3}
}

// GestureTuningProvider is implemented by a Window that knows its platform's
// thresholds — including a live system setting, where one exists.
type GestureTuningProvider interface {
	GestureTuning() GestureTuning
}

// Resolved fills zero fields with the documented defaults.
func (g GestureTuning) Resolved() GestureTuning {
	if g.TouchSlop <= 0 {
		g.TouchSlop = 10
	}
	if g.LongPress <= 0 {
		g.LongPress = 0.5
	}
	if g.DoubleTap <= 0 {
		g.DoubleTap = 0.3
	}
	return g
}
