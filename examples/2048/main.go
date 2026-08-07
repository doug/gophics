// Command 2048 is the sliding-tile puzzle: arrow keys or swipe to slide the
// board; equal tiles merge; reach 2048. It is one widget.Canvas driving smooth
// slide + spawn animations off a per-frame Ticker — a compact showcase of the
// paint + input + animation path.
//
//	go run ./examples/2048
package main

import (
	"fmt"
	"log"
	"math"
	"math/rand"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

const boardN = 4

// textDark/textLite are the tile ink colors — part of the classic 2048 identity,
// consumed only by tileText(). The board CHROME (window, frame, slots, header,
// buttons, chips) is themed and follows the platform light/dark scheme; see draw.
var (
	textDark = paint.RGB(0.47, 0.43, 0.39)
	textLite = paint.RGB(0.98, 0.96, 0.94)
)

// tileColor is the classic 2048 palette.
func tileColor(v int) paint.Color {
	switch v {
	case 2:
		return paint.RGB(0.93, 0.89, 0.85)
	case 4:
		return paint.RGB(0.93, 0.88, 0.78)
	case 8:
		return paint.RGB(0.95, 0.69, 0.47)
	case 16:
		return paint.RGB(0.96, 0.58, 0.39)
	case 32:
		return paint.RGB(0.96, 0.49, 0.37)
	case 64:
		return paint.RGB(0.96, 0.37, 0.23)
	case 128:
		return paint.RGB(0.93, 0.81, 0.45)
	case 256:
		return paint.RGB(0.93, 0.80, 0.38)
	case 512:
		return paint.RGB(0.93, 0.78, 0.31)
	case 1024:
		return paint.RGB(0.93, 0.77, 0.25)
	case 2048:
		return paint.RGB(0.93, 0.76, 0.18)
	default:
		return paint.RGB(0.24, 0.23, 0.20) // beyond 2048
	}
}

func tileText(v int) paint.Color {
	if v <= 4 {
		return textDark
	}
	return textLite
}

type Game struct{}

func (Game) CreateState() widget.State { return &game{} }

type game struct {
	widget.StateBase[Game]
	grid        [boardN][boardN]int
	score, best int
	over        bool

	anims   []mov // active slide animation
	sliding bool
	slideT  float64

	spR, spC int // spawned cell for its pop-in
	spawning bool
	spawnT   float64

	merges  [][2]int // cells that merged this move — they pop after the slide
	popping bool
	popT    float64

	swDX, swDY float32 // swipe accumulator
	swiped     bool
	pressPos   geom.Pt
	newBtn     geom.Rect   // set during draw, hit-tested on tap
	th         theme.Theme // chrome theme captured in Build (draw has no ctx)
	ctx        widget.Ctx
}

// mov is one tile sliding from (fr,fc) to (tr,tc).
type mov struct{ v, fr, fc, tr, tc int }

const (
	slideDur = 0.13 // tile glide — long enough to read the motion
	spawnDur = 0.16 // new-tile pop-in
	popDur   = 0.18 // merged-tile bounce
)

// stateHook, if set, receives the game state on mount — for tests to drive and
// inspect input end to end.
var stateHook func(*game)

func (s *game) Init(ctx widget.Ctx) {
	s.ctx = ctx
	ctx.AddTicker(s)
	s.reset()
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *game) reset() {
	s.grid = [boardN][boardN]int{}
	s.score, s.over = 0, false
	s.anims, s.sliding, s.spawning, s.popping = nil, false, false, false
	s.merges = nil
	s.addRandomTile()
	s.addRandomTile()
}

func (s *game) addRandomTile() {
	var empty [][2]int
	for r := 0; r < boardN; r++ {
		for c := 0; c < boardN; c++ {
			if s.grid[r][c] == 0 {
				empty = append(empty, [2]int{r, c})
			}
		}
	}
	if len(empty) == 0 {
		return
	}
	cell := empty[rand.Intn(len(empty))]
	v := 2
	if rand.Intn(10) == 0 {
		v = 4
	}
	s.grid[cell[0]][cell[1]] = v
	s.spR, s.spC = cell[0], cell[1]
	s.spawning, s.spawnT = true, 0
}

// Tick advances the slide/spawn animations; it idles (returns false) when
// nothing is animating, and a keypress/swipe re-arms it via SetState.
func (s *game) Tick(dt float64) bool {
	if s.sliding {
		if s.slideT += dt / slideDur; s.slideT >= 1 {
			s.slideT, s.sliding = 1, false
			s.afterSlide()
		}
		s.ctx.Invalidate()
	}
	if s.spawning {
		if s.spawnT += dt / spawnDur; s.spawnT >= 1 {
			s.spawnT, s.spawning = 1, false
		}
		s.ctx.Invalidate()
	}
	if s.popping {
		if s.popT += dt / popDur; s.popT >= 1 {
			s.popT, s.popping = 1, false
		}
		s.ctx.Invalidate()
	}
	return s.sliding || s.spawning || s.popping
}

func (s *game) afterSlide() {
	s.anims = nil
	if len(s.merges) > 0 { // the tiles that combined bounce once
		s.popping, s.popT = true, 0
	}
	s.addRandomTile()
	if !s.movesAvailable() {
		s.over = true
	}
}

func (s *game) movesAvailable() bool {
	for r := 0; r < boardN; r++ {
		for c := 0; c < boardN; c++ {
			if s.grid[r][c] == 0 {
				return true
			}
			if c+1 < boardN && s.grid[r][c] == s.grid[r][c+1] {
				return true
			}
			if r+1 < boardN && s.grid[r][c] == s.grid[r+1][c] {
				return true
			}
		}
	}
	return false
}

// lineCoords returns the four lines for a direction, each ordered front-first
// (the edge tiles pile against). dir: 0 left, 1 right, 2 up, 3 down.
func lineCoords(dir int) [boardN][boardN][2]int {
	var out [boardN][boardN][2]int
	for i := 0; i < boardN; i++ {
		for j := 0; j < boardN; j++ {
			var r, c int
			switch dir {
			case 0:
				r, c = i, j
			case 1:
				r, c = i, boardN-1-j
			case 2:
				r, c = j, i
			case 3:
				r, c = boardN-1-j, i
			}
			out[i][j] = [2]int{r, c}
		}
	}
	return out
}

// slide is the pure game step: given a grid and direction it returns the
// resulting grid, the per-tile movements (for the slide animation), the score
// gained, and whether anything moved. No state, so it's unit-testable.
func slide(grid [boardN][boardN]int, dir int) (out [boardN][boardN]int, movements []mov, gained int, moved bool) {
	for _, coords := range lineCoords(dir) {
		type sc struct{ v, r, c int }
		var vals []sc
		for _, cc := range coords {
			if v := grid[cc[0]][cc[1]]; v != 0 {
				vals = append(vals, sc{v, cc[0], cc[1]})
			}
		}
		slot := 0
		for i := 0; i < len(vals); {
			dest := coords[slot]
			if i+1 < len(vals) && vals[i].v == vals[i+1].v {
				nv := vals[i].v * 2
				out[dest[0]][dest[1]] = nv
				gained += nv
				movements = append(movements,
					mov{vals[i].v, vals[i].r, vals[i].c, dest[0], dest[1]},
					mov{vals[i+1].v, vals[i+1].r, vals[i+1].c, dest[0], dest[1]})
				moved = true
				i += 2
			} else {
				out[dest[0]][dest[1]] = vals[i].v
				movements = append(movements, mov{vals[i].v, vals[i].r, vals[i].c, dest[0], dest[1]})
				if vals[i].r != dest[0] || vals[i].c != dest[1] {
					moved = true
				}
				i++
			}
			slot++
		}
	}
	return out, movements, gained, moved
}

// move applies a slide and, if anything moved, commits it + starts the animation.
func (s *game) move(dir int) {
	if s.sliding || s.over {
		return
	}
	out, movements, gained, moved := slide(s.grid, dir)
	if !moved {
		return
	}
	s.grid = out
	if s.score += gained; s.score > s.best {
		s.best = s.score
	}
	// A destination reached by two movements is a merge — remember it so it can
	// pop when the slide lands.
	destCount := map[[2]int]int{}
	for _, m := range movements {
		destCount[[2]int{m.tr, m.tc}]++
	}
	s.merges = s.merges[:0]
	for cell, n := range destCount {
		if n >= 2 {
			s.merges = append(s.merges, cell)
		}
	}
	s.anims, s.sliding, s.slideT = movements, true, 0
	s.popping = false
	s.ctx.Invalidate()
}

func (s *game) Build(ctx widget.Ctx) widget.Widget {
	// Derive the theme from the platform color scheme, provide it to the tree,
	// and capture it for draw (which has no ctx) so the chrome follows light/dark.
	th := theme.Auto(ctx)
	s.th = th
	content := widget.Interactive{
		Handler: widget.Handler{
			OnKey: func(k shell.Key) {
				if k.Kind != shell.KeyPress {
					return
				}
				switch k.Code {
				case shell.KeyLeft:
					s.move(0)
				case shell.KeyRight:
					s.move(1)
				case shell.KeyUp:
					s.move(2)
				case shell.KeyDown:
					s.move(3)
				case shell.KeyEnter, shell.KeySpace:
					s.reset()
					s.ctx.Invalidate()
				}
			},
			OnPress: func(p geom.Pt) {
				s.pressPos, s.swDX, s.swDY, s.swiped = p, 0, 0, false
			},
			OnDrag: func(_, d geom.Pt) {
				if s.swiped {
					return
				}
				s.swDX, s.swDY = s.swDX+d.X, s.swDY+d.Y
				const th = 24
				switch {
				case abs(s.swDX) < th && abs(s.swDY) < th:
					return
				case abs(s.swDX) > abs(s.swDY):
					s.swiped = true
					if s.swDX < 0 {
						s.move(0) // left
					} else {
						s.move(1) // right
					}
				default:
					s.swiped = true
					if s.swDY < 0 {
						s.move(2) // up
					} else {
						s.move(3) // down
					}
				}
			},
			OnTap: func() {
				if s.over || s.newBtn.Contains(s.pressPos) {
					s.reset()
					s.ctx.Invalidate()
				}
			},
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Fill{Color: th.Bg, Child: content},
	}
}

func (s *game) draw(c paint.Canvas, sz geom.Size) {
	th := s.th
	c.Clear(th.Bg)
	// headerH must clear the header's tallest element — the New Game button,
	// whose bottom is at y=110 (top 78 + height 32) — plus a gap before the board.
	const pad, headerH = 18, 124
	board := min(sz.W-2*pad, sz.H-headerH-pad)
	if board < 40 {
		return
	}
	bx, by := (sz.W-board)/2, float32(headerH)

	// Header: title, score/best chips, New Game.
	p := s.ctx.Painter()
	c.TextIn("", "2048", geom.Pt{X: bx, Y: 56}, 42, th.Text)
	drawChip(c, geom.RectXYWH(bx+board-158, 24, 74, 48), "SCORE", s.score, th, p)
	drawChip(c, geom.RectXYWH(bx+board-78, 24, 78, 48), "BEST", s.best, th, p)
	// Size the button to its label (+ padding) so the text never clips.
	const btnLabel, btnSize, btnPadX = "New Game", float32(14), float32(16)
	lblW := p.MeasureWidth(btnLabel, btnSize)
	s.newBtn = geom.RectXYWH(bx, 78, lblW+2*btnPadX, 32)
	c.FillRRect(s.newBtn, 6, th.Primary)
	c.TextIn("", btnLabel, geom.Pt{X: s.newBtn.Min.X + btnPadX, Y: 78 + 16 + btnSize*0.35}, btnSize, th.OnPrimary)

	// Board + slots — neutral chrome that frames the tiles in both schemes:
	// the frame (Border) sits darker than Bg on light and lighter on dark, and
	// the empty slots (Surface) recede so the classic tile ramp stays the focus.
	c.FillRRect(geom.RectXYWH(bx, by, board, board), 8, th.Border)
	gap := board * 0.028
	cell := (board - gap*float32(boardN+1)) / boardN
	xy := func(r, cc int) (float32, float32) {
		return bx + gap + float32(cc)*(cell+gap), by + gap + float32(r)*(cell+gap)
	}
	for r := 0; r < boardN; r++ {
		for cc := 0; cc < boardN; cc++ {
			x, y := xy(r, cc)
			c.FillRRect(geom.RectXYWH(x, y, cell, cell), 5, th.Surface)
		}
	}

	// drawTile paints one tile at full size; scale (≠1) applies a transform about
	// the tile centre for spawn/merge pops, so the number glyph is rasterized once
	// at a whole-pixel size and scaled by the GPU — no per-frame atlas churn.
	drawTile := func(v int, x, y, scale float32) {
		if v == 0 {
			return
		}
		cx, cy := x+cell/2, y+cell/2
		if scale != 1 {
			c.PushTransform(paint.Transform{SX: scale, SY: scale, PivotX: cx, PivotY: cy})
		}
		c.FillRRect(geom.RectXYWH(x, y, cell, cell), 5, tileColor(v))
		txt := fmt.Sprintf("%d", v)
		fs := tileFont(v, cell)
		tw := p.MeasureWidth(txt, fs)
		c.TextIn("", txt, geom.Pt{X: cx - tw/2, Y: cy + fs*0.35}, fs, tileText(v))
		if scale != 1 {
			c.PopTransform()
		}
	}

	if s.sliding {
		t := easeOut(float32(s.slideT))
		for _, m := range s.anims {
			fx, fy := xy(m.fr, m.fc)
			tx, ty := xy(m.tr, m.tc)
			drawTile(m.v, lerp(fx, tx, t), lerp(fy, ty, t), 1)
		}
	} else {
		for r := 0; r < boardN; r++ {
			for cc := 0; cc < boardN; cc++ {
				scale := float32(1)
				switch {
				case s.spawning && r == s.spR && cc == s.spC:
					scale = spawnScale(float32(s.spawnT))
				case s.popping && s.isMerge(r, cc):
					scale = mergePop(float32(s.popT))
				}
				x, y := xy(r, cc)
				drawTile(s.grid[r][cc], x, y, scale)
			}
		}
	}

	if s.over {
		c.FillRRect(geom.RectXYWH(bx, by, board, board), 8, th.Bg.WithAlpha(0.62))
		mid := bx + board/2
		center(c, p, "Game Over", mid, by+board/2, 30, th.Text)
		center(c, p, "tap to try again", mid, by+board/2+30, 15, th.Muted)
	}
}

// center draws s horizontally centred on x at baseline y (measured, not guessed).
func center(c paint.Canvas, p *paint.Painter, s string, x, y, size float32, col paint.Color) {
	c.TextIn("", s, geom.Pt{X: x - p.MeasureWidth(s, size)/2, Y: y}, size, col)
}

// isMerge reports whether cell (r,c) is a destination that merged this move.
func (s *game) isMerge(r, c int) bool {
	for _, m := range s.merges {
		if m[0] == r && m[1] == c {
			return true
		}
	}
	return false
}

func drawChip(c paint.Canvas, r geom.Rect, label string, val int, th theme.Theme, p *paint.Painter) {
	c.FillRRect(r, 6, th.Border)
	center(c, p, label, r.Min.X+r.Dx()/2, r.Min.Y+17, 10, th.Muted)
	center(c, p, fmt.Sprintf("%d", val), r.Min.X+r.Dx()/2, r.Min.Y+40, 18, th.Text)
}

// tileFont shrinks the number as it grows more digits. The result is rounded to
// a whole pixel: pop/spawn animations scale the glyph with a transform (not by
// re-sizing the font), and the board size snaps per resize, so the text
// rasterizer only ever sees a small set of integer sizes — no per-frame churn.
func tileFont(v int, cell float32) float32 {
	var f float32
	switch {
	case v >= 1000:
		f = cell * 0.30
	case v >= 100:
		f = cell * 0.38
	default:
		f = cell * 0.46
	}
	return float32(math.Round(float64(f)))
}

func lerp(a, b, t float32) float32 { return a + (b-a)*t }
func easeOut(t float32) float32    { u := 1 - t; return 1 - u*u*u }

// spawnScale grows a new tile 0→1 with a slight overshoot (easeOutBack) so it
// pops in rather than fading up.
func spawnScale(t float32) float32 {
	const s = 1.70158
	u := t - 1
	return 1 + (s+1)*u*u*u + s*u*u
}

// mergePop is a one-shot bounce (1 → ~1.2 → 1) for a tile that just merged.
func mergePop(t float32) float32 {
	return 1 + 0.22*float32(math.Sin(math.Pi*float64(t)))
}
func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}

func main() {
	if err := app.Run(Game{}, app.Config{
		Title:      "2048",
		Size:       geom.Size{W: 420, H: 560},
		Background: theme.Light().Bg, // pre-context window fill; the tree themes itself
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
