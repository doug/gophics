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

	fx  effects
	tkr fxTicker

	// hpGhost trails the real HP fraction so a hit leaves a visible wound on
	// the bar for a moment instead of just being a shorter bar.
	hpGhost float32
}

// effects is everything that is purely presentational: where entities are
// drawn as opposed to where they are, and the short-lived flourishes that make
// a turn feel like it happened. The game logic never reads any of it.
type effects struct {
	clock  float64             // seconds since mount, for the torch flicker
	pos    map[*Entity]geom.Pt // render position, easing toward the real cell
	flash  map[*Entity]float32 // white hit flash, 1 → 0
	lunge  map[*Entity]geom.Pt // attack shove, decaying to zero
	floats []floater
	shake  float32
}

// floater is a damage number rising off a hit.
type floater struct {
	x, y float32 // world pixels at spawn
	text string
	col  paint.Color
	age  float32
}

// fxTicker advances the effects. It reports "still running" only while there
// is something to animate, so a game sitting still costs no frames.
type fxTicker struct{ s *gameState }

func (t *fxTicker) Tick(dt float64) bool {
	s := t.s
	s.fx.clock += dt
	busy := s.fx.advance(float32(dt), s.g, s.ts)
	if want := clamp01(float32(s.g.player.HP) / float32(s.g.player.MaxHP)); s.hpGhost > want {
		s.hpGhost -= float32(dt) * 0.6
		if s.hpGhost < want {
			s.hpGhost = want
		}
		busy = true
	} else {
		s.hpGhost = want
	}
	// The torch flickers forever, but only repaint for it while something is
	// on screen to see; a still frame does not need 60 fps of flicker.
	if busy {
		s.SetState(nil)
	}
	return busy
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
	colDamage = paint.RGB(1.00, 0.86, 0.55)
	colXP     = paint.RGB(0.44, 0.72, 0.92)
)

func (s *gameState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	s.atlas = buildAtlas()
	s.fx.pos = map[*Entity]geom.Pt{}
	s.fx.flash = map[*Entity]float32{}
	s.fx.lunge = map[*Entity]geom.Pt{}
	s.tkr.s = s
	ctx.AddTicker(&s.tkr)
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
	s.g.onHit = s.onHit
	s.attachSound()
	if stateHook != nil {
		stateHook(s)
	}
}

// Dispose stops the effects ticker so it cannot outlive the widget.
func (s *gameState) Dispose() { s.ctx.RemoveTicker(&s.tkr) }

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
	// Restart is on any key once the run is over, so the switch below only has
	// to describe a live game.
	if s.g.dead || s.g.won {
		s.act(0, 0)
		return
	}
	switch c {
	case shell.KeyLeft, shell.KeyA:
		s.step(-1, 0)
	case shell.KeyRight, shell.KeyD:
		s.step(1, 0)
	case shell.KeyUp, shell.KeyW:
		s.step(0, -1)
	case shell.KeyDown, shell.KeyS:
		s.step(0, 1)
	case shell.KeyQ:
		s.g.Quaff()
		s.after()
	case shell.KeySpace:
		s.g.Wait()
		s.after()
	}
}

// step is a movement action.
func (s *gameState) step(dx, dy int) {
	s.g.Move(dx, dy)
	s.after()
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
		s.g.onHit = s.onHit
		s.fx.pos = map[*Entity]geom.Pt{}
		s.fx.flash = map[*Entity]float32{}
		s.fx.lunge = map[*Entity]geom.Pt{}
		s.fx.floats = nil
		s.attachSound()
	} else {
		s.g.Move(dx, dy)
	}
	s.after()
}

// after runs the housekeeping every action shares.
func (s *gameState) after() {
	if s.music != nil {
		s.music.SetVolume(0.28 + 0.05*float64(s.g.depth-1)) // tenser as you descend
	}
	s.SetState(nil)
}

