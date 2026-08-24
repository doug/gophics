// Command gallery is the gophics widget catalog: a sectioned, interactive
// showcase of the framework's higher-level components.
//
// The catalog itself is package ui, so the same tree runs from this desktop
// binary, from a web build, and from the mobile bind package next door —
// gomobile cannot bind package main, which is the whole reason for the split.
package main

import (
	"log"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/gallery/ui"
)

func main() {
	if err := app.Run(ui.Gallery{}, ui.Config()); err != nil {
		log.Fatal(err)
	}
}
