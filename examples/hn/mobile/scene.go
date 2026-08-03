//go:build !gossamer_verify

package hnmobile

import (
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"

	hn "github.com/doug/gossamer/examples/hn/ui"
)

// scene returns the widget tree the mobile host runs. The default is the HN
// app; building with -tags gossamer_verify swaps in the GPU bring-up scene
// (see scene_verify.go and docs/mobile-gpu-bringup.md) — e.g.
// `gossamer run -p ios -tags gossamer_verify ./examples/hn/mobile`.
func scene() (widget.Widget, paint.Color) { return hn.Root(), hn.Background() }
