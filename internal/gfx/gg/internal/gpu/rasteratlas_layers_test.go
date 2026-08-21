//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
)

// On rasterAtlas there is no way to render into a layer texture, so layer
// markers and backdrop blurs must be dropped rather than resolved.
//
// That strategy rasterizes shapes into a CPU pixmap and uploads the pixmap to
// the target. A layer target is a bare texture with no pixmap behind it, so the
// upload finds nothing to send and reports success having written nothing — and
// the caller composites a freshly created, still-black texture as if it were
// the group's contents. A translucent white card over black is flat grey, which
// is what a frosted panel turned into on a Pixel.
func TestRasterAtlasDropsLayersInsteadOfCompositingBlack(t *testing.T) {
	rc := &GPURenderContext{shared: &GPUShared{strategy: strategyRasterAtlas}}
	rc.pendingDraws = []drawCommand{
		{kind: drawCmdPushLayer},
		{kind: drawCmdFillShape},
		{kind: drawCmdPopLayer},
		{kind: drawCmdBackdropBlur},
		{kind: drawCmdFillShape},
	}

	// Dimensions are fine; the strategy is what makes a layer impossible.
	rc.resolveLayers(gg.GPURenderTarget{Width: 64, Height: 64})

	for _, d := range rc.pendingDraws {
		switch d.kind {
		case drawCmdPushLayer, drawCmdPopLayer:
			t.Errorf("a layer marker survived on rasterAtlas; the group would be " +
				"composited from an empty texture and render black")
		case drawCmdBackdropBlur:
			t.Errorf("a backdrop blur survived on rasterAtlas; it would frost from " +
				"an empty texture and render black")
		}
	}
	if len(rc.pendingDraws) != 2 {
		t.Errorf("kept %d draws, want the 2 real ones", len(rc.pendingDraws))
	}
}