func (s *gameState) draw(c paint.Canvas, size geom.Size) {
	c.Clear(colBG)
	g := s.g
	ts := tileSize(size)
	s.ts = ts
	// Follow the player's eased position, not the cell, so the camera glides
	// with them instead of jumping a whole tile ahead of the sprite.
	pp := s.renderPos(g.player)
	ox := pp.X - size.W/2 + ts/2
	oy := pp.Y - (size.H-hudHeight)/2 + ts/2 // leave room for the HUD
	if s.fx.shake > 0 {
		k := s.fx.shake * s.fx.shake * ts * 0.16
		ox += k * float32(math.Sin(s.fx.clock*61))
		oy += k * float32(math.Sin(s.fx.clock*47))
	}
	s.origin = geom.Pt{X: ox, Y: oy}

	x0, y0 := int(ox/ts)-1, int(oy/ts)-1
	x1 := x0 + int(size.W/ts) + 3
	y1 := y0 + int(size.H/ts) + 3
	for y := y0; y <= y1; y++ {
		for x := x0; x <= x1; x++ {
			if !g.seenAt(x, y) {
				continue
			}
			s.blit(c, s.terrainTile(x, y), x, y, false, s.light(x, y))
		}
	}
	// Torch halo: a translucent warm glow centered on the player, breathing.
	fl := s.torch()
	gs := ts * 7 * fl
	c.DrawSprite(s.atlas, paint.Sprite{Src: src(TGlow),
		Dst:   geom.RectXYWH(pp.X-ox+ts/2-gs/2, pp.Y-oy+ts/2-gs/2, gs, gs),
		Alpha: 0.42 * fl})

	for _, it := range g.items {
		if g.visibleAt(it.X, it.Y) {
			s.blit(c, it.Tile, it.X, it.Y, false, s.light(it.X, it.Y))
		}
	}
	for _, m := range g.monsters {
		if m.Alive && g.visibleAt(m.X, m.Y) {
			s.entity(c, m, s.light(m.X, m.Y))
		}
	}
	s.entity(c, g.player, paint.Color{R: 1, G: 1, B: 1, A: 1})
	s.drawFloats(c)

	s.vignette(c, size)
	s.drawHUD(c, size)
	switch {
	case g.dead:
		s.summary(c, size, "You died", false)
	case g.won:
		s.summary(c, size, "You claimed the Amulet", true)
	}
}

// entity draws one creature at its eased position: shadow, sprite, and the
// white flash of a fresh wound.
func (s *gameState) entity(c paint.Canvas, e *Entity, tint paint.Color) {
	p := s.renderPos(e)
	dst := geom.RectXYWH(p.X-s.origin.X, p.Y-s.origin.Y, s.ts, s.ts)
	c.DrawSprite(s.atlas, paint.Sprite{Src: src(TShadow), Dst: dst, Nearest: true})
	c.DrawSprite(s.atlas, paint.Sprite{Src: src(e.Tile), Dst: dst, Nearest: true, FlipX: e.FlipX, Tint: tint})
	if f := s.fx.flash[e]; f > 0 {
		c.DrawSprite(s.atlas, paint.Sprite{Src: src(e.Tile), Dst: dst, Nearest: true, FlipX: e.FlipX,
			Tint: paint.Color{R: 1, G: 1, B: 1, A: 1}, Alpha: f})
	}
}

func (s *gameState) cell(x, y int) geom.Rect {
	return geom.RectXYWH(float32(x)*s.ts-s.origin.X, float32(y)*s.ts-s.origin.Y, s.ts, s.ts)
}

func (s *gameState) blit(c paint.Canvas, id TileID, x, y int, flip bool, tint paint.Color) {
	c.DrawSprite(s.atlas, paint.Sprite{Src: src(id), Dst: s.cell(x, y), Nearest: true, FlipX: flip, Tint: tint})
}

// Torchlight runs from a warm core to a cold edge, and remembered cells are
// colder still. The hue shift is doing most of the work: a falloff that only
// darkens reads as haze over the room, where warm-to-cool reads as a flame in
// the dark, and it also tells you at a glance which parts of the map you are
// looking at versus only recalling.
var (
	lightCore = paint.RGB(1.00, 0.93, 0.76) // at the torch
	lightEdge = paint.RGB(0.26, 0.25, 0.36) // at the limit of sight
	lightMem  = paint.RGB(0.20, 0.22, 0.34) // explored, out of sight
)

// light returns the tint for a cell.
func (s *gameState) light(x, y int) paint.Color {
	if !s.g.visibleAt(x, y) {
		return lightMem
	}
	d := math.Hypot(float64(x-s.g.player.X), float64(y-s.g.player.Y)) / float64(fovRadius+1)
	if d > 1 {
		d = 1
	}
	// Squared falloff, so the bright core is generous and the shoulder is
	// short — a linear ramp washes the whole room out evenly.
	t := float32(d * d)
	return paint.Color{
		R: lightCore.R + (lightEdge.R-lightCore.R)*t,
		G: lightCore.G + (lightEdge.G-lightCore.G)*t,
		B: lightCore.B + (lightEdge.B-lightCore.B)*t,
		A: 1,
	}
}

