package shell

// Unified permission capability. Individual capabilities carry their own
// Authorize (Camera, Notifier); this adds a single place to query/request the
// common runtime permissions uniformly. A Window opts in by implementing
// PermissionWindow; widgets reach it via ctx.Permissions(), nil where
// unsupported. Callbacks fire on the UI goroutine.

// PermissionKind names a runtime permission.
type PermissionKind uint8

const (
	PermCamera PermissionKind = iota
	PermMicrophone
	PermLocation
	PermNotifications
)

// PermissionWindow is implemented by a Window that can query/request runtime
// permissions. The app runner type-asserts it and publishes Permissions().
type PermissionWindow interface {
	Permissions() Permissions
}

// Permissions queries and requests runtime permissions. (Reuses the Permission
// outcome type — PermissionPrompt, Granted, Denied — from media.go.)
type Permissions interface {
	// Status reports the current state without prompting (PermissionPrompt when
	// undecided or unknowable).
	Status(PermissionKind) Permission
	// Request prompts for kind if needed and reports the outcome.
	Request(PermissionKind, func(Permission))
}
