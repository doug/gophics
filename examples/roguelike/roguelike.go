package main

import (
	"fmt"
	"image"
	"math"
	"math/rand"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/sound/procedural"
	"github.com/doug/gophics/widget"
)

// Roguelike is the root widget: a tile dungeon crawler rendered entirely with
// paint.DrawSprite from one procedurally-generated atlas. Sound is optional
// (nil → silent, e.g. in tests).
type Roguelike struct {
	Seed  int64
	Sound *sound.Mixer
}

func (Roguelike) CreateState() widget.State { return &gameState{} }

// stateHook lets tests observe the mounted state.
var stateHook func(*gameState)

type gameState struct {
	widget.StateBase[Roguelike]
	ctx      widget.Ctx
	g        *Game
	atlas    *image.RGBA
	restarts int64

	snd     *sound.Mixer
	rng     *rand.Rand
	samples map[SoundID]*sound.Sample
	music   *sound.Voice

	origin geom.Pt // last camera origin (world px), for tap→cell mapping
	ts     float32 // last tile size on screen
}

var (
	colBG     = paint.RGB(0.04, 0.045, 0.06)
	colPanel  = paint.Color{R: 0.08, G: 0.09, B: 0.12, A: 0.93}
	colInk    = paint.RGB(0.86, 0.88, 0.92)
	colDim    = paint.RGB(0.55, 0.58, 0.64)
	colHP     = paint.RGB(0.80, 0.27, 0.30)
	colHPbg   = paint.Color{R: 1, G: 1, B: 1, A: 0.12}
	colCoin   = paint.RGB(0.90, 0.74, 0.30)
	colBanner = paint.Color{R: 0, G: 0, B: 0, A: 0.62}
)

func (s *gameState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.atlas = buildAtlas()
	s.rng = rand.New(rand.NewSource(1))
	s.snd = s.W().Sound
	if s.snd != nil {
		s.samples = map[SoundID]*sound.Sample{
			SndHit:     procedural.Hit(),
			SndCoin:    procedural.Coin(),
			SndPotion:  procedural.Blip(720, 0.14),
			SndDescend: procedural.Thud(),
			SndDie:     procedural.Blip(140, 0.4),
			SndWin:     procedural.Coin(),
		}
		s.music = s.snd.PlaySource(procedural.DungeonMusic(1),
			sound.PlayOptions{Volume: 0.30, FadeIn: 2 * time.Second}) // ambient loop, fades in
	}
	s.g = newGame(s.W().Seed)
	s.attachSound()
	if stateHook != nil {
		stateHook(s)
	}
}

// attachSound wires the current game's sound hook to the mixer (a no-op without
// audio). Called on mount and after each restart. Hits get a small random pitch
// for variety and a pan from the game (positional combat).
func (s *gameState) attachSound() {
	if s.snd == nil {
		return
	}
	s.g.sfx = func(id SoundID, pan float64) {
		smp := s.samples[id]
		if smp == nil {
			return
		}
		opts := sound.PlayOptions{Volume: 0.55, Pan: pan}
		if id == SndHit {
			opts.Pitch = 0.9 + s.rng.Float64()*0.35
		}
		s.snd.Play(smp, opts)
	}
}

func (s *gameState) Build(_ widget.Ctx) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{
			OnKey: func(k shell.Key) {
				if k.Kind == shell.KeyPress {
					s.key(k.Code)
				}
			},
			OnPress: func(p geom.Pt) { s.tap(p) },
		},
		Child: widget.Canvas{Clip: true, Draw: s.draw},
	}
}

func (s *gameState) key(c shell.KeyCode) {
	switch c {
	case shell.KeyLeft:
		s.act(-1, 0)
	case shell.KeyRight:
		s.act(1, 0)
	case shell.KeyUp:
		s.act(0, -1)
	case shell.KeyDown:
		s.act(0, 1)
	}
}

// tap moves one step toward the tapped cell (touch/mouse control).
func (s *gameState) tap(p geom.Pt) {
	if s.ts == 0 {
		return
	}
	cx := int((p.X + s.origin.X) / s.ts)
	cy := int((p.Y + s.origin.Y) / s.ts)
	s.act(sign(cx-s.g.player.X), sign(cy-s.g.player.Y))
}

func (s *gameState) act(dx, dy int) {
	if s.g.dead || s.g.won {
		s.restarts++
		s.g = newGame(s.W().Seed + s.restarts) // any input after death/win starts anew
		s.attachSound()
	} else {
		s.g.Move(dx, dy)
	}
	if s.music != nil {
		s.music.SetVolume(0.28 + 0.05*float64(s.g.depth-1)) // tenser as you descend
	}
	s.SetState(nil)
}

