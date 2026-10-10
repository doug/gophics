// Package ui is the widget tree of the scratch app the bind-package generator
// is tested against. It exports Root and Config because that pair is the
// convention the generator reads — see checkUIExports.
package ui

import (
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Root is the app's widget tree.
func Root() widget.Widget { return widget.Center(widget.Text{Value: "bind"}) }

// Config is the app's window configuration.
func Config() app.Config { return app.Config{Background: paint.RGB(1, 1, 1)} }
