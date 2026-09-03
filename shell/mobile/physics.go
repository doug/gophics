package mobile

import (
	"runtime"

	"github.com/doug/gophics/shell"
)

// ScrollPhysics is the platform's fling curve. The bridge is compiled once per
// platform by gomobile, so GOOS is the answer — there is no runtime to ask.
func (b *Bridge) ScrollPhysics() shell.ScrollPhysics {
	if runtime.GOOS == "android" {
		return shell.AndroidScrollPhysics()
	}
	return shell.IOSScrollPhysics()
}
