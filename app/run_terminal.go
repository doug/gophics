//go:build gophics_term && !js && !android && !ios

package app

import (
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/terminal"
)

// desktopRun routes app.Run to the terminal backend (kitty graphics protocol)
// instead of a desktop window when built with -tags gophics_term. The app code
// is unchanged; only the presentation shell differs.
//
// The terminal transports CPU pixels (the kitty protocol carries an RGBA image),
// so there is no GPU present path here — force the CPU rasterizer and release
// the GPU accelerator's device, which would otherwise sit idle for the whole run.
func desktopRun(h shell.Handler, cfg shell.Config) error {
	paint.UseCPU()
	return terminal.Run(h, cfg)
}
