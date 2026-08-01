package ui

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
)

// Config returns the app's window/runtime configuration, including the named
// font families the markdown renderer uses (bold/italic/mono).
func Config() app.Config {
	return app.Config{
		Title:      "gossamer · notes",
		Size:       geom.Size{W: 900, H: 640},
		Background: colBg,
		Font:       goregular.TTF,
		FontFamilies: map[string][]byte{
			"bold":   gobold.TTF,
			"italic": goitalic.TTF,
			"mono":   gomono.TTF,
		},
	}
}
