//go:build windows && !(js && wasm)

package gl

// ep normalises an entry point for the null check in Validate; see the Linux
// counterpart. Windows already stores them as uintptr.
func ep(p uintptr) uintptr { return p }
