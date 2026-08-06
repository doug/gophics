package shell

// Haptic plays short tactile feedback. It is an optional platform capability: a
// Window exposes it by implementing HapticWindow, and callers reach it through
// the widget layer (ctx.Haptic()), which returns nil when the running platform
// can't provide it. The web shell maps kinds to navigator.vibrate; the mobile
// hosts bridge them to the OS generators (iOS UIFeedbackGenerator, Android
// performHapticFeedback/Vibrator); desktop (GLFW) has no haptics, so ctx.Haptic
// is nil there. Play is a best-effort hint — a no-op where the device or user
// settings disable vibration.
type Haptic interface {
	// Play triggers one feedback event of the given kind. Safe to call every
	// frame's worth of taps; the platform coalesces/rate-limits as needed.
	Play(HapticKind)
}

// HapticKind names the feedback patterns, mapped per platform to the closest
// native generator. Ordered light→heavy, then the three notification patterns.
type HapticKind uint8

const (
	// HapticSelection is the lightest tick — a value changed under the finger
	// (a toggle flipping, a picker advancing). iOS: UISelectionFeedbackGenerator.
	HapticSelection HapticKind = iota
	// HapticLight is a light impact — a small UI element engaging.
	HapticLight
	// HapticMedium is a medium impact — a notable action commits (a long-press
	// firing, an item picked up).
	HapticMedium
	// HapticHeavy is a heavy impact — a large or final action.
	HapticHeavy
	// HapticSuccess, HapticWarning, and HapticError are notification patterns
	// for an operation's outcome (iOS: UINotificationFeedbackGenerator).
	HapticSuccess
	HapticWarning
	HapticError
)

// HapticWindow is implemented by a Window that can play haptics. The app runner
// type-asserts the Window to it and, when present, publishes Haptic() to the
// widget tree.
type HapticWindow interface {
	// Haptic returns the tactile-feedback capability, or nil if unavailable.
	Haptic() Haptic
}
