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
	hn "github.com/doug/gossamer/examples/hn/ui"
	"github.com/doug/gossamer/shell/mobile"
)

var bridge *mobile.Bridge

// Start builds the app; call once from the host before any other call.
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

// Resize sets the surface size in physical pixels and the density scale.
func Resize(widthPx, heightPx int, scale float64) {
	bridge.Resize(widthPx, heightPx, float32(scale))
}

// NeedsFrame reports whether the UI wants a repaint (poll each vsync).
func NeedsFrame() bool { return bridge.NeedsFrame() }

// RenderFrame renders and returns RGBA8888 pixels (widthPx*heightPx*4).
func RenderFrame(dtSeconds float64) []byte { return bridge.RenderFrame(dtSeconds) }

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
