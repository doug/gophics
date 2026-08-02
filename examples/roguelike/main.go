// Command roguelike is a tile-based dungeon crawler on gossamer, and the driver
// example for paint.DrawSprite: the whole map, monsters, and items are blitted
// from one procedurally-generated atlas texture (no binary assets). Turn-based,
// with a minimal d20 combat core. Arrow keys or tap to move; bump to attack;
// reach the stairs to descend.
package main

import (
	"log"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
)

func main() {
	err := app.Run(Roguelike{Seed: time.Now().UnixNano()}, app.Config{
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
