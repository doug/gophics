package shell

// The screen-wake capability: keep the display on while something is happening
// that the user is watching rather than touching.
//
// Every platform dims and then locks the screen after an idle timeout measured
// from the last input event, which is correct almost always and wrong for a
// specific, recognisable set of apps: a recipe on a kitchen counter, a route on
// a dashboard, a score being read from a stand, a video. Those look idle to the
// OS and are not idle to the person.
//
// It is deliberately a lease rather than a setting. Acquire returns a release
// function, so the wake is scoped to the thing that needed it and cannot outlive
// it by being forgotten — the failure mode of a boolean flag is a flat battery
// hours later, in a screen the user has long since navigated away from.
//
// A Window opts in by implementing WakeLockWindow; widgets reach it via
// ctx.WakeLock(), nil where unsupported.

// WakeLockWindow is implemented by a Window that can hold the screen awake.
type WakeLockWindow interface {
	WakeLock() WakeLock
}

// WakeLock keeps the display from sleeping.
type WakeLock interface {
	// Acquire asks the platform to keep the screen on and returns the release.
	//
	// Calling release more than once is safe and does nothing after the first.
	// Multiple simultaneous leases are allowed: the screen stays awake until the
	// last one is released, so two widgets can each hold one without either
	// having to know about the other.
	//
	// reason is a short label for platforms that surface one (and for logging);
	// it is never shown to the user as UI.
	Acquire(reason string) (release func())
	// Held reports whether any lease is currently outstanding.
	Held() bool
}
