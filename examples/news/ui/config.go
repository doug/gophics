package ui

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/goitalic"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
)

// Fonts is the family set the reader needs. A news reader is mostly prose, and
// prose needs more than one weight: an article's emphasis, its headings, its
// quoted code and its captions all carry meaning that is lost when everything
// renders in the same face.
//
// It lives here rather than inside Config so the mobile bind surface, which
// builds its own app.Config, uses exactly the same set.
func Fonts() map[string][]byte {
	return map[string][]byte{
		"bold":       gobold.TTF,
		"italic":     goitalic.TTF,
		"bolditalic": gobolditalic.TTF,
		"mono":       gomono.TTF,
	}
}

// Config is the desktop and web window configuration.
func Config() app.Config {
	return app.Config{
		Title:        "gophics · news",
		Size:         geom.Size{W: 420, H: 820}, // a phone-shaped window, since that is the target
		Background:   Background(),
		Font:         goregular.TTF,
		FontFamilies: Fonts(),
	}
}
