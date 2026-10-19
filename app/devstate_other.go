//go:build gophics_term || android || ios

package app

// setupDevState is the entry point for a platform whose own process boots the
// app, so it has nothing to do here.
//
// A terminal build has no hot-restart loop (`gophics dev -p terminal` falls
// through to the desktop path, which replaces the process and therefore the
// terminal the app is drawing in). A mobile build never calls Run at all: the
// native host owns the process, so the mobile hand-off hangs off the Bridge's
// lifecycle instead — see devstate_mobile.go and shell/mobile/devstate.go.
func setupDevState(*shellHandler) {}
