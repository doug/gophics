//go:build js && wasm

package app

import (
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/shell/web"
)

func desktopRun(h shell.Handler, cfg shell.Config) error {
	return web.Run(h, cfg)
}
