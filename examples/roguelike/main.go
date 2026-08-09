// Command roguelike is a tile-based dungeon crawler on gophics, and the driver
// example for paint.DrawSprite: the whole map, monsters, and items are blitted
// from one procedurally-generated atlas texture (no binary assets). Turn-based,
// with a minimal d20 combat core. Arrow keys or tap to move; bump to attack;
// reach the stairs to descend.
//
//	go run ./examples/roguelike
package main

import (
	"log"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/sound/device"
)

func main() {
	// Audio is best-effort: if the device won't open, the game runs silent.
	mixer := sound.NewMixer()
	if closer, err := device.Open(mixer); err != nil {
		log.Printf("audio disabled: %v", err)
	} else {
		defer closer.Close()
	}

	err := app.Run(Roguelike{Seed: time.Now().UnixNano(), Sound: mixer}, app.Config{
		Title:        "Roguelike",
		Size:         geom.Size{W: 900, H: 680},
		Background:   colBG,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	})
	if err != nil {
		log.Fatal(err)
	}
}
