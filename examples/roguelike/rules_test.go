package main

import (
	"testing"

	"github.com/doug/gophics/shell"
)

// freeNeighbour finds a walkable cell next to (x, y) with nothing standing on
// it, diagonals included when diag is set.
func freeNeighbour(g *Game, x, y int, diag bool) (int, int, bool) {
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			if dx == 0 && dy == 0 || (!diag && dx != 0 && dy != 0) {
				continue
			}
			nx, ny := x+dx, y+dy
			if g.d.walkable(nx, ny) && g.monsterAt(nx, ny) == nil && !(nx == g.player.X && ny == g.player.Y) {
				return nx, ny, true
			}
		}
	}
	return 0, 0, false
}

// TestWinningStepIsNotFatal: claiming the amulet ends the run, and the monsters
// do not get a swing at the player on the way out. Move used to end the turn
// after the pickup, and draw checks dead before won — so a brute standing next
// to the amulet could turn the win into "You died".
func TestWinningStepIsNotFatal(t *testing.T) {
	g := newGame(1)
	ax, ay, ok := freeNeighbour(g, g.player.X, g.player.Y, false)
	if !ok {
		t.Fatal("nowhere next to the player to put the amulet")
	}
	mx, my, ok := freeNeighbour(g, ax, ay, true)
	if !ok {
		t.Fatal("nowhere next to the amulet to put a monster")
	}
	g.items = append(g.items, &Item{X: ax, Y: ay, Tile: TAmulet, Amulet: true})
	g.monsters = append(g.monsters, &Entity{X: mx, Y: my, Tile: TGoblin, Name: "brute", Alive: true,
		HP: 50, MaxHP: 50, Atk: 100, AC: 10, Damage: 6, Speed: 3})
	g.player.HP = 1 // any hit that lands is fatal, and with Atk 100 every hit lands

	g.Move(ax-g.player.X, ay-g.player.Y)
	if !g.won {
		t.Fatal("stepping onto the amulet did not win")
	}
	if g.dead {
		t.Fatal("the player died on the winning step")
	}
}

// TestDiagonalKeys: monsters step diagonally and a tap can, so the keyboard
// must be able to as well — the vi keys y/u/b/n.
func TestDiagonalKeys(t *testing.T) {
	_, st := mount(t, 7)
	keys := map[shell.KeyCode][2]int{
		shell.KeyY: {-1, -1}, shell.KeyU: {1, -1}, shell.KeyB: {-1, 1}, shell.KeyN: {1, 1},
	}
	moved := 0
	for code, d := range keys {
		g := st.g
		x, y := g.player.X, g.player.Y
		if !g.d.walkable(x+d[0], y+d[1]) || g.monsterAt(x+d[0], y+d[1]) != nil {
			continue
		}
		st.key(code)
		if g.player.X != x+d[0] || g.player.Y != y+d[1] {
			t.Errorf("key %v moved the player from (%d,%d) to (%d,%d), want (%d,%d)",
				code, x, y, g.player.X, g.player.Y, x+d[0], y+d[1])
		}
		moved++
		if st.g.dead || st.g.won {
			break
		}
	}
	if moved == 0 {
		t.Fatal("no diagonal was free to test; the player should start inside a room")
	}
}

// TestCorridorsEnterRoomsThroughDoors: doors are placed where a corridor meets
// a room — and only there. Each door is a passage (walkable either side along
// one axis, wall along the other) and no room edge is lined with them.
func TestCorridorsEnterRoomsThroughDoors(t *testing.T) {
	doors := 0
	for seed := int64(1); seed <= 12; seed++ {
		g := newGame(seed)
		d := g.d
		for y := range d.H {
			for x := range d.W {
				if d.at(x, y) != CellDoor {
					continue
				}
				doors++
				if d.inRoom(x, y) {
					t.Errorf("seed %d: door at (%d,%d) is inside a room", seed, x, y)
				}
				ns := d.walkable(x, y-1) && d.walkable(x, y+1) && !d.walkable(x-1, y) && !d.walkable(x+1, y)
				ew := d.walkable(x-1, y) && d.walkable(x+1, y) && !d.walkable(x, y-1) && !d.walkable(x, y+1)
				if !ns && !ew {
					t.Errorf("seed %d: door at (%d,%d) is not a passage", seed, x, y)
				}
				if d.at(x-1, y) == CellDoor || d.at(x+1, y) == CellDoor || d.at(x, y-1) == CellDoor || d.at(x, y+1) == CellDoor {
					t.Errorf("seed %d: doors at (%d,%d) are lined up", seed, x, y)
				}
			}
		}
	}
	if doors == 0 {
		t.Fatal("no dungeon over twelve seeds had a door; corridors always meet rooms somewhere")
	}
}
