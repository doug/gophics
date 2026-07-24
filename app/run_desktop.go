//go:build !js

package app

import (
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/shell/desktop"
)

func desktopRun(h shell.Handler, cfg shell.Config) error {
	return desktop.Run(h, cfg)
}
