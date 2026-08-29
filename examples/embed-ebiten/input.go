package main

import (
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// Input translation: the bulk of what an embedding host actually writes.
//
// Ebiten reports state, gophics wants events, so every tick this diffs one
// against the other. Pointer position is offset into the overlay's coordinate
// space — the panel is on the right of the window, and gophics knows nothing
// about where the host put it.
//
// Doing this honestly is the point of the example. A host that only forwarded
// clicks would look like it worked and then have no text input, no scrolling
// and no keyboard focus, which is exactly the shape of gap a "seam" is supposed
// to reveal before someone hits it in their own app.

// keyMap translates the keys gophics names. Printable characters do not appear
// here: they arrive as shell.Text, the same split the platform shells make.
var keyMap = map[ebiten.Key]shell.KeyCode{
	ebiten.KeyEnter:       shell.KeyEnter,
	ebiten.KeyNumpadEnter: shell.KeyEnter,
	ebiten.KeyBackspace:   shell.KeyBackspace,
	ebiten.KeyDelete:      shell.KeyDelete,
	ebiten.KeyEscape:      shell.KeyEscape,
	ebiten.KeyTab:         shell.KeyTab,
	ebiten.KeyLeft:        shell.KeyLeft,
	ebiten.KeyRight:       shell.KeyRight,
	ebiten.KeyUp:          shell.KeyUp,
	ebiten.KeyDown:        shell.KeyDown,
	ebiten.KeyHome:        shell.KeyHome,
	ebiten.KeyEnd:         shell.KeyEnd,
	ebiten.KeyA:           shell.KeyA,
	ebiten.KeyC:           shell.KeyC,
	ebiten.KeyV:           shell.KeyV,
	ebiten.KeyX:           shell.KeyX,
}

func (g *game) pumpInput() {
	if g.frame == nil {
		return // no frame has been built yet, so there is nothing to send to
	}

	// Pointer. Positions are window-relative; the overlay starts at
	// winW-overlayW, so shift into its space. Events outside the panel are
	// still delivered: gophics hit-tests them and finds nothing, which is what
	// lets a click on the game pass through cleanly.
	x, y := ebiten.CursorPosition()
	pos := geom.Pt{X: float32(x - (winW - overlayW)), Y: float32(y)}
	if x != g.lastX || y != g.lastY {
		g.h.Event(g, shell.Pointer{Kind: shell.PointerMove, Pos: pos})
		g.lastX, g.lastY = x, y
	}
	down := ebiten.IsMouseButtonPressed(ebiten.MouseButtonLeft)
	if down != g.lastDown {
		kind := shell.PointerUp
		if down {
			kind = shell.PointerDown
		}
		g.h.Event(g, shell.Pointer{Kind: kind, Pos: pos})
		g.lastDown = down
	}
	if _, dy := ebiten.Wheel(); dy != 0 {
		g.h.Event(g, shell.Pointer{
			Kind:   shell.PointerScroll,
			Pos:    pos,
			Scroll: geom.Pt{Y: float32(dy) * 16}, // wheel notches to logical px
		})
	}

	// Keys, press and release, so a held modifier reads correctly.
	mods := currentMods()
	for k, code := range keyMap {
		if inpututil.IsKeyJustPressed(k) {
			g.h.Event(g, shell.Key{Kind: shell.KeyPress, Code: code, Mods: mods})
		}
		if inpututil.IsKeyJustReleased(k) {
			g.h.Event(g, shell.Key{Kind: shell.KeyRelease, Code: code, Mods: mods})
		}
	}

	// Committed text. Ebiten hands back the runes typed this tick, already past
	// the platform IME, which is exactly shell.Text.
	if runes := ebiten.AppendInputChars(nil); len(runes) > 0 {
		g.h.Event(g, shell.Text{S: string(runes)})
	}
}

func currentMods() shell.Mods {
	var m shell.Mods
	if ebiten.IsKeyPressed(ebiten.KeyShift) {
		m |= shell.ModShift
	}
	if ebiten.IsKeyPressed(ebiten.KeyControl) {
		m |= shell.ModCtrl
	}
	if ebiten.IsKeyPressed(ebiten.KeyAlt) {
		m |= shell.ModAlt
	}
	if ebiten.IsKeyPressed(ebiten.KeyMeta) {
		m |= shell.ModSuper
	}
	return m
}
