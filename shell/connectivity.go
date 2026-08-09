package shell

// Connectivity capability: report network reachability and notify on change. A
// Window opts in by implementing ConnectivityWindow; widgets reach it via
// ctx.Connectivity(), nil where unsupported. Callbacks fire on the UI goroutine.

// ConnectivityWindow is implemented by a Window that can report network status.
// The app runner type-asserts it and publishes Connectivity() to the widget
// tree — the same shape as NotifyWindow/HapticWindow.
type ConnectivityWindow interface {
	Connectivity() Connectivity
}

// Connectivity reports whether the device currently has network access.
type Connectivity interface {
	// Online reports whether the device currently believes it has network
	// access. It is a best-effort hint (the browser's navigator.onLine, the OS
	// reachability flag) — not a guarantee that any given host is reachable.
	Online() bool
	// OnChange registers f, called whenever the online state flips; the argument
	// is the new state. Some platforms cannot observe changes and never invoke f
	// (see the per-platform docs) — treat a never-firing OnChange as "unknown",
	// not "always online".
	OnChange(func(online bool))
}
