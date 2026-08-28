package healthui

import (
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

// Root is the dashboard with the synthetic provider — what the desktop and web
// builds run.
//
// The mobile bind package builds its own App with a device-backed provider
// instead (HealthKit / Health Connect), which is the one thing about this app
// that genuinely differs by platform. It shares Config with this one, so only
// the data source varies and not the fonts or the background.
func Root() widget.Widget { return App{} }

// Config is the app's window and font configuration, shared by the desktop
// entry point and the mobile bind surface so the two cannot drift.
func Config() app.Config {
	return app.Config{
		Title:      "Health",
		Size:       geom.Size{W: 390, H: 760}, // phone-portrait, signalling the mobile target
		Background: BG,
		Font:       goregular.TTF,
	}
}
