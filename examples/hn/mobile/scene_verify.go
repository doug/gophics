//go:build gophics_verify

package hnmobile

import (
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"

	gpucheck "github.com/doug/gophics/examples/gpucheck/ui"
)

// scene (verify build) is the GPU bring-up diagnostic instead of HN. Selected
// by -tags gophics_verify. See design/mobile-gpu-bringup.md.
func scene() (widget.Widget, paint.Color) { return gpucheck.Root(), gpucheck.Background() }
