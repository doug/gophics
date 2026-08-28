package ui

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Root is Tally's widget tree — the same one on the desktop, the web and a
// phone. It lives here rather than in main because gomobile cannot bind package
// main, and the mobile bind surface is generated from these two functions.
func Root() widget.Widget { return App{} }

// Config is the app's window, font and identity configuration.
//
// It lives beside Root for the same reason: the mobile side used to rebuild
// this by hand, which is how an app ends up with one font on the desktop and
// another on a phone. Size is the desktop window; the mobile host owns its own
// surface and ignores it.
//
// The mono family is registered because Tally's figures are tabular: money only
// lines up in columns if the digits are the same width.
func Config() app.Config {
	return app.Config{
		Title:      "Tally",
		AppID:      "com.gophics.tally",
		Size:       geom.Size{W: 1040, H: 680},
		Background: theme.Light().Bg,
		Font:       goregular.TTF,
		FontFamilies: map[string][]byte{
			theme.FontBold: gobold.TTF,
			"mono":         gomono.TTF,
		},
	}
}
