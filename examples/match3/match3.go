package main

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/sound/procedural"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

const (
	cols     = 8
	rows     = 8
	numTypes = 6
)

// Phases of the resolve loop. Input is only accepted in phaseIdle.
const (
	phaseIdle = iota
	phaseSwap
	phaseSwapBack
	phaseClear
	phaseFall
)

var (
	// colBG is the window background at Start, before a widget context exists
	// (main passes it as Config.Background). Inside the tree every chrome color
	// comes from the active theme (theme.Auto in Build, captured onto the state
	// for the ctx-less Canvas draw), so the game follows the platform light/dark
	// scheme. This matches the light identity's background.
	colBG = theme.Light().Bg
	// white is the gem gloss + colorblind glyph ink — part of the signature gem
	// look, not chrome, so it stays fixed across themes.
	white = paint.RGB(1, 1, 1)
)

// gem is a type's fill (dark→light gradient) plus a symbol id for colorblind
// distinctness (the inner white glyph shape).
type gem struct {
	base, light paint.Color
	sym         int
}

var gems = [numTypes]gem{
	{paint.RGB(0.86, 0.22, 0.30), paint.RGB(0.98, 0.42, 0.48), 0}, // red — circle
	{paint.RGB(0.95, 0.55, 0.18), paint.RGB(1.00, 0.74, 0.38), 1}, // orange — square
	{paint.RGB(0.95, 0.82, 0.25), paint.RGB(1.00, 0.93, 0.50), 2}, // yellow — diamond
	{paint.RGB(0.30, 0.78, 0.45), paint.RGB(0.48, 0.92, 0.62), 3}, // green — triangle
	{paint.RGB(0.28, 0.62, 0.95), paint.RGB(0.50, 0.80, 1.00), 4}, // blue — ring
	{paint.RGB(0.66, 0.44, 0.95), paint.RGB(0.82, 0.64, 1.00), 5}, // purple — plus
}

type cell struct{ r, c int }

var noCell = cell{-1, -1}

func (c cell) ok() bool { return c.r >= 0 && c.r < rows && c.c >= 0 && c.c < cols }

func adjacent(a, b cell) bool {
	dr, dc := abs(a.r-b.r), abs(a.c-b.c)
	return dr+dc == 1
}

// Match3 is the game widget; Seed makes a run reproducible, Sound is optional.
type Match3 struct {
	Seed  int64
	Sound *sound.Mixer
}

func (Match3) CreateState() widget.State { return &game{} }

type game struct {
	widget.StateBase[Match3]
	ctx  widget.Ctx
	rng  *rand.Rand
	snd  *sound.Mixer
	pool map[string]*sound.Sample

	grid  [rows][cols]int8 // gem type, -1 = empty
	phase int
	ctrl  *anim.Controller

	swapA, swapB cell // the pair in flight (phaseSwap/phaseSwapBack)
	swapValid    bool

	clearing [rows][cols]bool // gems shrinking out (phaseClear)
	fallFrom [rows][cols]int8 // a gem's start row for the fall (phaseFall); <0 = spawned above

	sel     cell // tap-selected gem, or noCell
	press   cell // gem under the current pointer-down
	swiped  bool // a swipe already fired this gesture
	pressAt geom.Pt

	score int
	chain int
	moves int
	size  geom.Size

	// th is the active theme, refreshed each Build. The Canvas draw closure has
	// no ctx, so chrome colors are read from here (th.Bg/Surface/Text/Muted/
	// Primary); the gem palette stays the game's own signature colors.
	th theme.Theme
}

func (s *game) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.rng = rand.New(rand.NewSource(s.W().Seed))
	s.snd = s.W().Sound
	s.sel = noCell
	s.pool = map[string]*sound.Sample{
		"swap":    procedural.Blip(520, 0.05),
		"bad":     procedural.Thud(),
		"clear":   procedural.Coin(),
		"cascade": procedural.Blip(880, 0.06),
	}
	s.ctrl = &anim.Controller{Curve: anim.EaseInOut, OnChange: func() {
		s.SetState(nil)
		if !s.ctrl.Running() && s.ctrl.Value() >= 1 {
			s.advance()
		}
	}}
	ctx.AddTicker(s.ctrl)
	s.newBoard()
}

