package main

import (
	"fmt"
	"os"
	"testing"
)

// A roguelike is only as good as its difficulty curve, and a curve is not
// something you can eyeball from one screenshot. TestBalance plays the game
// many times with a crude but honest bot and reports where runs end.
//
// It asserts only the things that would make the game plainly broken — every
// run dying on level one, or nobody ever dying — because tuning is a judgement
// call and a test that pins exact rates would fail on every future tweak. Run
// with BALANCE_REPORT=1 to see the distribution.

// bot plays one game and returns the depth reached, whether it won, and turns.
func bot(seed int64, maxTurns int) (depth int, won, dead bool, turns int) {
	g := newGame(seed)
	for turns = 0; turns < maxTurns && !g.dead && !g.won; turns++ {
		p := g.player

		// Drink when badly hurt and holding something — the decision the
		// player faces, made by the simplest sensible rule.
		if g.potions > 0 && p.HP*3 <= p.MaxHP {
			g.Quaff()
			continue
		}
		// Fight anything adjacent.
		if m := adjacentMonster(g); m != nil {
			g.Move(sign(m.X-p.X), sign(m.Y-p.Y))
			continue
		}
		// Otherwise walk toward the nearest interesting thing: loot while any
		// is reachable, then the stairs down.
		step, ok := botStep(g)
		if !ok {
			break // nothing reachable — the level is exhausted
		}
		g.Move(step[0], step[1])
	}
	return g.depth, g.won, g.dead, turns
}

// botStep breadth-firsts from the player to the nearest goal and returns the
// first move along that path. A straight-line walker gets wedged on the first
// wall corner and reports the dungeon as impassable, which says more about the
// bot than the game.
func botStep(g *Game) ([2]int, bool) {
	goal := map[[2]int]bool{}
	for _, it := range g.items {
		goal[[2]int{it.X, it.Y}] = true
	}
	if len(goal) == 0 {
		for y := 0; y < g.d.H; y++ {
			for x := 0; x < g.d.W; x++ {
				if g.d.at(x, y) == CellStairs {
					goal[[2]int{x, y}] = true
				}
			}
		}
	}
	if len(goal) == 0 {
		return [2]int{}, false
	}

	start := [2]int{g.player.X, g.player.Y}
	prev := map[[2]int][2]int{start: start}
	queue := [][2]int{start}
	var found ([2]int)
	ok := false
	for len(queue) > 0 && !ok {
		cur := queue[0]
		queue = queue[1:]
		if goal[cur] {
			found, ok = cur, true
			break
		}
		for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			n := [2]int{cur[0] + d[0], cur[1] + d[1]}
			if _, seen := prev[n]; seen || !g.d.walkable(n[0], n[1]) {
				continue
			}
			prev[n] = cur
			queue = append(queue, n)
		}
	}
	if !ok {
		return [2]int{}, false
	}
	// Walk the chain back to the cell adjacent to the player.
	cur := found
	for prev[cur] != start {
		cur = prev[cur]
	}
	return [2]int{cur[0] - start[0], cur[1] - start[1]}, true
}

func adjacentMonster(g *Game) *Entity {
	for _, m := range g.monsters {
		if m.Alive && abs(m.X-g.player.X) <= 1 && abs(m.Y-g.player.Y) <= 1 {
			return m
		}
	}
	return nil
}

// botTarget picks a destination: the first item it can see, else the stairs.
func botTarget(g *Game) (int, int, bool) {
	for _, it := range g.items {
		return it.X, it.Y, true
	}
	for y := 0; y < g.d.H; y++ {
		for x := 0; x < g.d.W; x++ {
			if g.d.at(x, y) == CellStairs {
				return x, y, true
			}
		}
	}
	return 0, 0, false
}

func TestBalance(t *testing.T) {
	const runs = 200
	var wins, deaths, stalls, totalTurns int
	depths := map[int]int{}
	for i := 0; i < runs; i++ {
		d, won, dead, turns := bot(int64(i)+1, 4000)
		depths[d]++
		totalTurns += turns
		switch {
		case won:
			wins++
		case dead:
			deaths++
		default:
			stalls++
		}
	}

	if os.Getenv("BALANCE_REPORT") != "" {
		fmt.Printf("\n%d runs — %d won, %d died, %d stalled, %d turns avg\n",
			runs, wins, deaths, stalls, totalTurns/runs)
		for d := 1; d <= maxDepth; d++ {
			fmt.Printf("  ended on depth %d: %3d  %s\n", d, depths[d], bar(depths[d], runs))
		}
	}

	// A game where the bot never dies has no teeth; one where it never gets
	// anywhere is a wall. Both are broken in a way worth failing over.
	if deaths == 0 {
		t.Error("no run ever ended in death — the dungeon poses no threat")
	}
	if depths[1] > runs*3/4 {
		t.Errorf("%d of %d runs ended on depth 1 — the first level is a wall", depths[1], runs)
	}
}

func bar(n, of int) string {
	w := n * 40 / max(1, of)
	s := ""
	for i := 0; i < w; i++ {
		s += "█"
	}
	return s
}
