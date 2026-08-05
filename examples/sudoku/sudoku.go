package main

import "math/rand"

// Grid is a 9x9 Sudoku grid in row-major order; 0 is an empty cell.
type Grid [81]int

func idx(r, c int) int { return r*9 + c }

// canPlace reports whether v (1..9) can go at (r,c) without repeating in the
// row, column, or 3x3 box.
func canPlace(g *Grid, r, c, v int) bool {
	for i := 0; i < 9; i++ {
		if g[idx(r, i)] == v || g[idx(i, c)] == v {
			return false
		}
	}
	br, bc := r/3*3, c/3*3
	for dr := 0; dr < 3; dr++ {
		for dc := 0; dc < 3; dc++ {
			if g[idx(br+dr, bc+dc)] == v {
				return false
			}
		}
	}
	return true
}

func (g *Grid) firstEmpty() int {
	for i := 0; i < 81; i++ {
		if g[i] == 0 {
			return i
		}
	}
	return -1
}

// fill solves g in place by backtracking, trying digits in the order candidates
// returns (randomized during generation). Reports whether it fully solved.
func fill(g *Grid, candidates func() []int) bool {
	i := g.firstEmpty()
	if i < 0 {
		return true
	}
	r, c := i/9, i%9
	for _, v := range candidates() {
		if canPlace(g, r, c, v) {
			g[i] = v
			if fill(g, candidates) {
				return true
			}
			g[i] = 0
		}
	}
	return false
}

// countSolutions counts solutions of g, stopping once it reaches limit — enough
// to tell "unique" (==1) from "ambiguous" (>1).
func countSolutions(g Grid, limit int) int {
	n := 0
	var rec func(*Grid)
	rec = func(gg *Grid) {
		if n >= limit {
			return
		}
		i := gg.firstEmpty()
		if i < 0 {
			n++
			return
		}
		r, c := i/9, i%9
		for v := 1; v <= 9; v++ {
			if canPlace(gg, r, c, v) {
				gg[i] = v
				rec(gg)
				gg[i] = 0
				if n >= limit {
					return
				}
			}
		}
	}
	rec(&g)
	return n
}

// generate builds a random puzzle with a unique solution, removing clues toward
// targetClues while uniqueness holds. Returns the puzzle and its solution.
func generate(rng *rand.Rand, targetClues int) (puzzle, solution Grid) {
	shuffled := func() []int {
		d := []int{1, 2, 3, 4, 5, 6, 7, 8, 9}
		rng.Shuffle(len(d), func(i, j int) { d[i], d[j] = d[j], d[i] })
		return d
	}
	var full Grid
	fill(&full, shuffled)
	solution = full

	puzzle = full
	clues := 81
	for _, i := range rng.Perm(81) {
		if clues <= targetClues {
			break
		}
		saved := puzzle[i]
		puzzle[i] = 0
		if countSolutions(puzzle, 2) == 1 {
			clues--
		} else {
			puzzle[i] = saved // removing this clue made the puzzle ambiguous — keep it
		}
	}
	return puzzle, solution
}

// conflicts marks each filled cell that repeats its value within its row,
// column, or box.
func (g *Grid) conflicts() [81]bool {
	var bad [81]bool
	for i := 0; i < 81; i++ {
		v := g[i]
		if v == 0 {
			continue
		}
		r, c := i/9, i%9
		for j := 0; j < 9; j++ {
			if k := idx(r, j); k != i && g[k] == v {
				bad[i] = true
			}
			if k := idx(j, c); k != i && g[k] == v {
				bad[i] = true
			}
		}
		br, bc := r/3*3, c/3*3
		for dr := 0; dr < 3; dr++ {
			for dc := 0; dc < 3; dc++ {
				if k := idx(br+dr, bc+dc); k != i && g[k] == v {
					bad[i] = true
				}
			}
		}
	}
	return bad
}

// solved reports whether the grid is completely and validly filled.
func (g *Grid) solved() bool {
	bad := g.conflicts()
	for i := 0; i < 81; i++ {
		if g[i] == 0 || bad[i] {
			return false
		}
	}
	return true
}
