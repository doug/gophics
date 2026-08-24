// Package tallymobile is the gomobile-bind surface for Tally.
//
// It holds only what is Tally's own, which is building the tree. Everything
// generic — the frame loop, input, lifecycle, accessibility — is on
// shell/mobile.Bridge, which the CLI binds alongside this package, so a host
// calls those methods on the Bridge that Start returns.
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

// Start builds the app and must be called once from the host before anything
// else. It returns "" on success or the error text.
//
// The mono family is registered because Tally's figures are tabular: money only
// lines up in columns if the digits are the same width.
// Start builds the app and returns the bridge the host drives it through.
//
// Call it once, before anything else. On failure it returns a nil bridge and
// the error to show — two results because the second is an error, which is the
// one shape gomobile allows.
func Start() (*mobile.Bridge, error) {
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
		return nil, err
	}
	return mobile.NewBridge(h), nil
}
