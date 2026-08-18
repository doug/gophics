//go:build linux && !(js && wasm)

package gl

import "unsafe"

// ep normalises an entry point for the null check in Validate. The Linux
// context stores pointers as unsafe.Pointer and the Windows one as uintptr, so
// the shared required-list needs one conversion per platform rather than two
// copies of the list.
func ep(p unsafe.Pointer) uintptr { return uintptr(p) }
