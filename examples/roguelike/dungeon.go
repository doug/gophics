package main

import (
	"math/rand"
	"slices"
)

// Cell is a map tile's terrain.
type Cell uint8

const (
	CellWall Cell = iota
	CellFloor
	CellDoor
	CellStairs
)

// Room is an axis-aligned rectangle of floor.
type Room struct{ X, Y, W, H int }

func (r Room) center() (int, int) { return r.X + r.W/2, r.Y + r.H/2 }
func (r Room) overlaps(o Room) bool {
	return r.X <= o.X+o.W && r.X+r.W >= o.X && r.Y <= o.Y+o.H && r.Y+r.H >= o.Y
}

// Dungeon is a grid of cells with the rooms that were carved.
type Dungeon struct {
	W, H  int
	cells []Cell
	rooms []Room
}

func (d *Dungeon) at(x, y int) Cell {
	if x < 0 || y < 0 || x >= d.W || y >= d.H {
		return CellWall
	}
	return d.cells[y*d.W+x]
}

func (d *Dungeon) set(x, y int, c Cell) {
	if x >= 0 && y >= 0 && x < d.W && y < d.H {
		d.cells[y*d.W+x] = c
	}
}

// walkable reports whether an entity can stand on (x, y).
func (d *Dungeon) walkable(x, y int) bool {
	c := d.at(x, y)
	return c == CellFloor || c == CellDoor || c == CellStairs
}

// opaque reports whether (x, y) blocks line of sight.
func (d *Dungeon) opaque(x, y int) bool { return d.at(x, y) == CellWall }

// genDungeon carves up to maxRooms non-overlapping rooms and connects each to
// the previous with an L-shaped corridor. The last room gets the stairs down.
func genDungeon(w, h, maxRooms int, rng *rand.Rand) *Dungeon {
	d := &Dungeon{W: w, H: h, cells: make([]Cell, w*h)} // all walls
	for range maxRooms {
		rw, rh := 4+rng.Intn(7), 3+rng.Intn(5)
		rx, ry := 1+rng.Intn(w-rw-2), 1+rng.Intn(h-rh-2)
		room := Room{rx, ry, rw, rh}
		clash := slices.ContainsFunc(d.rooms, room.overlaps)
		if clash {
			continue
		}
		for y := ry; y < ry+rh; y++ {
			for x := rx; x < rx+rw; x++ {
				d.set(x, y, CellFloor)
			}
		}
		if len(d.rooms) > 0 {
			px, py := d.rooms[len(d.rooms)-1].center()
			cx, cy := room.center()
			d.carveCorridor(px, py, cx, cy, rng)
		}
		d.rooms = append(d.rooms, room)
	}
	if n := len(d.rooms); n > 0 {
		sx, sy := d.rooms[n-1].center()
		d.set(sx, sy, CellStairs)
	}
	return d
}

func (d *Dungeon) carveCorridor(x0, y0, x1, y1 int, rng *rand.Rand) {
	if rng.Intn(2) == 0 {
		d.hLine(x0, x1, y0)
		d.vLine(y0, y1, x1)
	} else {
		d.vLine(y0, y1, x0)
		d.hLine(x0, x1, y1)
	}
}

func (d *Dungeon) hLine(x0, x1, y int) {
	if x0 > x1 {
		x0, x1 = x1, x0
	}
	for x := x0; x <= x1; x++ {
		if d.at(x, y) == CellWall {
			d.set(x, y, CellFloor)
		}
	}
}

func (d *Dungeon) vLine(y0, y1, x int) {
	if y0 > y1 {
		y0, y1 = y1, y0
	}
	for y := y0; y <= y1; y++ {
		if d.at(x, y) == CellWall {
			d.set(x, y, CellFloor)
		}
	}
}
