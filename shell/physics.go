package shell

// ScrollPhysics is how content keeps moving after a touch fling — the platform's
// decay curve, which the widget layer imitates where the OS does not provide
// momentum itself.
//
// This exists because the measurement harness (tools/uitrace) recorded the
// same flick on two Apple platforms and got two different curves: an iPhone
// decays exponentially with τ ≈ 0.5s, a Mac trackpad with τ ≈ 0.19s and not
// quite exponentially — and Android's Scroller is a third model again, a spline
// with a friction coefficient. A user's reference for "native" is the device
// in their hand, so one constant cannot be right on more than one platform.
// gophics passes a Mac's own momentum through untouched; this type carries the
// other two.
//
// Where it comes from: a shell that knows its platform implements
// ScrollPhysicsProvider (mobile from GOOS, web from the user agent), and the
// app runner reads it once when the window is wired. An app that deliberately
// wants one identity across platforms — a game — sets app.Config.ScrollPhysics
// instead, which wins. The zero value means "whatever the platform says",
// resolving to iOS's curve where nothing says anything, because that is what
// the constants were before this type existed.
//
// It is not a capability on purpose: every platform has an answer and nothing
// calls back, the two tests the capability pattern is for. The provider
// interface is named to keep it out of capgen's <X>Window convention.
type ScrollPhysics struct {
	Model FlingModel
	// Tau is the exponential time constant in seconds, for FlingExponential.
	// Zero means 0.5 — UIKit's decelerationRate "normal" (0.998 per
	// millisecond), measured on an iOS 26 Simulator at 0.518.
	Tau float64
	// Friction is the spline model's coefficient, for FlingSpline. Zero means
	// 0.015 — Android's ViewConfiguration.getScrollFriction().
	Friction float64
}

// FlingModel selects the decay curve.
type FlingModel uint8

const (
	// FlingPlatform means "ask the shell"; the runner replaces it with the
	// platform's answer, or FlingExponential where no shell says.
	FlingPlatform FlingModel = iota
	// FlingExponential is v(t) = v0·e^(−t/Tau): UIKit's UIScrollView.
	FlingExponential
	// FlingSpline is Android's OverScroller model: a fixed position spline
	// stretched to a duration and distance that both grow with the log of the
	// release velocity, scaled by Friction.
	FlingSpline
)

// IOSScrollPhysics is UIKit's curve.
func IOSScrollPhysics() ScrollPhysics { return ScrollPhysics{Model: FlingExponential, Tau: 0.5} }

// AndroidScrollPhysics is OverScroller's curve at the platform-default friction.
func AndroidScrollPhysics() ScrollPhysics { return ScrollPhysics{Model: FlingSpline, Friction: 0.015} }

// ScrollPhysicsProvider is implemented by a Window that knows which platform
// it is on. Optional, so hosts outside this module are not broken by its
// arrival; the runner type-asserts it.
type ScrollPhysicsProvider interface {
	ScrollPhysics() ScrollPhysics
}

// Resolved fills in the defaults: FlingPlatform becomes FlingExponential, and
// zero parameters become the platform values documented on the fields.
func (p ScrollPhysics) Resolved() ScrollPhysics {
	if p.Model == FlingPlatform {
		p.Model = FlingExponential
	}
	if p.Tau <= 0 {
		p.Tau = 0.5
	}
	if p.Friction <= 0 {
		p.Friction = 0.015
	}
	return p
}
