//go:build darwin && !ios && !js

package desktop

import (
	"github.com/doug/gophics/internal/objc"
	"github.com/doug/gophics/shell"
)

// GestureTuning reads the Mac's live double-click interval. It is a system
// setting — System Settings → Mouse/Trackpad → double-click speed — and a
// user who slowed it down did so because 0.3s was not enough for them; an app
// that ignores it is the one app on their Mac that does not double-click.
func (w *window) GestureTuning() shell.GestureTuning {
	g := shell.IOSGestureTuning() // touch values, for a touchscreen Mac if one ever exists
	if err := objc.LoadFramework("AppKit"); err == nil {
		if cls := objc.Class("NSEvent"); cls.Valid() {
			if d := cls.SendDouble("doubleClickInterval"); d > 0 && d < 5 {
				g.DoubleTap = d
			}
		}
	}
	return g
}
