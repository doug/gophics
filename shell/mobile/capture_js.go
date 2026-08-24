//go:build js && wasm

package mobile

// CaptureGPU is unavailable in the browser: the page owns the canvas and its
// own devtools can capture it, so there is nothing here worth duplicating.
func (b *Bridge) CaptureGPU(dtSeconds float64) []byte { return nil }
