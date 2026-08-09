package shell

// Lifecycle capability: report the app's foreground/background run state and
// notify on change. A Window opts in by implementing LifecycleWindow; widgets
// reach it via ctx.Lifecycle(), nil where unsupported. Callbacks fire on the UI
// goroutine.
//
// The three states form a coarse ladder shared across platforms:
//   - StateActive     — foreground and receiving input (the normal running state).
//   - StateInactive   — foreground but not the input focus (another window/app is
//                        frontmost, a system overlay is up, a desktop window lost
//                        keyboard focus). Still visible; keep animating.
//   - StateBackground — not visible (tab hidden, app sent to the background).
//                        A good moment to pause work and persist state.
//
// Not every platform can distinguish all three (see the per-platform docs); a
// platform reports the finest state it can observe and never invents one.

// LifecycleWindow is implemented by a Window that can report app run state. The
// app runner type-asserts it and publishes Lifecycle() to the widget tree — the
// same shape as ConnectivityWindow/HapticWindow.
type LifecycleWindow interface {
	Lifecycle() Lifecycle
}

// AppState is the app's coarse foreground/background run state, ordered as a
// ladder: Active (0) < Inactive (1) < Background (2).
type AppState uint8

const (
	// StateActive is foreground and focused: the normal running state.
	StateActive AppState = iota
	// StateInactive is foreground but not the input focus (transient overlay,
	// another window frontmost, desktop keyboard focus lost). Still visible.
	StateInactive
	// StateBackground is not visible: tab hidden or app sent to the background.
	StateBackground
)

// String renders the state for logs and tests.
func (s AppState) String() string {
	switch s {
	case StateActive:
		return "active"
	case StateInactive:
		return "inactive"
	case StateBackground:
		return "background"
	default:
		return "unknown"
	}
}

// Lifecycle reports the current run state and notifies on transitions.
type Lifecycle interface {
	// State returns the current run state. It is always readable, even on
	// platforms whose OnChange never fires.
	State() AppState
	// OnChange registers f, called with the new state on every transition. Some
	// platforms cannot observe every transition (see the per-platform docs) and
	// may never invoke f — treat a never-firing OnChange as "no finer signal
	// available", reading State() when you need the current value.
	OnChange(func(AppState))
}
