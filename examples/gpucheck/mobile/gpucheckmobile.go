// Package gpucheckmobile is the gomobile-bind surface for the GPU bring-up
// diagnostic, so it can be run on a device as itself.
//
// It used to be reached by building *another* app with -tags gophics_verify,
// which swapped this scene in for that app's own. That worked, and it put a
// build-tagged file into an example whose job is to teach the app it contains:
// someone reading hn to learn how a mobile host is wired found a second scene
// file belonging to something else entirely. A diagnostic that needs a host is
// an app that needs a host, so it has its own.
//
// There is no host project checked in beside this one, because the diagnostic
// does not need a permanent one. Point the build at any host with -host, or
// scaffold one with `gophics create`:
//
//	gophics run -p ios -host ./examples/hn/ios ./examples/gpucheck/mobile
//
// What it verifies, and how to read the result, is in examples/gpucheck/ui.
package gpucheckmobile

import (
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/shell/mobile"

	gpucheck "github.com/doug/gophics/examples/gpucheck/ui"
)

// Start builds the diagnostic and returns the bridge the host drives it
// through. Same contract as any app's Start: call it once, before anything
// else, and a nil bridge means the error is the one to show.
func Start() (*mobile.Bridge, error) {
	h, err := app.NewHandler(gpucheck.Root(), gpucheck.Config())
	if err != nil {
		return nil, err
	}
	return mobile.NewBridge(h), nil
}
