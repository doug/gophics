// Command hn is the desktop/web entry point for the HN app
// (examples/hn/ui); examples/hn/mobile is the Android bind surface.
package main

import (
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/examples/hn/ui"
	"github.com/doug/gossamer/geom"
)

func main() {
	err := app.Run(ui.Root(), app.Config{
		Title:        "gossamer · hn",
		Size:         geom.Size{W: 480, H: 720},
		Background:   ui.Background(),
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	})
	if err != nil {
		log.Fatal(err)
	}
}
