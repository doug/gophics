//go:build gossamer_term && !js && !android && !ios

package app

import (
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/shell/terminal"
)

// desktopRun routes app.Run to the terminal backend (kitty graphics protocol)
// instead of a desktop window when built with -tags gossamer_term. The app code
// is unchanged; only the presentation shell differs.
//
// The terminal transports CPU pixels (the kitty protocol carries an RGBA image),
// so there is no GPU present path here — force the CPU rasterizer and release
// the GPU accelerator's device, which would otherwise sit idle for the whole run.
func desktopRun(h shell.Handler, cfg shell.Config) error {
	paint.UseCPU()
	return terminal.Run(h, cfg)
}
