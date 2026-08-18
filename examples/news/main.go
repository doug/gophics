// Command news is a personal RSS reader: one queue of everything you follow,
// ranked by what you actually read, with full articles and pictures stored on
// the device so it works with no connection.
//
// This is the desktop and web entry point. The widget tree, Root and Config
// live in the importable examples/news/ui package, which the Android and iOS
// builds reach through examples/news/mobile.
//
//	go run ./examples/news
package main

import (
	"log"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/news/ui"
)

func main() {
	if err := app.Run(ui.Root(), ui.Config()); err != nil {
		log.Fatal(err)
	}
}
