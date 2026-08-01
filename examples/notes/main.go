// Command notes is a local-first Markdown notes app built on gossamer — one
// codebase for desktop, web, and terminal. The widget tree, Root, and Config
// live in the importable examples/notes/ui package.
package main

import (
	"log"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/examples/notes/ui"
)

func main() {
	if err := app.Run(ui.Root(), ui.Config()); err != nil {
		log.Fatal(err)
	}
}
