//go:build gossamer_term && !js && !android && !ios

package app

import (
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/shell/terminal"
)

// desktopRun routes app.Run to the terminal backend (kitty graphics protocol)
// instead of a desktop window when built with -tags gossamer_term. The app code
// is unchanged; only the presentation shell differs.
func desktopRun(h shell.Handler, cfg shell.Config) error {
	return terminal.Run(h, cfg)
}
