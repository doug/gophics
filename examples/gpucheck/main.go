// Command gpucheck runs the mobile-GPU verification scene on desktop, which is
// also where the desktop-GPU reference for the on-device comparison comes from
// (gpu_ref_test.go). On a phone the same scene runs through its own bind,
// examples/gpucheck/mobile.
package main

import (
	"log"

	"github.com/doug/gophics/app"
	gpucheck "github.com/doug/gophics/examples/gpucheck/ui"
)

func main() {
	if err := app.Run(gpucheck.Root(), gpucheck.Config()); err != nil {
		log.Fatal(err)
	}
}
