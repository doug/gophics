package main

import (
	"fmt"
	"math/rand"
	"strings"
)

// Entity is the player or a monster.
type Entity struct {
	X, Y            int
	Tile            TileID
	Name            string
	HP, MaxHP       int
	Atk, AC, Damage int // d20 attack bonus, armor class, damage die
	Alive           bool
	FlipX           bool
}

// Item is a pickup lying on the floor.
type Item struct {
	X, Y   int
	Tile   TileID
	Gold   int  // >0 → gold; else a potion
	Amulet bool // the win goal
}

// maxDepth is where the Amulet of Yendor waits.
const maxDepth = 5

// SoundID names a sound effect; the widget maps these to samples. The engine
// only emits ids, staying decoupled from the audio package (and silent in tests).
type SoundID int

const (
	SndHit SoundID = iota
	SndCoin
	SndPotion
	SndDescend
	SndDie
	SndWin
)

// Game is the full, rendering-free game state.
type Game struct {
	d        *Dungeon
	player   *Entity
	monsters []*Entity
	items    []*Item
	seen     []bool // ever revealed (fog memory)
	visible  []bool // in the current field of view
	rng      *rand.Rand
	log      []string
	depth    int
	gold     int
	dead     bool
	won      bool
	sfx      func(id SoundID, pan float64) // optional; set by the widget
}

func (g *Game) play(id SoundID, pan float64) {
	if g.sfx != nil {
		g.sfx(id, pan)
	}
}

// panAt maps a world x to a stereo pan (-1..1) relative to the player.
func (g *Game) panAt(x int) float64 {
	p := float64(x-g.player.X) / fovRadius
	if p < -1 {
		p = -1
	} else if p > 1 {
		p = 1
	}
	return p
}

const fovRadius = 7

// newGame builds level 1 with a seeded RNG.
func newGame(seed int64) *Game {
	g := &Game{rng: rand.New(rand.NewSource(seed)), depth: 1}
	g.build()
	return g
}

// build lays out the current depth: dungeon, player, monsters, items, FOV.
func (g *Game) build() {
	d := genDungeon(48, 32, 14, g.rng)
	g.d = d
	g.seen = make([]bool, d.W*d.H)
	g.visible = make([]bool, d.W*d.H)
	g.monsters = nil
	g.items = nil

	px, py := d.rooms[0].center()
	if g.player == nil {
		g.player = &Entity{Tile: TPlayer, Name: "you", HP: 20, MaxHP: 20, Atk: 4, AC: 13, Damage: 6, Alive: true}
	}
	g.player.X, g.player.Y = px, py

	// Populate the other rooms with monsters and loot, scaling with depth.
	for _, r := range d.rooms[1:] {
		switch g.rng.Intn(3) {
		case 0:
			mx, my := r.center()
			g.monsters = append(g.monsters, &Entity{X: mx, Y: my, Tile: TGoblin,
				Name: "goblin", HP: 7 + g.depth, MaxHP: 7 + g.depth, Atk: 3, AC: 12, Damage: 5, Alive: true})
		case 1:
			mx, my := r.center()
			g.monsters = append(g.monsters, &Entity{X: mx, Y: my, Tile: TRat,
				Name: "rat", HP: 3, MaxHP: 3, Atk: 2, AC: 10, Damage: 3, Alive: true})
		}
		if g.rng.Intn(2) == 0 {
			ix, iy := r.X+1+g.rng.Intn(r.W-1), r.Y+1+g.rng.Intn(r.H-1)
			if g.rng.Intn(2) == 0 {
				g.items = append(g.items, &Item{X: ix, Y: iy, Tile: TGold, Gold: 3 + g.rng.Intn(12)})
			} else {
				g.items = append(g.items, &Item{X: ix, Y: iy, Tile: TPotion})
			}
		}
	}
	// The Amulet of Yendor waits on the deepest level (no stairs down there).
	if g.depth >= maxDepth {
		sx, sy := d.rooms[len(d.rooms)-1].center()
		d.set(sx, sy, CellFloor)
		g.items = append(g.items, &Item{X: sx, Y: sy, Tile: TAmulet, Amulet: true})
	}

	g.computeFOV()
	if g.depth >= maxDepth {
		g.logf("Level %d — the Amulet of Yendor is near!", g.depth)
	} else {
		g.logf("You enter dungeon level %d.", g.depth)
	}
}

func (g *Game) logf(format string, a ...any) {
	g.log = append(g.log, fmt.Sprintf(format, a...))
	if len(g.log) > 6 {
		g.log = g.log[len(g.log)-6:]
	}
}

// monsterAt returns the living monster on (x, y), if any.
func (g *Game) monsterAt(x, y int) *Entity {
	for _, m := range g.monsters {
		if m.Alive && m.X == x && m.Y == y {
			return m
		}
	}
	return nil
}

// Move attempts to move the player by (dx, dy): attack a monster there, step
// onto walkable floor (picking up items, descending stairs), else do nothing.
// A successful action passes the turn to the monsters.
func (g *Game) Move(dx, dy int) {
	if g.dead || g.won || (dx == 0 && dy == 0) {
		return
	}
	if dx < 0 {
		g.player.FlipX = true
	} else if dx > 0 {
		g.player.FlipX = false
	}
	nx, ny := g.player.X+dx, g.player.Y+dy
	if m := g.monsterAt(nx, ny); m != nil {
		g.attack(g.player, m)
		g.monstersAct()
		g.computeFOV()
		return
	}
	if !g.d.walkable(nx, ny) {
		return
	}
	g.player.X, g.player.Y = nx, ny
	g.pickup()
	if g.d.at(nx, ny) == CellStairs {
		g.descend()
		return
	}
	g.monstersAct()
	g.computeFOV()
}

