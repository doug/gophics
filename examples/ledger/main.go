// Command ledger is a local-first personal-finance dashboard built on gossamer,
// and the driver app for the built-in chart package: it renders spending,
// balance, and weekly-activity charts from a sample dataset (a real Ledger would
// import a local CSV/OFX file). One codebase runs on desktop, web, and mobile.
package main

import (
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
)

func main() {
	err := app.Run(Ledger{}, app.Config{
		Title:        "Ledger",
		Size:         geom.Size{W: 900, H: 820},
		Background:   colBG,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	})
	if err != nil {
		log.Fatal(err)
	}
}
