//go:build !gophics_verify

package newsmobile

import (
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/widget"

	news "github.com/doug/gophics/examples/news/ui"
)

// scene returns the widget tree the mobile host runs. The default is the
// reader; building with -tags gophics_verify swaps in the GPU bring-up scene
// (see scene_verify.go and design/mobile-gpu-bringup.md) — e.g.
// `gophics run -p ios -tags gophics_verify ./examples/news/mobile`.
func scene() (widget.Widget, app.Config) { return news.Root(), news.Config() }
