//go:build gossamer_verify

package hnmobile

import (
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"

	gpucheck "github.com/doug/gossamer/examples/gpucheck/ui"
)

// scene (verify build) is the GPU bring-up diagnostic instead of HN. Selected
// by -tags gossamer_verify. See docs/mobile-gpu-bringup.md.
func scene() (widget.Widget, paint.Color) { return gpucheck.Root(), gpucheck.Background() }
