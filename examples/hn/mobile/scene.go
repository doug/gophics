//go:build !gophics_verify

package hnmobile

import (
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"

	hn "github.com/doug/gophics/examples/hn/ui"
)

// scene returns the widget tree the mobile host runs. The default is the HN
// app; building with -tags gophics_verify swaps in the GPU bring-up scene
// (see scene_verify.go and docs/mobile-gpu-bringup.md) — e.g.
// `gophics run -p ios -tags gophics_verify ./examples/hn/mobile`.
func scene() (widget.Widget, paint.Color) { return hn.Root(), hn.Background() }
