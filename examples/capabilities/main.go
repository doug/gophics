// Command capabilities is a live inspector for the platform-capability layer
// (shell/*.go via ctx.<Cap>()): every capability the running platform exposes,
// shown working. A capability the platform cannot provide renders greyed-out
// "unsupported", so the same screen honestly reports what each target actually
// has.
//
//	gophics run                     # desktop
//	gophics run -p web              # browser
//	gophics run -p android          # a phone or tablet
package main

import (
	"log"

	"github.com/doug/gophics/app"

	"github.com/doug/gophics/examples/capabilities/ui"
)

func main() {
	if err := app.Run(ui.Root(), ui.Config()); err != nil {
		log.Fatal(err)
	}
}
