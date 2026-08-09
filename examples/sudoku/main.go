// Command sudoku is a full Sudoku game: tap a cell (or arrow-key to it) and
// type 1-9 to fill it, toggle Notes for pencil marks, and get live conflict
// highlighting. Puzzles are generated with a guaranteed-unique solution. It is
// one widget.Canvas driving the board plus an on-screen number pad, so it plays
// with a keyboard on desktop and by touch on mobile.
//
//	go run ./examples/sudoku
package main

import (
	"log"
	"math/rand"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// palette is the board's resolved chrome — the colors the Canvas Draw func needs,
// captured from the active theme.Theme each Build so the whole game follows the
// platform light/dark scheme. The signature Sudoku semantics (player blue,
// conflict red, selection/peer/same washes) are preserved but re-expressed in
// theme tokens so they read on both light and dark cells. Cell washes are Lerped
// onto Surface (not alpha-over-bg) so an empty highlighted cell stays an opaque,
// legible surface color in either scheme.
type palette struct {
	bg        paint.Color // page/window background
	cellBg    paint.Color // empty cell
	selBg     paint.Color // selected cell wash
	peerBg    paint.Color // same row/col/box wash
	sameBg    paint.Color // matching-value wash
	badBg     paint.Color // conflict cell wash
	ink       paint.Color // given clue + title text
	player    paint.Color // player-entered digit (blue → Primary)
	badFg     paint.Color // conflict digit (red → Danger)
	noteInk   paint.Color // pencil marks
	lineThin  paint.Color // thin grid lines
	lineThick paint.Color // thick box lines
	btnBg     paint.Color // number pad + control button chrome
	accent    paint.Color // active control / notes-on
	onAccent  paint.Color // label on an active (accent) control
	disabled  paint.Color // exhausted number-pad digit
	good      paint.Color // "Solved!" status
	sub       paint.Color // idle status
}

// paletteFrom maps the theme tokens onto the Sudoku chrome, keeping the game's
// signature look while adapting it for contrast in both schemes.
func paletteFrom(th theme.Theme) palette {
	surf := th.Surface
	return palette{
		bg:        th.Bg,
		cellBg:    surf,
		selBg:     paint.Lerp(surf, th.Primary, 0.34),
		peerBg:    paint.Lerp(surf, th.Primary, 0.12),
		sameBg:    paint.Lerp(surf, th.Primary, 0.22),
		badBg:     paint.Lerp(surf, th.Danger, 0.28),
		ink:       th.Text,
		player:    th.Primary,
		badFg:     th.Danger,
		noteInk:   th.Muted,
		lineThin:  th.Border,
		lineThick: th.Muted, // darker/stronger box divider
		btnBg:     surf,
		accent:    th.Primary,
		onAccent:  th.OnPrimary,
		disabled:  th.Muted,
		good:      th.Success,
		sub:       th.Muted,
	}
}

// clueTarget is how few givens generation aims for (uniqueness may keep more).
const clueTarget = 32

type Game struct{}

func (Game) CreateState() widget.State { return &game{} }

type game struct {
	widget.StateBase[Game]
	rng      *rand.Rand
	puzzle   Grid
	solution Grid
	board    Grid       // current player state; starts as puzzle
	given    [81]bool   // fixed clue cells
	notes    [81]uint16 // pencil marks, bit v set = note v present
	sel      int        // selected cell index, -1 = none
	noteMode bool
	won      bool

	pal       palette // chrome colors captured from the active theme in Build
	ctx       widget.Ctx
	boardRect geom.Rect // set during draw, hit-tested on press
	cell      float32
	numBtn    [9]geom.Rect
	notesBtn  geom.Rect
	eraseBtn  geom.Rect
	newBtn    geom.Rect
}

// stateHook, if set, receives the game state on mount — for tests to drive and
// inspect input end to end.
var stateHook func(*game)

func (s *game) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	s.reset()
	if stateHook != nil {
		stateHook(s)
	}
}

func (s *game) reset() {
	s.puzzle, s.solution = generate(s.rng, clueTarget)
	s.board = s.puzzle
	for i := 0; i < 81; i++ {
		s.given[i] = s.puzzle[i] != 0
		s.notes[i] = 0
	}
	s.sel = -1
	s.noteMode = false
	s.won = false
}

// input places digit v (1..9) in the selected cell, honoring note mode and
// leaving clue cells untouched.
func (s *game) input(v int) {
	if s.sel < 0 || s.given[s.sel] || s.won {
		return
	}
	if s.noteMode {
		if s.board[s.sel] == 0 {
			s.notes[s.sel] ^= 1 << uint(v) // toggle the pencil mark
		}
	} else if s.board[s.sel] == v {
		s.board[s.sel] = 0 // typing the same digit clears it
	} else {
		s.board[s.sel] = v
		s.notes[s.sel] = 0
		s.clearPeerNotes(s.sel, v)
		if s.board.solved() {
			s.won = true
		}
	}
	s.ctx.Invalidate()
}