func (s *game) Dispose() { s.ctx.RemoveTicker(s.ctrl) }

func (s *game) Build(ctx widget.Ctx) widget.Widget {
	// Resolve the platform theme and provide it to the tree; also capture it on
	// the state so the ctx-less Canvas draw closure can read chrome colors.
	s.th = theme.Auto(ctx)
	board := widget.Interactive{
		Handler: widget.Handler{
			OnPress:   s.onPress,
			OnDrag:    s.onDrag,
			OnRelease: s.onRelease,
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
	return widget.Provide[theme.Theme]{
		Value: s.th,
		Child: widget.Fill{Color: s.th.Bg, Child: board},
	}
}

// --- board setup ---

// newBoard fills the grid with no pre-existing matches (a fair start).
func (s *game) newBoard() {
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			for {
				t := int8(s.rng.Intn(numTypes))
				// Reject a type that would already make a run of three.
				if c >= 2 && s.grid[r][c-1] == t && s.grid[r][c-2] == t {
					continue
				}
				if r >= 2 && s.grid[r-1][c] == t && s.grid[r-2][c] == t {
					continue
				}
				s.grid[r][c] = t
				break
			}
		}
	}
	s.phase = phaseIdle
	s.ctx.Invalidate()
}

// --- input ---

func (s *game) onPress(p geom.Pt) {
	if s.phase != phaseIdle {
		return
	}
	s.press = s.cellAt(p)
	s.pressAt = p
	s.swiped = false
	if !s.press.ok() {
		return
	}
	if s.sel.ok() && adjacent(s.sel, s.press) {
		s.trySwap(s.sel, s.press)
		s.sel = noCell
	} else if s.sel == s.press {
		s.sel = noCell // tap again to deselect
	} else {
		s.sel = s.press
	}
	s.ctx.Invalidate()
}

func (s *game) onDrag(pos, _ geom.Pt) {
	if s.phase != phaseIdle || s.swiped || !s.press.ok() {
		return
	}
	d := geom.Pt{X: pos.X - s.pressAt.X, Y: pos.Y - s.pressAt.Y}
	cellPx := s.layout().cell
	if math.Hypot(float64(d.X), float64(d.Y)) < float64(cellPx)*0.4 {
		return // below the swipe threshold
	}
	// Swap with the neighbor in the dominant swipe direction.
	n := s.press
	if abs2(d.X) > abs2(d.Y) {
		if d.X > 0 {
			n.c++
		} else {
			n.c--
		}
	} else {
		if d.Y > 0 {
			n.r++
		} else {
			n.r--
		}
	}
	if n.ok() {
		s.swiped = true
		s.sel = noCell
		s.trySwap(s.press, n)
	}
}

func (s *game) onRelease() { s.press = noCell }

// --- resolve loop ---

func (s *game) trySwap(a, b cell) {
	s.swapA, s.swapB = a, b
	// Peek: is the swap a match? (grid is committed only on completion.)
	s.grid[a.r][a.c], s.grid[b.r][b.c] = s.grid[b.r][b.c], s.grid[a.r][a.c]
	s.swapValid = s.hasMatch()
	s.grid[a.r][a.c], s.grid[b.r][b.c] = s.grid[b.r][b.c], s.grid[a.r][a.c]
	s.moves++
	s.play("swap", a.c)
	s.start(phaseSwap, 140*time.Millisecond)
}

