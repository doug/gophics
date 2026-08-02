package main

import (
	"image"
	"image/color"
	"math"
)

// The tileset is generated in Go at startup — no binary assets. Every tile is a
// 16×16 region of one shared atlas image, blitted with paint.DrawSprite; the
// shared atlas means one cached GPU texture for the whole map.

const tile = 16

// TileID indexes a tile in the atlas strip.
type TileID int

const (
	TFloor TileID = iota
	TWall
	TPlayer
	TGoblin
	TRat
	TPotion
	TGold
	TStairs
	TDoor
	TAmulet
	TGlow
	tileCount
)

// src is the atlas source rectangle for a tile.
func src(id TileID) image.Rectangle {
	x := int(id) * tile
	return image.Rect(x, 0, x+tile, tile)
}

var (
	cOutline = color.RGBA{18, 20, 28, 255}
	cFloor   = color.RGBA{44, 46, 58, 255}
	cFloor2  = color.RGBA{54, 57, 72, 255}
	cWall    = color.RGBA{92, 84, 78, 255}
	cWall2   = color.RGBA{68, 62, 58, 255}
	cPlayer  = color.RGBA{86, 196, 214, 255}
	cGoblin  = color.RGBA{86, 168, 78, 255}
	cRat     = color.RGBA{140, 130, 128, 255}
	cWhite   = color.RGBA{240, 244, 248, 255}
	cRed     = color.RGBA{206, 66, 68, 255}
	cGold    = color.RGBA{226, 186, 74, 255}
	cBrown   = color.RGBA{120, 84, 52, 255}
	cGlass   = color.RGBA{170, 210, 220, 255}
)

// buildAtlas draws every tile into one image and returns it.
func buildAtlas() *image.RGBA {
	a := image.NewRGBA(image.Rect(0, 0, int(tileCount)*tile, tile))

	// Floor: dark stone with a few lighter specks.
	fill(a, TFloor, cFloor)
	for _, p := range [][2]int{{3, 4}, {10, 3}, {6, 11}, {12, 9}, {2, 13}, {9, 8}} {
		px(a, TFloor, p[0], p[1], cFloor2)
	}

	// Wall: brick with mortar lines.
	fill(a, TWall, cWall)
	for y := 0; y < tile; y++ {
		for x := 0; x < tile; x++ {
			if y%5 == 4 || (x+((y/5)%2)*4)%8 == 7 {
				px(a, TWall, x, y, cWall2)
			}
		}
	}

	// Player: cyan adventurer blob with eyes.
	creature(a, TPlayer, cPlayer, cWhite, cOutline)
	// Goblin: green, angry (red pupils).
	creature(a, TGoblin, cGoblin, cRed, cOutline)
	// Rat: small gray, with a tail.
	disc(a, TRat, 8, 9, 4, cRat)
	outline(a, TRat, cOutline)
	px(a, TRat, 13, 11, cRat)
	px(a, TRat, 14, 12, cRat)
	px(a, TRat, 6, 8, cOutline)
	px(a, TRat, 10, 8, cOutline)

	// Potion: flask with red liquid.
	rect(a, TPotion, 7, 3, 8, 5, cGlass)
	disc(a, TPotion, 8, 10, 4, cGlass)
	disc(a, TPotion, 8, 11, 3, cRed)
	outline(a, TPotion, cOutline)

	// Gold: a small pile of coins.
	for _, p := range [][2]int{{6, 10}, {9, 10}, {7, 7}} {
		disc(a, TGold, p[0], p[1], 2, cGold)
	}

	// Stairs down: nested steps.
	for i := 0; i < 4; i++ {
		rect(a, TStairs, 3+i, 3+i*3, 12, 5+i*3, shade(cFloor2, 1-float64(i)*0.18))
	}

	// Door: brown with a knob.
	rect(a, TDoor, 3, 2, 12, 13, cBrown)
	outlineRect(a, TDoor, 3, 2, 12, 13, cOutline)
	px(a, TDoor, 10, 7, cGold)

	// Amulet: gold medallion with a red gem and a chain.
	px(a, TAmulet, 8, 3, cGold)
	px(a, TAmulet, 8, 4, cGold)
	disc(a, TAmulet, 8, 9, 4, cGold)
	disc(a, TAmulet, 8, 9, 2, cRed)
	outline(a, TAmulet, cOutline)

	// Glow: a soft radial warm halo (premultiplied alpha) for the torch aura.
	glowTile(a)

	return a
}

