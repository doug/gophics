// Package gallerymobile is the gomobile-bind surface for the widget catalog.
//
// It holds only what is the gallery's own, which is building the tree.
// Everything generic — the frame loop, input, lifecycle, accessibility — is on
// shell/mobile.Bridge, which the CLI binds alongside this package, so a host
// calls those methods on the Bridge that Start returns.
package gallerymobile

import (
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/gallery/ui"
	"github.com/doug/gophics/shell/mobile"
)

// Start builds the app and returns the bridge the host drives it through.
//
// Call it once, before anything else. On failure it returns a nil bridge and
// the error to show — two results because the second is an error, which is the
// one shape gomobile allows.
func Start() (*mobile.Bridge, error) {
	h, err := app.NewHandler(ui.Gallery{}, ui.Config())
	if err != nil {
		return nil, err
	}
	return mobile.NewBridge(h), nil
}