// clearPeerNotes removes v from the pencil marks of every cell that shares a
// row, column, or box with i — the tidy-up a player would do by hand.
func (s *game) clearPeerNotes(i, v int) {
	r, c := i/9, i%9
	mask := ^(uint16(1) << uint(v))
	for j := 0; j < 9; j++ {
		s.notes[idx(r, j)] &= mask
		s.notes[idx(j, c)] &= mask
	}
	br, bc := r/3*3, c/3*3
	for dr := 0; dr < 3; dr++ {
		for dc := 0; dc < 3; dc++ {
			s.notes[idx(br+dr, bc+dc)] &= mask
		}
	}
}

func (s *game) erase() {
	if s.sel < 0 || s.given[s.sel] || s.won {
		return
	}
	s.board[s.sel] = 0
	s.notes[s.sel] = 0
	s.ctx.Invalidate()
}

// move steps the selection by (dc,dr) cells, clamped to the board.
func (s *game) move(dc, dr int) {
	if s.sel < 0 {
		s.sel = idx(4, 4)
	} else {
		r := clampi(s.sel/9+dr, 0, 8)
		c := clampi(s.sel%9+dc, 0, 8)
		s.sel = idx(r, c)
	}
	s.ctx.Invalidate()
}

func (s *game) toggleNotes() {
	s.noteMode = !s.noteMode
	s.ctx.Invalidate()
}

func (s *game) onPress(p geom.Pt) {
	if s.boardRect.Contains(p) {
		c := int((p.X - s.boardRect.Min.X) / s.cell)
		r := int((p.Y - s.boardRect.Min.Y) / s.cell)
		if r >= 0 && r < 9 && c >= 0 && c < 9 {
			s.sel = idx(r, c)
			s.ctx.Invalidate()
		}
		return
	}
	for v := 0; v < 9; v++ {
		if s.numBtn[v].Contains(p) {
			s.input(v + 1)
			return
		}
	}
	switch {
	case s.notesBtn.Contains(p):
		s.toggleNotes()
	case s.eraseBtn.Contains(p):
		s.erase()
	case s.newBtn.Contains(p):
		s.reset()
		s.ctx.Invalidate()
	}
}

