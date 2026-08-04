//go:build darwin

package platform

import "github.com/doug/gossamer/internal/gfx/gogpu/internal/platform/darwin"

// SystemScaleFactor returns the primary display DPI scale factor.
// On macOS this queries [NSScreen mainScreen].backingScaleFactor and is safe
// to call before NSApplication initialization or window creation.
func SystemScaleFactor() float64 {
	return darwin.MainScreenScaleFactor()
}
