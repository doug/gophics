//go:build !gossamer_gpu

package app

import (
	"log"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/shell"
)

// present rasterizes the recorded scene on the CPU and hands the finished
// pixels to the shell (the default M1 model). The gossamer_gpu build replaces
// this with a GPU-rasterizing variant (present_gpu.go).
func (h *shellHandler) present(f shell.Frame, changed bool, damage geom.Rect) {
	if changed {
		canvas := h.core.Painter.Begin(f)
		h.core.ReplayDamaged(canvas, damage)
	}
	// Present even when skipped: the painter's surface is retained, and the
	// swapchain still needs this frame's image.
	if err := h.core.Painter.End(f); err != nil {
		log.Printf("gossamer: present: %v", err)
	}
}
