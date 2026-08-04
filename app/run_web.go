//go:build js && wasm

package app

import (
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/web"
)

func desktopRun(h shell.Handler, cfg shell.Config) error {
	return web.Run(h, cfg)
}
