//go:build !js

package desktop

import (
	"github.com/doug/gophics/internal/gfx/gpucontext"
	"github.com/doug/gophics/shell"
)

// Cursor shapes the pointer. The app draws its own text and controls, so the
// windowing layer cannot know what is under the pointer; a widget declares the
// shape for its region and the runner applies it.
func (w *window) Cursor() shell.Cursor { return desktopCursor{w} }

type desktopCursor struct{ w *window }

// Set goes through runOnMain for the reason SetTitle does: a window mutation
// off the main thread aborts the process on macOS, and this is called from the
// render thread's hover handling.
func (c desktopCursor) Set(s shell.CursorShape) {
	if c.w.cursor == s {
		return // the runner sets this on every hover change
	}
	c.w.cursor = s
	shape := cursorShape(s)
	c.w.runOnMain(func() { c.w.app.SetCursor(shape) })
}

func cursorShape(s shell.CursorShape) gpucontext.CursorShape {
	switch s {
	case shell.CursorText:
		return gpucontext.CursorText
	case shell.CursorPointer:
		return gpucontext.CursorPointer
	case shell.CursorCrosshair:
		return gpucontext.CursorCrosshair
	case shell.CursorMove:
		return gpucontext.CursorMove
	case shell.CursorNotAllowed:
		return gpucontext.CursorNotAllowed
	case shell.CursorWait:
		return gpucontext.CursorWait
	}
	return gpucontext.CursorDefault
}
