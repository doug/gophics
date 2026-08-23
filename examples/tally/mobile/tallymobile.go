// Package tallymobile is the gomobile-bind surface for Tally: a flat,
// bind-friendly API (ints, floats, strings, []byte) over shell/mobile, mirroring
// the shape the HN example proved on device.
//
// The whole UI is the same Go code the desktop runs; the host owns only the
// layer, the display link, touch and the keyboard.
//
// Build the frameworks:
//
//	gomobile bind -target=ios,iossimulator -o ios/Tallymobile.xcframework ./mobile
//	gomobile bind -target=android -androidapi 24 -o android/app/libs/tallymobile.aar ./mobile
package tallymobile

import (
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell/mobile"
	"github.com/doug/gophics/theme"
)

var bridge *mobile.Bridge

// Start builds the app and must be called once from the host before anything
// else. It returns "" on success or the error text.
//
// The mono family is registered because Tally's figures are tabular: money only
// lines up in columns if the digits are the same width.
func Start() string {
	h, err := app.NewHandler(newRoot(), app.Config{
		Title:      "Tally",
		AppID:      "com.gophics.tally",
		Size:       geom.Size{W: 390, H: 844},
		Background: theme.Light().Bg,
		Font:       goregular.TTF,
		FontFamilies: map[string][]byte{
			theme.FontBold: gobold.TTF,
			"mono":         gomono.TTF,
		},
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

// SetSurface hands over the host's native render surface so rendering runs on the
// GPU (iOS: displayHandle 0, windowHandle = CAMetalLayer*; Android: displayHandle
// 0, windowHandle = ANativeWindow*). Call after the surface exists and on every
// resize or rotation.
func SetSurface(displayHandle, windowHandle int64, widthPx, heightPx int, scale float64) {
	bridge.SetSurface(displayHandle, windowHandle, widthPx, heightPx, float32(scale))
}

// ClearSurface releases the GPU surface (call when the host surface is destroyed).
func ClearSurface() { bridge.ClearSurface() }

// GpuActive reports whether GPU rendering is live. When false (the iOS Simulator,
// some emulators, or before SetSurface) present with the CPU path: each vsync call
// Snapshot and blit the returned pixels.
func GpuActive() bool { return bridge.GPUActive() }

// RenderFrame renders one frame on the GPU to the surface set by SetSurface.
func RenderFrame(dtSeconds float64) { bridge.RenderFrame(dtSeconds) }

// Snapshot renders one frame offscreen and returns RGBA8888 pixels
// (FrameWidth×FrameHeight) — the CPU present path, and screenshots.
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

// SetDarkMode forwards the host colour scheme.
func SetDarkMode(dark bool) { bridge.SetDarkMode(dark) }

// TakeOpenedURL returns a URL the UI asked to open ("" if none).
func TakeOpenedURL() string { return bridge.TakeOpenedURL() }

// TakeHaptic returns the next queued haptic event's kind, or -1 when none are
// pending. The host drains it each frame and plays it on the OS generator.
func TakeHaptic() int { return bridge.TakeHaptic() }

// Focused forwards app focus/visibility.
func Focused(f bool) { bridge.Focused(f) }

// SetInsets forwards safe-area insets in physical pixels — the notch and the home
// indicator, which a full-bleed ledger has to respect.
func SetInsets(top, right, bottom, left float64) {
	bridge.SetInsets(float32(top), float32(right), float32(bottom), float32(left))
}

// SetKeyboardHeight forwards the on-screen keyboard's height in physical pixels,
// or 0 when it is dismissed. Without it a phone form is unusable: the keyboard
// covers the field being typed into and the platform moves nothing.
func SetKeyboardHeight(heightPx float64) { bridge.SetKeyboardHeight(float32(heightPx)) }

// TextInputActive reports whether the UI wants the on-screen keyboard.
func TextInputActive() bool { return bridge.TextInputActive() }

// Composition forwards IME preedit (kind: 0 start, 1 update, 2 end).
func Composition(kind int, preedit string, cursor int, committed string) {
	bridge.Composition(kind, preedit, cursor, committed)
}

// Accessibility: the host refreshes, then reads a flat node tree and activates by
// ID. Rects are physical pixels. VoiceOver and TalkBack both consume this — an app
// that draws its own pixels has no native views for a screen reader to find, so
// the semantics tree is the only thing standing between Tally and being unusable
// for someone who cannot see it (and it is something Apple review looks at).
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
