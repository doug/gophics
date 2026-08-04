// Command hn is the desktop/web entry point for the HN app. The widget tree,
// Root, and Config live in the importable examples/hn/ui package, keeping the
// entry point trivial and the UI easy to test and reuse.
package main

import (
	"log"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/hn/ui"
)

func main() {
	if err := app.Run(ui.Root(), ui.Config()); err != nil {
		log.Fatal(err)
	}
}
