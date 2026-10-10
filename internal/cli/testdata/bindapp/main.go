// Command bindapp is a scratch app for the bind-package generator's tests.
//
// It lives under testdata on purpose. Generating a bind package writes Go into
// the app's own build/ directory, and the test deletes it again afterwards —
// which, done inside examples/, drops a package into the middle of the module
// for as long as the test runs, and leaves one behind when the run is
// interrupted. The go tool skips testdata when it expands ./..., so nothing
// here is ever enumerated by a sweep, while `go build <import path>` still
// compiles it, which is all the test needs.
package main

import (
	"log"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/internal/cli/testdata/bindapp/ui"
)

func main() {
	if err := app.Run(ui.Root(), ui.Config()); err != nil {
		log.Fatal(err)
	}
}
