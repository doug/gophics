package main

import "github.com/doug/gossamer/paint"

// rasterizer names the backend actually in effect at runtime (the renderer is
// resolved from Config/GOSSAMER_RENDERER, no longer a build tag), for the demo
// readout.
func rasterizer() string {
	if paint.GPUAvailable() {
		return "GPU"
	}
	return "CPU"
}
