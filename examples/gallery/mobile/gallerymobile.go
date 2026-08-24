// Package gallerymobile is the gomobile-bind surface for the widget catalog.
//
// It holds only what is the gallery's own, which is building the tree.
// Everything generic — the frame loop, input, lifecycle, accessibility — is on
// shell/mobile.Bridge, which the CLI binds alongside this package, so a host
// calls those methods on the Bridge that Start returns.
package gallerymobile

import (
	"fmt"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/gallery/ui"
	gtext "github.com/doug/gophics/internal/gfx/gg/text"
	"github.com/doug/gophics/shell/mobile"
)

// Start builds the app and returns the bridge the host drives it through.
//
// Call it once, before anything else. On failure it returns a nil bridge and
// the error to show — two results because the second is an error, which is the
// one shape gomobile allows.
func Start() (*mobile.Bridge, error) {
	h, err := app.NewHandler(ui.Gallery{}, ui.Config())
	if err != nil {
		return nil, err
	}
	return mobile.NewBridge(h), nil
}

// AtlasStats reports the glyph atlas counters as a line the host can log.
//
// Temporary, for chasing a rendering fault that only appears on a real device.
// It exists because Go's own log output does not reach the device syslog —
// gomobile writes it through os_log at a level the stream drops — while NSLog
// from the host does, so the shortest path to seeing these numbers off-device
// is to hand them to Swift and let it do the logging.
func AtlasStats() string {
	refusals, evictions, compactions := gtext.AtlasStats()
	writes, late, uploads := gtext.AtlasWriteStats()
	return fmt.Sprintf("ref=%d ev=%d cmp=%d w=%d late=%d up=%d nilview=%d",
		refusals, evictions, compactions, writes, late, uploads, gtext.AtlasNilViews())
}
