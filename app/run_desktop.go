//go:build !js && !android && !ios && !gophics_term

package app

import (
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/desktop"
)

func desktopRun(h shell.Handler, cfg shell.Config) error {
	return desktop.Run(h, cfg)
}
