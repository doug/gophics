// Command hn is the desktop/web entry point for the HN app
// (examples/hn/ui); examples/hn/mobile is the Android bind surface.
//
// Root and Config are exported so `gossamer dev --hot` can build this package
// as a plugin and hot-reload the widget tree.
package main

import (
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/examples/hn/ui"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/widget"
)

// Root returns the app's root widget.
func Root() widget.Widget { return ui.Root() }

// Config returns the app's window/runtime configuration.
func Config() app.Config {
	return app.Config{
		Title:        "gossamer · hn",
		Size:         geom.Size{W: 480, H: 720},
		Background:   ui.Background(),
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}
}

func main() {
	if err := app.Run(Root(), Config()); err != nil {
		log.Fatal(err)
	}
}