// terrainTile picks the tile for a cell. A wall with a walkable cell below it
// is a face the torch can light; every other wall is the top of the rock, and
// painting the two differently is what gives the grid depth.
func (s *gameState) terrainTile(x, y int) TileID {
	switch s.g.d.at(x, y) {
	case CellWall:
		if s.g.d.walkable(x, y+1) {
			return TWall
		}
		return TWallTop
	case CellStairs:
		return TStairs
	case CellDoor:
		return TDoor
	default:
		// Vary the floor by position so a large room is not one flat texture.
		switch (x*7 ^ y*13) % 3 {
		case 0:
			return TFloor2
		case 1:
			return TFloor3
		}
		return TFloor
	}
}

// tileSize scales tiles to the viewport so the lit radius roughly fills the
// frame. It was fixed at 32px, which on a large window left the visible island
// marooned in black — the field of view is only a few tiles wide, so the tile
// has to grow with the window rather than the count of tiles on screen. The
// scale is a whole number because these are nearest-neighbour pixel blits.
func tileSize(size geom.Size) float32 {
	const targetAcross = 19
	sc := math.Round(float64(size.W) / (targetAcross * tile))
	if sc < 2 {
		sc = 2
	}
	if sc > 6 {
		sc = 6
	}
	return float32(tile) * float32(sc)
}

// vignette darkens the frame edges, so the eye settles on the torch instead of
// the corners and the dungeon feels enclosed.
func (s *gameState) vignette(c paint.Canvas, size geom.Size) {
	const clear = 0.0
	edge := paint.Color{A: 0.55}
	none := paint.Color{}
	w := size.W * 0.20
	h := size.H * 0.20
	c.FillRRectGradient(geom.RectXYWH(0, 0, w, size.H), 0, edge, none, true)
	c.FillRRectGradient(geom.RectXYWH(size.W-w, 0, w, size.H), 0, none, edge, true)
	c.FillRRectGradient(geom.RectXYWH(0, 0, size.W, h), 0, edge, none, false)
	c.FillRRectGradient(geom.RectXYWH(0, size.H-h, size.W, h), 0, none, edge, false)
	_ = clear
}

// hudHeight is the bottom panel's height, reserved from the world viewport.
const hudHeight float32 = 90

func (s *gameState) drawHUD(c paint.Canvas, size geom.Size) {
	g := s.g
	h := hudHeight
	top := size.H - h
	c.FillRect(geom.RectXYWH(0, top, size.W, h), colPanel)
	// A lit rule along the top edge, so the panel reads as a frame around the
	// dungeon rather than a slab dropped on top of it.
	c.FillRect(geom.RectXYWH(0, top, size.W, 1), paint.Color{R: 0.55, G: 0.48, B: 0.38, A: 0.5})

	const pad = 18
	x, y := float32(pad), top+20

	// HP, with a trailing ghost so a hit reads as damage taken rather than a
	// bar that was always that length.
	const bw, bh = 190, 13
	frac := clamp01(float32(g.player.HP) / float32(g.player.MaxHP))
	c.FillRRect(geom.RectXYWH(x, y, bw, bh), bh/2, colHPbg)
	if s.hpGhost > frac {
		c.FillRRect(geom.RectXYWH(x, y, bw*s.hpGhost, bh), bh/2, paint.Color{R: 0.9, G: 0.5, B: 0.5, A: 0.35})
	}
	c.FillRRect(geom.RectXYWH(x, y, bw*frac, bh), bh/2, colHP)
	c.TextIn("bold", fmt.Sprintf("%d/%d", g.player.HP, g.player.MaxHP),
		geom.Pt{X: x + 8, Y: y + bh - 2}, 11, colInk)

	// Experience toward the next level, directly under HP: the two bars are
	// the run in one glance — how close to dying, how close to stronger.
	xy := y + bh + 6
	xf := clamp01(float32(g.xp) / float32(xpToLevel(g.level)))
	c.FillRRect(geom.RectXYWH(x, xy, bw, 5), 2.5, colHPbg)
	c.FillRRect(geom.RectXYWH(x, xy, bw*xf, 5), 2.5, colXP)

	// Stats.
	sx := x + bw + 26
	stat := func(label string, val string, col paint.Color) {
		c.TextIn("", label, geom.Pt{X: sx, Y: y + 2}, 11, colDim)
		c.TextIn("bold", val, geom.Pt{X: sx, Y: y + 18}, 15, col)
		sx += 78
	}
	stat("LEVEL", fmt.Sprintf("%d", g.level), colInk)
	stat("DEPTH", fmt.Sprintf("%d", g.depth), colInk)
	stat("GOLD", fmt.Sprintf("%d", g.gold), colCoin)
	stat("POTIONS", fmt.Sprintf("%d", g.potions), potionColor(g.potions))

	// Controls, so the game explains itself without a manual.
	c.TextIn("", "move ←↑↓→ / wasd    Q quaff    space wait",
		geom.Pt{X: size.W - 300, Y: y + 2}, 11, colDim)
	c.TextIn("", fmt.Sprintf("Amulet · depth %d", maxDepth),
		geom.Pt{X: size.W - 300, Y: y + 20}, 12, colDim)

	// The last few log lines, newest brightest, oldest fading out.
	// Two lines, sized and placed to sit inside the panel — three at 15px
	// pushed the oldest off the bottom edge.
	ly := top + 58
	n := len(g.log)
	from := max(0, n-2)
	for i, line := range g.log[from:] {
		col := colDim
		if i < len(g.log[from:])-1 {
			col.A = 0.5 // older line recedes
		}
		c.TextIn("", line, geom.Pt{X: pad, Y: ly + float32(i)*15}, 12, col)
	}
}