// advance runs at the end of each animated phase.
func (s *game) advance() {
	switch s.phase {
	case phaseSwap:
		if s.swapValid {
			a, b := s.swapA, s.swapB
			s.grid[a.r][a.c], s.grid[b.r][b.c] = s.grid[b.r][b.c], s.grid[a.r][a.c]
			s.chain = 0
			s.beginClear()
		} else {
			s.play("bad", s.swapA.c)
			s.start(phaseSwapBack, 140*time.Millisecond)
		}
	case phaseSwapBack:
		s.phase = phaseIdle
		s.ctx.Invalidate()
	case phaseClear:
		for r := 0; r < rows; r++ {
			for c := 0; c < cols; c++ {
				if s.clearing[r][c] {
					s.grid[r][c] = -1
				}
			}
		}
		s.beginFall()
	case phaseFall:
		if s.hasMatch() {
			s.chain++
			s.beginClear()
		} else {
			s.chain = 0
			s.phase = phaseIdle
			s.ctx.Invalidate()
		}
	}
}

// beginClear marks the current matches and animates them shrinking out.
func (s *game) beginClear() {
	s.clearing = s.matches()
	n := 0
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if s.clearing[r][c] {
				n++
			}
		}
	}
	mult := s.chain + 1
	s.score += n * 10 * mult
	if s.chain > 0 {
		s.play("cascade", cols/2)
	} else {
		s.play("clear", cols/2)
	}
	s.start(phaseClear, 200*time.Millisecond)
}

// beginFall compacts each column and spawns fresh gems above, recording where
// every gem falls from so the animation can slide it into place.
func (s *game) beginFall() {
	var next [rows][cols]int8
	for c := 0; c < cols; c++ {
		w := rows - 1
		for r := rows - 1; r >= 0; r-- {
			if s.grid[r][c] != -1 {
				next[w][c] = s.grid[r][c]
				s.fallFrom[w][c] = int8(r)
				w--
			}
		}
		spawn := int8(-1)
		for ; w >= 0; w-- {
			next[w][c] = int8(s.rng.Intn(numTypes))
			s.fallFrom[w][c] = spawn // above the top edge
			spawn--
		}
	}
	s.grid = next
	// Distance-scaled duration so tall drops don't feel instant.
	s.start(phaseFall, 260*time.Millisecond)
}

func (s *game) start(phase int, d time.Duration) {
	s.phase = phase
	s.ctrl.Duration = d
	s.ctrl.Jump(0)
	s.ctrl.Forward()
	s.ctx.Invalidate()
}

// --- match detection ---

func (s *game) hasMatch() bool {
	m := s.matches()
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			if m[r][c] {
				return true
			}
		}
	}
	return false
}

// matches returns the mask of gems that are part of a run of 3+ (rows or cols).
func (s *game) matches() [rows][cols]bool {
	var m [rows][cols]bool
	for r := 0; r < rows; r++ {
		run := 1
		for c := 1; c <= cols; c++ {
			if c < cols && s.grid[r][c] != -1 && s.grid[r][c] == s.grid[r][c-1] {
				run++
			} else {
				if run >= 3 {
					for k := c - run; k < c; k++ {
						m[r][k] = true
					}
				}
				run = 1
			}
		}
	}
	for c := 0; c < cols; c++ {
		run := 1
		for r := 1; r <= rows; r++ {
			if r < rows && s.grid[r][c] != -1 && s.grid[r][c] == s.grid[r-1][c] {
				run++
			} else {
				if run >= 3 {
					for k := r - run; k < r; k++ {
						m[k][c] = true
					}
				}
				run = 1
			}
		}
	}
	return m
}

// --- sound ---

func (s *game) play(name string, col int) {
	if s.snd == nil {
		return
	}
	smp := s.pool[name]
	if smp == nil {
		return
	}
	pan := (float64(col)/float64(cols-1))*2 - 1 // -1 left … +1 right
	pitch := 1.0
	if name == "cascade" {
		pitch = 1 + float64(s.chain)*0.12 // each cascade rings higher
	}
	s.snd.Play(smp, sound.PlayOptions{Volume: 0.5, Pan: pan, Pitch: pitch})
}

// --- layout + drawing ---

type box struct {
	x, y, cell float32
}

