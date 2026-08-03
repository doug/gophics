// Package hnmobile is the gomobile-bind surface for the HN app: a flat,
// bind-friendly API (ints, floats, strings, []byte) over shell/mobile.
//
// Build the Android library (requires the Android NDK):
//
//	go install golang.org/x/mobile/cmd/gomobile@latest
//	gomobile init
//	gomobile bind -target=android -androidapi 24 -o examples/hn/android/app/libs/hnmobile.aar ./examples/hn/mobile
//
// then open examples/hn/android with Gradle (see its README).
package hnmobile

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	gpucheck "github.com/doug/gossamer/examples/gpucheck/ui"
	hn "github.com/doug/gossamer/examples/hn/ui"
	"github.com/doug/gossamer/shell/mobile"
)

var bridge *mobile.Bridge

// Start builds the HN app; call once from the host before any other call.
func Start() string {
	h, err := app.NewHandler(hn.Root(), app.Config{
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
		Background:   hn.Background(),
	})
	if err != nil {
		return err.Error()
	}
	bridge = mobile.NewBridge(h)
	return ""
}

// StartVerify builds the GPU-verification scene instead of HN — call this from
// the host (in place of Start) to run the mobile GPU bring-up check. See
// docs/mobile-gpu-bringup.md.
func StartVerify() string {
	h, err := app.NewHandler(gpucheck.Root(), app.Config{
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
		Background:   gpucheck.Background(),
	})
	if err != nil {
		return err.Error()
	}
	bridge = mobile.NewBridge(h)
	return ""
}

// Resize sets the surface size in physical pixels and the density scale.
func Resize(widthPx, heightPx int, scale float64) {
	bridge.Resize(widthPx, heightPx, float32(scale))
}

// NeedsFrame reports whether the UI wants a repaint (poll each vsync).
func NeedsFrame() bool { return bridge.NeedsFrame() }

// SetSurface hands over the host's native render surface so rendering runs on
// the GPU (iOS: displayHandle 0, windowHandle = CAMetalLayer*; Android:
// displayHandle 0, windowHandle = ANativeWindow*). Call after the surface is
// created and on every resize/rotation.
func SetSurface(displayHandle, windowHandle int64, widthPx, heightPx int, scale float64) {
	bridge.SetSurface(displayHandle, windowHandle, widthPx, heightPx, float32(scale))
}

// ClearSurface releases the GPU surface (call when the host surface is destroyed).
func ClearSurface() { bridge.ClearSurface() }

// RenderFrame renders one frame on the GPU to the surface set by SetSurface;
// call each vsync while NeedsFrame is true.
func RenderFrame(dtSeconds float64) { bridge.RenderFrame(dtSeconds) }

// Snapshot renders one frame offscreen and returns RGBA8888 pixels
// (FrameWidth×FrameHeight) — for screenshots/tests, not the live loop.
func Snapshot(dtSeconds float64) []byte { return bridge.Snapshot(dtSeconds) }

// FrameWidth / FrameHeight are the pixel dimensions of the last Snapshot.
func FrameWidth() int  { return bridge.FrameWidth() }
func FrameHeight() int { return bridge.FrameHeight() }

// Touch forwards a touch event: phase 0 down, 1 move, 2 up, 3 cancel.
func Touch(phase int, xPx, yPx float64) {
	bridge.Touch(phase, float32(xPx), float32(yPx))
}

// Text forwards committed keyboard text.
func Text(s string) { bridge.Text(s) }

// Key forwards a key by shell.KeyCode value (1=Enter, 2=Backspace, ...).
func Key(code int, pressed bool) { bridge.Key(code, pressed) }

// SetDarkMode forwards the host color scheme.
func SetDarkMode(dark bool) { bridge.SetDarkMode(dark) }

// TakeOpenedURL returns a URL the UI asked to open ("" if none); the host
// launches it in the browser.
func TakeOpenedURL() string { return bridge.TakeOpenedURL() }

// Focused forwards app focus/visibility.
func Focused(f bool) { bridge.Focused(f) }

// SetInsets forwards safe-area insets in physical pixels.
func SetInsets(top, right, bottom, left float64) {
	bridge.SetInsets(float32(top), float32(right), float32(bottom), float32(left))
}

// TextInputActive reports whether the UI wants the on-screen keyboard.
func TextInputActive() bool { return bridge.TextInputActive() }

// Composition forwards IME preedit (kind: 0 start, 1 update, 2 end).
func Composition(kind int, preedit string, cursor int, committed string) {
	bridge.Composition(kind, preedit, cursor, committed)
}

// Accessibility: the host refreshes then reads the flat node tree and
// activates by ID. Rects are physical pixels.
func A11yRefresh() int             { return bridge.A11yRefresh() }
func A11yID(i int) int             { return bridge.A11yID(i) }
func A11yParent(i int) int         { return bridge.A11yParent(i) }
func A11yRole(i int) string        { return bridge.A11yRole(i) }
func A11yLabel(i int) string       { return bridge.A11yLabel(i) }
func A11yValue(i int) string       { return bridge.A11yValue(i) }
func A11yHint(i int) string        { return bridge.A11yHint(i) }
func A11yX(i int) int              { return bridge.A11yX(i) }
func A11yY(i int) int              { return bridge.A11yY(i) }
func A11yW(i int) int              { return bridge.A11yW(i) }
func A11yH(i int) int              { return bridge.A11yH(i) }
func A11yTappable(i int) bool      { return bridge.A11yTappable(i) }
func A11yChildCount(i int) int     { return bridge.A11yChildCount(i) }
func A11yChild(i int, j int) int   { return bridge.A11yChild(i, j) }
func A11yActivate(id int)          { bridge.A11yActivate(id) }
func A11yHitTest(xPx, yPx int) int { return bridge.A11yHitTest(xPx, yPx) }
