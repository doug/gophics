package shell

// Local-notification capability: post a system notification. A Window opts in by
// implementing NotifyWindow; widgets reach it via ctx.Notifier(), nil where
// unsupported. Callbacks fire on the UI goroutine.

// NotifyWindow is implemented by a Window that can post local notifications. The
// app runner type-asserts it and publishes Notifier() to the widget tree.
type NotifyWindow interface {
	Notifier() Notifier
}

// Notification is a local (on-device) notification.
type Notification struct {
	Title string
	Body  string
	// Tag coalesces notifications: posting a new one with the same non-empty tag
	// replaces the previous instead of stacking.
	Tag string
}

// Notifier posts local notifications after authorization.
type Notifier interface {
	// Authorize requests permission to post notifications.
	Authorize(func(Permission))
	// Notify posts n. A no-op if permission was not granted.
	Notify(n Notification)
}
