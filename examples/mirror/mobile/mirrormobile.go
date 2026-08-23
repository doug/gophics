// Package mirrormobile is the gomobile-bind surface for the mirror app.
//
// It holds only what is mirror's own. Everything generic — the frame loop,
// input, lifecycle, accessibility, and registering the camera and microphone
// backends — is on shell/mobile.Bridge, which the CLI binds alongside this
// package, so a host calls those methods on the Bridge that Start returns.
//
// gomobile cannot bind package main and carries only a restricted vocabulary
// across the boundary, so the app itself lives in ../ui and this is the thin
// adapter over it.
package mirrormobile

import (
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/mirror/ui"
	"github.com/doug/gophics/shell/mobile"
)

// Start builds the app and returns the bridge the host drives it through.
//
// Call it once, before anything else. On failure it returns a nil bridge and
// the error to show — two results because the second is an error, which is the
// one shape gomobile allows.
func Start() (*mobile.Bridge, error) {
	h, err := app.NewHandler(ui.App{}, ui.Config())
	if err != nil {
		return nil, err
	}
	return mobile.NewBridge(h), nil
}
