package shell

// Battery capability: report charge level and charging state, and notify on
// change. A Window opts in by implementing BatteryWindow; widgets reach it via
// ctx.Battery(), nil where unsupported. Callbacks fire on the UI goroutine.

// BatteryWindow is implemented by a Window that can report battery status. The
// app runner type-asserts it and publishes Battery() to the widget tree — the
// same shape as NotifyWindow/HapticWindow.
type BatteryWindow interface {
	Battery() Battery
}

// Battery reports the device battery state.
type Battery interface {
	// Level is the current charge as a fraction in [0,1] (1 == full).
	Level() float32
	// Charging reports whether the device is currently charging (or on external
	// power).
	Charging() bool
	// OnChange registers f, called whenever the level or charging state changes.
	// It takes no argument — read Level()/Charging() for the new values.
	OnChange(func())
}
