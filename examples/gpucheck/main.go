// Command gpucheck runs the mobile-GPU verification scene on desktop (also the
// desktop-GPU reference for the on-device comparison). On mobile it's driven by
// examples/hn/mobile's StartVerify via the same host.
package main

import (
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	gpucheck "github.com/doug/gophics/examples/gpucheck/ui"
	"github.com/doug/gophics/geom"
)

func main() {
	err := app.Run(gpucheck.Root(), app.Config{
		Title:        "GPU Check",
		Size:         geom.Size{W: 440, H: 660},
		Background:   gpucheck.Background(),
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	})
	if err != nil {
		log.Fatal(err)
	}
}