func (s *game) Build(ctx widget.Ctx) widget.Widget {
	// Resolve the theme from the platform color scheme, capture the chrome colors
	// for the Canvas Draw func (which has no ctx), and provide the theme so the
	// whole game follows light/dark automatically.
	th := theme.Auto(ctx)
	s.pal = paletteFrom(th)
	board := widget.Interactive{
		Handler: widget.Handler{
			OnKey: func(k shell.Key) {
				if k.Kind != shell.KeyPress {
					return
				}
				switch k.Code {
				case shell.KeyLeft:
					s.move(-1, 0)
				case shell.KeyRight:
					s.move(1, 0)
				case shell.KeyUp:
					s.move(0, -1)
				case shell.KeyDown:
					s.move(0, 1)
				case shell.KeyBackspace, shell.KeyDelete:
					s.erase()
				case shell.KeySpace:
					s.toggleNotes()
				}
			},
			OnText: func(t string) {
				for _, ch := range t {
					if ch >= '1' && ch <= '9' {
						s.input(int(ch - '0'))
					}
				}
			},
			OnPress: func(p geom.Pt) { s.onPress(p) },
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Fill{Color: th.Bg, Child: board},
	}
}

func (s *game) draw(c paint.Canvas, sz geom.Size) {
	p := s.pal
	c.Clear(p.bg)
	const pad = 16
	c.TextIn("", "Sudoku", geom.Pt{X: pad, Y: 34}, 24, p.ink)
	if status, col := s.status(); status != "" {
		w := s.ctx.Painter().MeasureWidthIn("", status, 16)
		c.TextIn("", status, geom.Pt{X: sz.W - pad - w, Y: 34}, 16, col)
	}

	const belowH = 118 // number pad + control row
	top := float32(52)
	B := min(sz.W-2*pad, sz.H-top-belowH-pad)
	if B < 90 {
		return
	}
	bx, by := (sz.W-B)/2, top
	cell := B / 9
	s.boardRect = geom.RectXYWH(bx, by, B, B)
	s.cell = cell

	bad := s.board.conflicts()
	selR, selC, selV := -1, -1, 0
	if s.sel >= 0 {
		selR, selC, selV = s.sel/9, s.sel%9, s.board[s.sel]
	}

	// Cell backgrounds: selection, conflicts, peers, matching value.
	for i := 0; i < 81; i++ {
		r, cc := i/9, i%9
		var col paint.Color
		switch {
		case i == s.sel:
			col = p.selBg
		case bad[i]:
			col = p.badBg
		case selR >= 0 && (r == selR || cc == selC || (r/3 == selR/3 && cc/3 == selC/3)):
			col = p.peerBg
		case selV != 0 && s.board[i] == selV:
			col = p.sameBg
		default:
			col = p.cellBg
		}
		c.FillRect(geom.RectXYWH(bx+float32(cc)*cell, by+float32(r)*cell, cell, cell), col)
	}

	// Grid lines, thick every third for the boxes.
	for k := 0; k <= 9; k++ {
		off := float32(k) * cell
		w, col := float32(1), p.lineThin
		if k%3 == 0 {
			w, col = 2.4, p.lineThick
		}
		c.Line(geom.Pt{X: bx, Y: by + off}, geom.Pt{X: bx + B, Y: by + off}, w, col)
		c.Line(geom.Pt{X: bx + off, Y: by}, geom.Pt{X: bx + off, Y: by + B}, w, col)
	}

	// Digits and pencil marks.
	for i := 0; i < 81; i++ {
		r, cc := i/9, i%9
		x, y := bx+float32(cc)*cell, by+float32(r)*cell
		if v := s.board[i]; v != 0 {
			col := p.player
			if s.given[i] {
				col = p.ink
			}
			if bad[i] {
				col = p.badFg
			}
			fs := cell * 0.62
			c.TextIn("", digit(v), geom.Pt{X: x + cell/2 - fs*0.28, Y: y + cell/2 + fs*0.35}, fs, col)
		} else if s.notes[i] != 0 {
			ns := cell / 3
			for v := 1; v <= 9; v++ {
				if s.notes[i]&(1<<uint(v)) != 0 {
					nx := x + float32((v-1)%3)*ns
					ny := y + float32((v-1)/3)*ns
					fs := ns * 0.72
					c.TextIn("", digit(v), geom.Pt{X: nx + ns/2 - fs*0.28, Y: ny + ns/2 + fs*0.34}, fs, p.noteInk)
				}
			}
		}
	}

	// Number pad: one button per digit, dimmed once all nine are placed.
	counts := s.digitCounts()
	padY := by + B + 14
	padH := cell * 0.92
	for v := 1; v <= 9; v++ {
		rect := geom.RectXYWH(bx+float32(v-1)*cell+1.5, padY, cell-3, padH)
		s.numBtn[v-1] = rect
		c.FillRRect(rect, 6, p.btnBg)
		fg := p.ink
		if counts[v] >= 9 {
			fg = p.disabled
		}
		fs := padH * 0.5
		c.TextIn("", digit(v), geom.Pt{X: rect.Min.X + rect.Dx()/2 - fs*0.28, Y: rect.Min.Y + rect.Dy()/2 + fs*0.35}, fs, fg)
	}

	// Controls.
	cy := padY + padH + 12
	const ch, gap = 40, 8
	third := (B - 2*gap) / 3
	s.notesBtn = geom.RectXYWH(bx, cy, third, ch)
	s.eraseBtn = geom.RectXYWH(bx+third+gap, cy, third, ch)
	s.newBtn = geom.RectXYWH(bx+2*(third+gap), cy, third, ch)
	s.button(c, s.notesBtn, "Notes", s.noteMode)
	s.button(c, s.eraseBtn, "Erase", false)
	s.button(c, s.newBtn, "New", false)
}

func (s *game) status() (string, paint.Color) {
	switch {
	case s.won:
		return "Solved!", s.pal.good
	case s.noteMode:
		return "Notes on", s.pal.accent
	default:
		return "", s.pal.sub
	}
}

func (s *game) button(c paint.Canvas, rect geom.Rect, label string, active bool) {
	bgc, fg := s.pal.btnBg, s.pal.ink
	if active {
		bgc, fg = s.pal.accent, s.pal.onAccent
	}
	c.FillRRect(rect, 8, bgc)
	w := s.ctx.Painter().MeasureWidthIn("", label, 14.5)
	c.TextIn("", label, geom.Pt{X: rect.Min.X + (rect.Dx()-w)/2, Y: rect.Min.Y + rect.Dy()/2 + 5}, 14.5, fg)
}

func (s *game) digitCounts() [10]int {
	var n [10]int
	for i := 0; i < 81; i++ {
		n[s.board[i]]++
	}
	return n
}

// digit renders a single 1-9 value without importing strconv.
func digit(v int) string { return string(rune('0' + v)) }

func clampi(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func main() {
	if err := app.Run(Game{}, app.Config{
		Title:      "Sudoku",
		Size:       geom.Size{W: 400, H: 600},
		Background: theme.Light().Bg,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