func (s *gameState) draw(c paint.Canvas, size geom.Size) {
	c.Clear(colBG)
	g := s.g
	ts := float32(32)
	s.ts = ts
	ox := float32(g.player.X)*ts - size.W/2 + ts/2
	oy := float32(g.player.Y)*ts - (size.H-90)/2 + ts/2 // leave room for the HUD
	s.origin = geom.Pt{X: ox, Y: oy}

	x0, y0 := int(ox/ts)-1, int(oy/ts)-1
	x1 := x0 + int(size.W/ts) + 3
	y1 := y0 + int(size.H/ts) + 3
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if !g.seenAt(x, y) {
				continue
			}
			s.blit(c, terrainTile(g.d.at(x, y)), x, y, false, s.light(x, y))
		}
	}
	// Torch halo: a translucent warm glow centered on the player.
	gs := ts * 7
	c.DrawSprite(s.atlas, paint.Sprite{Src: src(TGlow),
		Dst:   geom.RectXYWH(float32(g.player.X)*ts-ox+ts/2-gs/2, float32(g.player.Y)*ts-oy+ts/2-gs/2, gs, gs),
		Alpha: 0.45})

	for _, it := range g.items {
		if g.visibleAt(it.X, it.Y) {
			s.blit(c, it.Tile, it.X, it.Y, false, s.light(it.X, it.Y))
		}
	}
	for _, m := range g.monsters {
		if m.Alive && g.visibleAt(m.X, m.Y) {
			s.blit(c, m.Tile, m.X, m.Y, m.FlipX, s.light(m.X, m.Y))
		}
	}
	s.blit(c, g.player.Tile, g.player.X, g.player.Y, g.player.FlipX, paint.Color{R: 1, G: 1, B: 1, A: 1})

	s.drawHUD(c, size)
	switch {
	case g.dead:
		s.banner(c, size, "You died — press a key to descend anew")
	case g.won:
		s.banner(c, size, "You claimed the Amulet! Press a key to delve anew")
	}
}

func (s *gameState) cell(x, y int) geom.Rect {
	return geom.RectXYWH(float32(x)*s.ts-s.origin.X, float32(y)*s.ts-s.origin.Y, s.ts, s.ts)
}

func (s *gameState) blit(c paint.Canvas, id TileID, x, y int, flip bool, tint paint.Color) {
	c.DrawSprite(s.atlas, paint.Sprite{Src: src(id), Dst: s.cell(x, y), Nearest: true, FlipX: flip, Tint: tint})
}

// light returns the tint for a cell: a torchlight falloff when in sight (bright
// at the player, dimming with distance), or a cool dim for remembered cells.
func (s *gameState) light(x, y int) paint.Color {
	if !s.g.visibleAt(x, y) {
		return paint.Color{R: 0.30, G: 0.32, B: 0.40, A: 1} // explored memory
	}
	d := math.Hypot(float64(x-s.g.player.X), float64(y-s.g.player.Y))
	b := 1.0 - d/float64(fovRadius+1)*0.72
	if b < 0.32 {
		b = 0.32
	}
	return paint.Color{R: float32(b), G: float32(b * 0.99), B: float32(b * 0.94), A: 1} // warm torchlight
}

func terrainTile(cell Cell) TileID {
	switch cell {
	case CellWall:
		return TWall
	case CellStairs:
		return TStairs
	case CellDoor:
		return TDoor
	default:
		return TFloor
	}
}

func (s *gameState) drawHUD(c paint.Canvas, size geom.Size) {
	g := s.g
	h := float32(90)
	panel := geom.RectXYWH(0, size.H-h, size.W, h)
	c.FillRect(panel, colPanel)

	// HP bar.
	bx, by, bw := float32(16), size.H-h+16, float32(180)
	c.FillRRect(geom.RectXYWH(bx, by, bw, 14), 7, colHPbg)
	frac := float32(g.player.HP) / float32(g.player.MaxHP)
	if frac < 0 {
		frac = 0
	}
	c.FillRRect(geom.RectXYWH(bx, by, bw*frac, 14), 7, colHP)
	c.Text(fmt.Sprintf("HP %d/%d", g.player.HP, g.player.MaxHP), geom.Pt{X: bx + bw + 12, Y: by + 12}, 14, colInk)
	c.Text(fmt.Sprintf("Depth %d", g.depth), geom.Pt{X: bx + bw + 130, Y: by + 12}, 14, colInk)
	c.Text(fmt.Sprintf("Gold %d", g.gold), geom.Pt{X: bx + bw + 220, Y: by + 12}, 14, colCoin)
	c.Text(fmt.Sprintf("Amulet · depth %d", maxDepth), geom.Pt{X: size.W - 170, Y: by + 12}, 14, colDim)

	// Last few log lines.
	ly := size.H - h + 40
	for _, line := range g.log {
		c.Text(line, geom.Pt{X: bx, Y: ly}, 13, colDim)
		ly += 16
	}
}

func (s *gameState) banner(c paint.Canvas, size geom.Size, msg string) {
	c.FillRect(geom.RectXYWH(0, size.H*0.4, size.W, 60), colBanner)
	c.TextIn("bold", msg, geom.Pt{X: size.W*0.5 - s.ctx.Painter().MeasureWidth(msg, 22)/2, Y: size.H*0.4 + 38}, 22, colInk)
}
