//go:build js && wasm

package web

import (
	"github.com/doug/gophics/shell"
)

// Cursor shapes the pointer by setting the canvas's CSS cursor. The app draws
// its own text and controls, so the browser has no idea what is under the
// pointer; this is the only way a reader gets an I-beam over a paragraph.
func (w *window) Cursor() shell.Cursor { return webCursor{w} }

type webCursor struct{ w *window }

func (c webCursor) Set(s shell.CursorShape) {
	name := cursorCSS(s)
	if c.w.cursor == name {
		return // the runner sets this on every hover change
	}
	c.w.cursor = name
	c.w.canvas.Get("style").Set("cursor", name)
}

// cursorCSS maps a shape to its CSS keyword. Every one of them is in CSS 2.1
// except not-allowed, which is CSS 3 and everywhere by now.
func cursorCSS(s shell.CursorShape) string {
	switch s {
	case shell.CursorText:
		return "text"
	case shell.CursorPointer:
		return "pointer"
	case shell.CursorCrosshair:
		return "crosshair"
	case shell.CursorMove:
		return "move"
	case shell.CursorNotAllowed:
		return "not-allowed"
	case shell.CursorWait:
		return "wait"
	}
	return "default"
}
