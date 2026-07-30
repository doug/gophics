package ui

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
)

// Config returns the app's window/runtime configuration. It lives in this
// importable package (alongside Root) so `gossamer dev --hot ./examples/hn/ui`
// can build a wrapper plugin around Root and Config.
func Config() app.Config {
	return app.Config{
		Title:        "gossamer · hn",
		Size:         geom.Size{W: 480, H: 720},
		Background:   Background(),
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}
}
