//go:build js || android || ios || gossamer_term

package app

// setupDevState is desktop-only; other platforms iterate via web live-reload or
// device builds, so state-preserving hot-restart is a no-op here.
func setupDevState(*shellHandler) {}