func (s *game) layout() box {
	const pad = 16
	top := float32(96) // score panel
	availW := s.size.W - 2*pad
	availH := s.size.H - top - pad
	cellPx := float32(math.Min(float64(availW)/cols, float64(availH)/rows))
	boardW := cellPx * cols
	return box{
		x:    (s.size.W - boardW) / 2,
		y:    top,
		cell: cellPx,
	}
}

func (s *game) cellAt(p geom.Pt) cell {
	b := s.layout()
	if b.cell == 0 {
		return noCell
	}
	// Floor (not int truncation): a tap in the band just left of / above the
	// board gives a negative offset, which must map to -1 (off-board), not 0.
	c := int(math.Floor(float64((p.X - b.x) / b.cell)))
	r := int(math.Floor(float64((p.Y - b.y) / b.cell)))
	cl := cell{r, c}
	if !cl.ok() {
		return noCell
	}
	return cl
}

func (s *game) draw(c paint.Canvas, size geom.Size) {
	s.size = size
	b := s.layout()
	th := s.th
	c.Clear(th.Bg)

	// Score panel.
	c.TextIn("bold", "MATCH 3", geom.Pt{X: b.x, Y: 40}, 30, th.Text)
	c.TextIn("", fmt.Sprintf("Score %d", s.score), geom.Pt{X: b.x, Y: 68}, 16, th.Muted)
	moves := fmt.Sprintf("Moves %d", s.moves)
	c.TextIn("", moves, geom.Pt{X: b.x + b.cell*cols - s.textW(moves, 16), Y: 68}, 16, th.Muted)
	if s.chain > 1 && s.phase == phaseClear {
		tag := fmt.Sprintf("x%d CHAIN!", s.chain+1)
		c.TextIn("bold", tag, geom.Pt{X: b.x + b.cell*cols - s.textW(tag, 18), Y: 42}, 18, th.Primary)
	}

	// Board backing — a neutral surface with a hairline border so the board
	// reads against the app background in both light and dark schemes.
	boardW, boardH := b.cell*cols, b.cell*rows
	backing := geom.RectXYWH(b.x-6, b.y-6, boardW+12, boardH+12)
	c.FillRRect(backing, 16, th.Surface)
	c.StrokeRRect(backing, 16, 1, th.Border)

	// Clip gems to the board so those falling in from above stay hidden until
	// they cross the top edge (otherwise they'd draw over the score panel).
	c.PushClip(geom.RectXYWH(b.x, b.y, boardW, boardH))
	p := s.ctrl.Value()
	for r := 0; r < rows; r++ {
		for cc := 0; cc < cols; cc++ {
			t := s.grid[r][cc]
			if t < 0 {
				continue
			}
			cx := b.x + float32(cc)*b.cell + b.cell/2
			cy := b.y + float32(r)*b.cell + b.cell/2
			scale := float32(1)

			switch s.phase {
			case phaseSwap, phaseSwapBack:
				// The two gems glide between their cells (grid is uncommitted,
				// so the gem logically at swapA travels to swapB and back).
				q := p
				if s.phase == phaseSwapBack {
					q = 1 - p // return trip
				}
				if (cell{r, cc}) == s.swapA {
					cx, cy = s.lerpCenter(b, s.swapA, s.swapB, q)
				} else if (cell{r, cc}) == s.swapB {
					cx, cy = s.lerpCenter(b, s.swapB, s.swapA, q)
				}
			case phaseClear:
				if s.clearing[r][cc] {
					scale = 1 - p // shrink out
				}
			case phaseFall:
				from := float32(s.fallFrom[r][cc])
				vr := anim.Lerp(from, float32(r), p)
				cy = b.y + vr*b.cell + b.cell/2
			}
			drawGem(c, cx, cy, b.cell*0.9*scale, gems[t])
		}
	}
	c.PopClip()

	// Selection ring.
	if s.sel.ok() && s.phase == phaseIdle {
		x := b.x + float32(s.sel.c)*b.cell
		y := b.y + float32(s.sel.r)*b.cell
		c.StrokeRRect(geom.RectXYWH(x+3, y+3, b.cell-6, b.cell-6), b.cell*0.28, 3, th.Primary)
	}

	c.TextIn("", "swipe a gem, or tap two neighbors, to swap",
		geom.Pt{X: b.x, Y: b.y + b.cell*rows + 26}, 13, th.Muted)

}