func (g *Game) pickup() {
	kept := g.items[:0]
	for _, it := range g.items {
		if it.X == g.player.X && it.Y == g.player.Y {
			if it.Amulet {
				g.won = true
				g.play(SndWin, 0)
				g.logf("You claim the Amulet of Yendor — you win!")
				continue
			}
			if it.Gold > 0 {
				g.gold += it.Gold
				g.play(SndCoin, 0)
				g.logf("You pick up %d gold.", it.Gold)
			} else {
				heal := 8
				g.player.HP = min(g.player.MaxHP, g.player.HP+heal)
				g.play(SndPotion, 0)
				g.logf("You quaff a potion (+%d HP).", heal)
			}
			continue
		}
		kept = append(kept, it)
	}
	g.items = kept
}

func (g *Game) descend() {
	g.play(SndDescend, 0)
	g.depth++
	g.gold += 5
	g.build()
}

// attack resolves one d20 attack: d20 + Atk vs AC, then Damage die on a hit.
func (g *Game) attack(a, b *Entity) {
	roll := g.rng.Intn(20) + 1
	// Pan toward the non-player combatant.
	loc := b
	if b == g.player {
		loc = a
	}
	pan := g.panAt(loc.X)
	if roll+a.Atk >= b.AC {
		dmg := g.rng.Intn(b.hitDie(a)) + 1
		b.HP -= dmg
		g.play(SndHit, pan)
		g.logf("%s hit %s for %d.", cap1(a.Name), b.Name, dmg)
		if b.HP <= 0 {
			b.Alive = false
			g.logf("%s dies.", cap1(b.Name))
			if b == g.player {
				g.dead = true
				g.play(SndDie, 0)
				g.logf("You have died on level %d.", g.depth)
			}
		}
	} else {
		g.logf("%s missed %s.", cap1(a.Name), b.Name)
	}
}

func (e *Entity) hitDie(a *Entity) int {
	if a.Damage > 0 {
		return a.Damage
	}
	return 4
}

// monstersAct runs each living monster: attack if adjacent to the player, else
// step toward the player when it can see them.
func (g *Game) monstersAct() {
	for _, m := range g.monsters {
		if !m.Alive || g.dead {
			continue
		}
		dx, dy := g.player.X-m.X, g.player.Y-m.Y
		if abs(dx) <= 1 && abs(dy) <= 1 {
			g.attack(m, g.player)
			continue
		}
		if !g.visibleAt(m.X, m.Y) {
			continue // only chase when in the lit area
		}
		sx, sy := sign(dx), sign(dy)
		if g.step(m, sx, sy) || g.step(m, sx, 0) || g.step(m, 0, sy) {
			m.FlipX = sx < 0
		}
	}
}

func (g *Game) step(m *Entity, dx, dy int) bool {
	if dx == 0 && dy == 0 {
		return false
	}
	nx, ny := m.X+dx, m.Y+dy
	if !g.d.walkable(nx, ny) || g.monsterAt(nx, ny) != nil || (nx == g.player.X && ny == g.player.Y) {
		return false
	}
	m.X, m.Y = nx, ny
	return true
}

func (g *Game) visibleAt(x, y int) bool {
	if x < 0 || y < 0 || x >= g.d.W || y >= g.d.H {
		return false
	}
	return g.visible[y*g.d.W+x]
}

func (g *Game) seenAt(x, y int) bool {
	if x < 0 || y < 0 || x >= g.d.W || y >= g.d.H {
		return false
	}
	return g.seen[y*g.d.W+x]
}

// computeFOV recomputes visibility: every cell within fovRadius with an
// unobstructed line from the player is visible (and remembered as seen).
func (g *Game) computeFOV() {
	for i := range g.visible {
		g.visible[i] = false
	}
	px, py := g.player.X, g.player.Y
	for y := py - fovRadius; y <= py+fovRadius; y++ {
		for x := px - fovRadius; x <= px+fovRadius; x++ {
			if x < 0 || y < 0 || x >= g.d.W || y >= g.d.H {
				continue
			}
			if (x-px)*(x-px)+(y-py)*(y-py) > fovRadius*fovRadius {
				continue
			}
			if g.los(px, py, x, y) {
				g.visible[y*g.d.W+x] = true
				g.seen[y*g.d.W+x] = true
			}
		}
	}
}

// los is a Bresenham line-of-sight test: true when no wall lies strictly between
// (x0,y0) and (x1,y1).
func (g *Game) los(x0, y0, x1, y1 int) bool {
	dx, dy := abs(x1-x0), abs(y1-y0)
	sx, sy := sign(x1-x0), sign(y1-y0)
	err := dx - dy
	x, y := x0, y0
	for {
		if x == x1 && y == y1 {
			return true
		}
		if !(x == x0 && y == y0) && g.d.opaque(x, y) {
			return false
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x += sx
		}
		if e2 < dx {
			err += dx
			y += sy
		}
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func sign(v int) int {
	switch {
	case v > 0:
		return 1
	case v < 0:
		return -1
	}
	return 0
}

func cap1(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
