// Command match3 is a match-3 puzzle on gossamer: swap adjacent gems to line up
// three or more; matches clear, gems fall in to fill the gaps, and fresh gems
// drop from the top — chaining into cascades. It's the animation driver
// example: every swap, clear, fall, and cascade runs through anim.Controller
// and the paint path, all procedural (no image assets). Swipe or tap a gem then
// an adjacent one to swap.
package main

import (
	"log"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/sound"
	"github.com/doug/gossamer/sound/device"
)

func main() {
	// Audio is best-effort: if the device won't open, the game runs silent.
	mixer := sound.NewMixer()
	if closer, err := device.Open(mixer); err != nil {
		log.Printf("audio disabled: %v", err)
	} else {
		defer closer.Close()
	}

	err := app.Run(Match3{Seed: time.Now().UnixNano(), Sound: mixer}, app.Config{
		Title:        "Match 3",
		Size:         geom.Size{W: 480, H: 760},
		Background:   colBG,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	})
	if err != nil {
		log.Fatal(err)
	}
}
