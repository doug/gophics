// Command mirror is a voice-reactive mirror: your camera, warped every frame by
// what the microphone hears.
//
// The app itself lives in ./ui so the same tree can be built as a desktop
// command and bound into a mobile host — gomobile cannot bind package main, so
// the CLI generates a bind package from ui.Root and ui.Config.
package main

import (
	"log"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/mirror/ui"
)

func main() {
	if err := app.Run(ui.Root(), ui.Config()); err != nil {
		log.Fatal(err)
	}
}
