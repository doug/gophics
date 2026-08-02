//go:build !nogpu

package paint

// Linking gg's GPU accelerator registers it process-globally at init (SDF
// shapes, tiled path rasterization, MSDF text via wgpu compute). This is the
// default so the GPU present path works out of the box; build with -tags nogpu
// for a pure-CPU binary that never links wgpu. Registration falls back to CPU
// on its own when no GPU device is available — see accel.go.
import _ "github.com/doug/gg/gpu"
