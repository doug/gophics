// Command solitaire is a Klondike solitaire built on gophics — one codebase
// for desktop, web, and (via shell/mobile) iOS/Android. The rules engine is the
// pure, exhaustively-tested examples/solitaire/klondike package; this command is
// the board: a single widget.Canvas that draws the cards (no image assets) and
// does its own drag/drop and hit-testing.
package main

import (
	"log"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
)

func main() {
	err := app.Run(Solitaire{Seed: time.Now().UnixNano()}, app.Config{
		Title:        "Solitaire",
		Size:         geom.Size{W: 920, H: 720},
		Background:   colFelt,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	})
	if err != nil {
		log.Fatal(err)
	}
}
