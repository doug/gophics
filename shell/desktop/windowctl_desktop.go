//go:build !js

// Desktop implementation of the shell window-control capability
// (shell/windowctl.go). It is a thin pass-through to the gogpu App, which owns
// the platform window. SetTitle, SetFullscreen/Fullscreen, and Size all map to
// existing gogpu App methods, so no new native code is needed here.
//
// Build-verified on darwin. A desktop window can't be driven headlessly in this
// environment, so that the title/fullscreen actually change on screen is not
// runtime-confirmed — the gogpu App methods themselves are exercised by
// gogpu's own tests (see internal/gfx/gogpu/window_chrome_test.go).

package desktop

import "github.com/doug/gophics/shell"

// WindowControl satisfies shell.WindowControlWindow for the desktop shell.
func (w *window) WindowControl() shell.WindowControl { return desktopWindowControl{w: w} }

type desktopWindowControl struct{ w *window }

// SetTitle forwards to gogpu App.SetTitle (already used by window.SetTitle).
func (c desktopWindowControl) SetTitle(title string) { c.w.app.SetTitle(title) }

// SetFullscreen forwards to gogpu App.SetFullscreen (native toggleFullScreen on
// macOS, borderless on Windows, EWMH/xdg on Linux).
func (c desktopWindowControl) SetFullscreen(on bool) { c.w.app.SetFullscreen(on) }

// Fullscreen forwards to gogpu App.IsFullscreen.
func (c desktopWindowControl) Fullscreen() bool { return c.w.app.IsFullscreen() }

// Size returns the logical window size from gogpu App.Size.
func (c desktopWindowControl) Size() (w, h float32) {
	lw, lh := c.w.app.Size()
	return float32(lw), float32(lh)
}

// Note: there is intentionally no SetSize. gogpu's App exposes no runtime
// window resize — only SetMinSize/SetMaxSize constraints, and darwin's platform
// Window.SetSize is not reachable through the App/PlatformWindow interface — so
// a SetSize here could not be honestly backed.
// TODO(desktop): if gogpu grows App.SetSize (wrapping platWindow), add it to
// shell.WindowControl and implement it here.
