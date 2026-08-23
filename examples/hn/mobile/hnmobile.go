// Package hnmobile is the gomobile-bind surface for the hn app.
//
// It holds only what is hn's own, which is nothing but building the tree.
// Everything generic — the frame loop, input, lifecycle, accessibility — is on
// shell/mobile.Bridge, which the CLI binds alongside this package, so a host
// calls those methods on the Bridge that Start returns.
//
// gomobile cannot bind package main and carries only a restricted vocabulary
// across the boundary, so this thin adapter is what a host talks to.
package hnmobile

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/shell/mobile"
)

// Start builds the app and returns the bridge the host drives it through.
//
// Call it once, before anything else. On failure it returns a nil bridge and
// the error to show — two results because the second is an error, which is the
// one shape gomobile allows.
func Start() (*mobile.Bridge, error) {
	root, bg := scene()
	h, err := app.NewHandler(root, app.Config{
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
		Background:   bg,
	})
	if err != nil {
		return nil, err
	}
	return mobile.NewBridge(h), nil
}