func (s *game) lerpCenter(b box, from, to cell, p float32) (x, y float32) {
	fx := b.x + float32(from.c)*b.cell + b.cell/2
	fy := b.y + float32(from.r)*b.cell + b.cell/2
	tx := b.x + float32(to.c)*b.cell + b.cell/2
	ty := b.y + float32(to.r)*b.cell + b.cell/2
	return anim.Lerp(fx, tx, p), anim.Lerp(fy, ty, p)
}

// drawGem renders one gem centered at (cx,cy) with side sz: a rounded, top-lit
// body, a gloss highlight, and a distinct white symbol.
func drawGem(c paint.Canvas, cx, cy, sz float32, g gem) {
	if sz < 1 {
		return
	}
	h := sz / 2
	rad := sz * 0.28
	r := geom.RectXYWH(cx-h, cy-h, sz, sz)
	c.FillRRectGradient(r, rad, g.light, g.base, false)

	// Gloss: a pre-blended highlight streak across the top — a lighter tint of
	// the gem rather than a PushOpacity layer. With 64 gems/frame, pre-blending
	// avoids allocating a full-surface layer pixmap per gem (a perf win; the
	// HiDPI layer-clipping bug this once tripped is fixed in gg).
	c.FillRRect(geom.RectXYWH(cx-h*0.66, cy-h*0.7, sz*0.66, sz*0.28), sz*0.14, mix(g.base, white, 0.55))

	// Symbol: a distinct white glyph for colorblind-safe identification.
	drawSymbol(c, cx, cy, sz*0.34, g.sym)
}

func drawSymbol(c paint.Canvas, cx, cy, s float32, sym int) {
	switch sym {
	case 0: // circle
		c.FillRRect(geom.RectXYWH(cx-s, cy-s, 2*s, 2*s), s, white)
	case 1: // square
		c.FillRRect(geom.RectXYWH(cx-s*0.85, cy-s*0.85, s*1.7, s*1.7), s*0.2, white)
	case 2: // diamond
		p := paint.NewPath()
		p.MoveTo(geom.Pt{X: cx, Y: cy - s}).LineTo(geom.Pt{X: cx + s, Y: cy}).
			LineTo(geom.Pt{X: cx, Y: cy + s}).LineTo(geom.Pt{X: cx - s, Y: cy}).Close()
		c.FillPath(p, white)
	case 3: // triangle
		p := paint.NewPath()
		p.MoveTo(geom.Pt{X: cx, Y: cy - s}).LineTo(geom.Pt{X: cx + s, Y: cy + s*0.8}).
			LineTo(geom.Pt{X: cx - s, Y: cy + s*0.8}).Close()
		c.FillPath(p, white)
	case 4: // ring
		c.StrokeRRect(geom.RectXYWH(cx-s*0.85, cy-s*0.85, s*1.7, s*1.7), s*0.85, s*0.5, white)
	case 5: // plus
		t := s * 0.42
		c.FillRRect(geom.RectXYWH(cx-t, cy-s, 2*t, 2*s), t*0.6, white)
		c.FillRRect(geom.RectXYWH(cx-s, cy-t, 2*s, 2*t), t*0.6, white)
	}
}

// --- small helpers ---

// mix blends a→b by t (0..1), opaque.
func mix(a, b paint.Color, t float32) paint.Color {
	return paint.Color{
		R: a.R + (b.R-a.R)*t,
		G: a.G + (b.G-a.G)*t,
		B: a.B + (b.B-a.B)*t,
		A: 1,
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func abs2(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

// textW estimates a string's width for right-alignment (approx, monospace-ish).
func (s *game) textW(str string, size float32) float32 {
	return s.ctx.Painter().MeasureWidthIn("", str, size)
}
