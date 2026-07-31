package ui

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
)

// Config returns the app's window/runtime configuration. It lives alongside
// Root in this importable package so both are reusable and testable apart from
// the entry point.
func Config() app.Config {
	return app.Config{
		Title:        "gossamer · hn",
		Size:         geom.Size{W: 480, H: 720},
		Background:   Background(),
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}
}
