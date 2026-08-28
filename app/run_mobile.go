//go:build android || ios

package app

import (
	"errors"

	"github.com/doug/gophics/shell"
)

// Run cannot run on a phone, and this is the one platform where that is a
// property of the toolchain rather than a gap.
//
// `gomobile bind` produces a library, not an executable: the native host owns
// the process and the event loop and calls into Go, so main never runs and
// there is nothing for Run to hand a window to. That is also why the app is
// built as `gophics build -p ios` rather than `go build` — the artifact is an
// .xcframework or an .aar, not a binary.
//
// What that costs an app author is nothing, as of the generated bind package:
// put the widget tree and the config in an importable ui package, and
//
//	func main() { app.Run(ui.Root(), ui.Config()) }
//
// is the desktop, web and terminal entry point while the CLI generates the
// mobile one from the same two functions. Reaching this error means a mobile
// build called Run directly, which the generated bind package does not do.
func desktopRun(shell.Handler, shell.Config) error {
	return errors.New("app: Run needs a process to own the event loop, and a " +
		"gomobile-bound library is not one — a mobile build is driven by the " +
		"native host through shell/mobile.Bridge, which the bind package the " +
		"`gophics` CLI generates from ui.Root and ui.Config already wires up")
}
