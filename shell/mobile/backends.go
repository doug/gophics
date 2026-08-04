//go:build !(js && wasm)

package mobile

// wgpu only links the HAL backends you import: hal.RegisterBackend runs from
// each backend package's init, so an unimported backend simply does not exist
// at runtime. The desktop shell gets its backends transitively through gogpu's
// renderer, but the mobile build never pulls gogpu in — so without this import
// the mobile binary has zero registered backends, CreateSurface fails with
// "no HAL instance available for surface creation", and SetSurface silently
// falls back to the CPU blit on every device (see gpu.go).
//
// allbackends picks the right set per platform: Vulkan on android/arm64, Metal
// (+ Vulkan via MoltenVK) on darwin/ios. Backends whose instance creation fails
// are skipped at runtime, so importing one that the device can't support is
// harmless.
import _ "github.com/doug/wgpu/hal/allbackends"
