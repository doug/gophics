// Package gallerymobile is the gomobile-bind surface for the widget catalog.
//
// It holds only what is the gallery's own, which is building the tree.
// Everything generic — the frame loop, input, lifecycle, accessibility — is on
// shell/mobile.Bridge, which the CLI binds alongside this package, so a host
// calls those methods on the Bridge that Start returns.
package gallerymobile

import (
	"os"
	"strings"

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

// CaptureGPU writes one GPU-rendered frame to path as a PNG, and the glyph
// atlas page beside it.
//
// For diagnosing a rendering fault that only a device shows. The host calls it
// with a path inside the app container and devicectl copies the files off.
// Rendering rather than screenshotting is the point: the frame comes off the
// same device, canvas and glyph atlas the screen does, so a fault in that
// atlas is in the file — and the atlas dump beside it says whether the atlas
// or the sampling is at fault. That pair is what found the page-index bug.
func CaptureGPU(bridge *mobile.Bridge, path string) string {
	if bridge == nil {
		return "no bridge"
	}
	data := bridge.CaptureGPU(1.0 / 60)
	if len(data) == 0 {
		return "capture produced nothing (no GPU surface?)"
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err.Error()
	}
	// The atlas beside the frame. If the atlas is right and the frame is not,
	// the GPU texture is stale; if the atlas is wrong, the fault is upstream of
	// the GPU altogether. Nothing else separates those two.
	if page := gtext.DumpAtlasPage(0); len(page) > 0 {
		_ = os.WriteFile(strings.TrimSuffix(path, ".png")+"_atlas.png", page, 0o644)
	}
	return ""
}
