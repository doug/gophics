package tallymobile

import (
	"github.com/doug/gophics/widget"

	"github.com/doug/tally/ui"
)

// newRoot is the widget tree the mobile hosts render — the same one the desktop
// binary runs, with no mobile-specific variant.
func newRoot() widget.Widget { return ui.App{} }