// potionColor greys the potion count out at zero, so "none left" is legible
// at a glance in the middle of a fight.
func potionColor(n int) paint.Color {
	if n == 0 {
		return colDim
	}
	return paint.RGB(0.72, 0.86, 0.62)
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// summary is the end-of-run card: what this run amounted to, so a death reads
// as a result rather than an interruption. A roguelike run you cannot look
// back on is just a session that stopped.
func (s *gameState) summary(c paint.Canvas, size geom.Size, title string, win bool) {
	g := s.g
	w, h := float32(360), float32(210)
	x, y := (size.W-w)/2, (size.H-hudHeight-h)/2

	c.FillRect(geom.RectXYWH(0, 0, size.W, size.H-hudHeight), colBanner)
	c.FillRRect(geom.RectXYWH(x, y, w, h), 14, paint.Color{R: 0.10, G: 0.10, B: 0.14, A: 0.97})
	accent := colHP
	if win {
		accent = colCoin
	}
	c.FillRRect(geom.RectXYWH(x, y, w, 4), 2, accent)

	tw := s.ctx.Painter().MeasureWidthIn("bold", title, 22)
	c.TextIn("bold", title, geom.Pt{X: x + (w-tw)/2, Y: y + 46}, 22, colInk)

	rows := [][2]string{
		{"Depth reached", fmt.Sprintf("%d of %d", g.depth, maxDepth)},
		{"Level", fmt.Sprintf("%d", g.level)},
		{"Monsters slain", fmt.Sprintf("%d", g.kills)},
		{"Gold", fmt.Sprintf("%d", g.gold)},
		{"Turns", fmt.Sprintf("%d", g.turns)},
	}
	ry := y + 78
	for _, r := range rows {
		c.TextIn("", r[0], geom.Pt{X: x + 28, Y: ry}, 13, colDim)
		vw := s.ctx.Painter().MeasureWidthIn("bold", r[1], 13)
		c.TextIn("bold", r[1], geom.Pt{X: x + w - 28 - vw, Y: ry}, 13, colInk)
		ry += 22
	}

	const hint = "press any key to delve again"
	hw := s.ctx.Painter().MeasureWidthIn("", hint, 12)
	c.TextIn("", hint, geom.Pt{X: x + (w-hw)/2, Y: y + h - 20}, 12, colDim)
}

// --- presentation effects -------------------------------------------------
//
// Turn-based does not have to mean snapping. Entities ease toward their new
// cell over a few frames, hits shove and flash, damage floats off, and the
// screen kicks when you are the one taking it. None of this changes a rule;
// it changes whether a turn reads as an event or as a diff.

const (
	moveEase   = 14.0 // higher settles faster
	flashDecay = 6.0
	lungeDecay = 12.0
	floatLife  = 0.9
	shakeDecay = 7.0
)

// advance steps every effect. It reports whether anything is still moving.
func (fx *effects) advance(dt float32, g *Game, ts float32) bool {
	busy := false

	// Drop entries for entities that are gone — the dead, and everything left
	// behind on previous levels. These maps are keyed by pointer, so without
	// this a long descent accumulates one entry per monster ever spawned.
	live := make(map[*Entity]bool, len(g.monsters)+1)
	for _, e := range g.all() {
		live[e] = true
	}
	for e := range fx.pos {
		if !live[e] {
			delete(fx.pos, e)
			delete(fx.flash, e)
			delete(fx.lunge, e)
		}
	}

	// Ease render positions toward the true cell.
	for _, e := range g.all() {
		want := geom.Pt{X: float32(e.X) * ts, Y: float32(e.Y) * ts}
		cur, ok := fx.pos[e]
		if !ok || ts == 0 {
			fx.pos[e] = want
			continue
		}
		d := geom.Pt{X: want.X - cur.X, Y: want.Y - cur.Y}
		if d.X*d.X+d.Y*d.Y < 0.25 {
			fx.pos[e] = want
			continue
		}
		k := min(1, dt*moveEase)
		fx.pos[e] = geom.Pt{X: cur.X + d.X*k, Y: cur.Y + d.Y*k}
		busy = true
	}

	for e, v := range fx.flash {
		v -= dt * flashDecay
		if v <= 0 {
			delete(fx.flash, e)
			continue
		}
		fx.flash[e] = v
		busy = true
	}
	for e, v := range fx.lunge {
		k := 1 - min(1, dt*lungeDecay)
		v = geom.Pt{X: v.X * k, Y: v.Y * k}
		if v.X*v.X+v.Y*v.Y < 0.2 {
			delete(fx.lunge, e)
			continue
		}
		fx.lunge[e] = v
		busy = true
	}
	if n := fx.floats[:0]; true {
		for _, f := range fx.floats {
			f.age += dt
			if f.age < floatLife {
				n = append(n, f)
				busy = true
			}
		}
		fx.floats = n
	}
	if fx.shake > 0 {
		fx.shake -= dt * shakeDecay
		if fx.shake < 0 {
			fx.shake = 0
		}
		busy = true
	}
	return busy
}

// renderPos is where an entity should be drawn: its eased position plus any
// attack lunge, falling back to the true cell before the first frame.
func (s *gameState) renderPos(e *Entity) geom.Pt {
	p, ok := s.fx.pos[e]
	if !ok {
		p = geom.Pt{X: float32(e.X) * s.ts, Y: float32(e.Y) * s.ts}
	}
	if l, ok := s.fx.lunge[e]; ok {
		p = geom.Pt{X: p.X + l.X, Y: p.Y + l.Y}
	}
	return p
}

// onHit is the game's report that a blow landed, turned into things to look at.
func (s *gameState) onHit(attacker, target *Entity, dmg int) {
	s.fx.flash[target] = 1
	if s.ts > 0 {
		dx := float32(target.X-attacker.X) * s.ts * 0.28
		dy := float32(target.Y-attacker.Y) * s.ts * 0.28
		s.fx.lunge[attacker] = geom.Pt{X: dx, Y: dy}
	}
	col := colDamage
	if target == s.g.player {
		col = colHP
		s.fx.shake = 1
	}
	s.fx.floats = append(s.fx.floats, floater{
		x: float32(target.X)*s.ts + s.ts/2, y: float32(target.Y) * s.ts,
		text: fmt.Sprintf("-%d", dmg), col: col,
	})
	s.SetState(nil)
}

// drawFloats paints the damage numbers rising off recent hits.
func (s *gameState) drawFloats(c paint.Canvas) {
	for _, f := range s.fx.floats {
		t := f.age / floatLife
		col := f.col
		col.A = 1 - t*t
		y := f.y - t*s.ts*0.9
		w := s.ctx.Painter().MeasureWidthIn("bold", f.text, 15)
		c.TextIn("bold", f.text, geom.Pt{X: f.x - s.origin.X - w/2, Y: y - s.origin.Y}, 15, col)
	}
}

// torch returns the current flicker multiplier — two offset sines, so it never
// settles into an obvious loop.
func (s *gameState) torch() float32 {
	t := s.fx.clock
	return float32(1 + 0.055*math.Sin(t*7.3) + 0.035*math.Sin(t*2.9+1.7))
}
