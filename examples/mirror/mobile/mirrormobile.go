// Package mirrormobile is the gomobile-bind surface for the mirror app.
//
// It holds only what is mirror's own. Everything generic — the frame loop,
// input, lifecycle, accessibility, and the camera and microphone host
// registration — lives in shell/mobile/bind, which the host binds alongside
// this package:
//
//	gophics run -platform android -host ./android ./mobile
//
// gomobile cannot bind package main and carries only a restricted vocabulary
// across the boundary, so the app itself lives in ../ui and this is the thin
// adapter over it.
package mirrormobile

import (
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/mirror/ui"
	"github.com/doug/gophics/shell/mobile"
	"github.com/doug/gophics/shell/mobile/bind"
)

// Start builds the app and must be called once from the host before anything
// else, including anything in the bind package. It returns "" on success, or
// the error to show.
func Start() string {
	h, err := app.NewHandler(ui.App{}, ui.Config())
	if err != nil {
		return err.Error()
	}
	bind.Attach(mobile.NewBridge(h))
	return ""
}
