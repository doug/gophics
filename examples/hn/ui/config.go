package ui

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
)

// Config returns the app's window/runtime configuration. It lives alongside
// Root in this importable package so both are reusable and testable apart from
// the entry point.
func Config() app.Config {
	return app.Config{
		Title:        "gophics · hn",
		Size:         geom.Size{W: 480, H: 720},
		Background:   Background(),
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
		// The API is built once and never varies by position in the tree, so
		// it is provided to the whole app here rather than nested around it.
		// A test swaps in a fake by overriding this field.
		Provide: []any{newLiveAPI()},
	}
}