func glowTile(a *image.RGBA) {
	const cx, cy = 7.5, 7.5
	for y := 0; y < tile; y++ {
		for x := 0; x < tile; x++ {
			f := 1 - math.Hypot(float64(x)-cx, float64(y)-cy)/8
			if f < 0 {
				f = 0
			}
			f *= f
			ax, ay := at(TGlow, x, y)
			a.SetRGBA(ax, ay, color.RGBA{
				R: uint8(255 * f), G: uint8(238 * f), B: uint8(205 * f), A: uint8(255 * f)})
		}
	}
}

// creature draws a rounded body with two eyes — the shared entity shape.
func creature(a *image.RGBA, id TileID, body, eye, out color.RGBA) {
	disc(a, id, 8, 9, 5, body)
	outline(a, id, out)
	disc(a, id, 6, 8, 1, cWhite)
	disc(a, id, 10, 8, 1, cWhite)
	px(a, id, 6, 8, eye)
	px(a, id, 10, 8, eye)
}

// --- low-level tile painters (tile-local coordinates) ---

func at(id TileID, x, y int) (int, int) { return int(id)*tile + x, y }

func px(a *image.RGBA, id TileID, x, y int, c color.RGBA) {
	if x < 0 || y < 0 || x >= tile || y >= tile {
		return
	}
	ax, ay := at(id, x, y)
	a.SetRGBA(ax, ay, c)
}

func fill(a *image.RGBA, id TileID, c color.RGBA) {
	for y := 0; y < tile; y++ {
		for x := 0; x < tile; x++ {
			px(a, id, x, y, c)
		}
	}
}

func rect(a *image.RGBA, id TileID, x0, y0, x1, y1 int, c color.RGBA) {
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			px(a, id, x, y, c)
		}
	}
}

func disc(a *image.RGBA, id TileID, cx, cy, r int, c color.RGBA) {
	for y := -r; y <= r; y++ {
		for x := -r; x <= r; x++ {
			if x*x+y*y <= r*r {
				px(a, id, cx+x, cy+y, c)
			}
		}
	}
}

// outline darkens the transparent-adjacent border of already-drawn body pixels.
func outline(a *image.RGBA, id TileID, out color.RGBA) {
	for y := 0; y < tile; y++ {
		for x := 0; x < tile; x++ {
			ax, ay := at(id, x, y)
			if a.RGBAAt(ax, ay).A == 0 {
				continue
			}
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := x+d[0], y+d[1]
				if nx < 0 || ny < 0 || nx >= tile || ny >= tile {
					continue
				}
				bx, by := at(id, nx, ny)
				if a.RGBAAt(bx, by).A == 0 {
					a.SetRGBA(bx, by, out)
				}
			}
		}
	}
}

func outlineRect(a *image.RGBA, id TileID, x0, y0, x1, y1 int, c color.RGBA) {
	for x := x0; x <= x1; x++ {
		px(a, id, x, y0, c)
		px(a, id, x, y1, c)
	}
	for y := y0; y <= y1; y++ {
		px(a, id, x0, y, c)
		px(a, id, x1, y, c)
	}
}

func shade(c color.RGBA, f float64) color.RGBA {
	cl := func(v uint8) uint8 {
		n := float64(v) * f
		if n > 255 {
			n = 255
		}
		return uint8(n)
	}
	return color.RGBA{cl(c.R), cl(c.G), cl(c.B), c.A}
}
