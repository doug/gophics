package mobile

import (
	"runtime"

	"github.com/doug/gophics/shell"
)

// GestureTuning is the platform's, by GOOS: the bridge is compiled once per
// platform and there is no runtime to ask.
func (b *Bridge) GestureTuning() shell.GestureTuning {
	if runtime.GOOS == "android" {
		return shell.AndroidGestureTuning()
	}
	return shell.IOSGestureTuning()
}
