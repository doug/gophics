// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build darwin && !ios && !(js && wasm)

package metal

// deviceRespondsTo reports whether the Metal device implements sel. Used only on
// macOS, where these introspection selectors genuinely exist on some devices and
// not others (e.g. a headless server GPU vs. a laptop iGPU).
func deviceRespondsTo(device ID, sel string) bool {
	if device == 0 {
		return false
	}
	return MsgSendBool(device, Sel("respondsToSelector:"), uintptr(Sel(sel)))
}

// DeviceIsLowPower returns true if the device is low-power (integrated).
func DeviceIsLowPower(device ID) bool {
	if !deviceRespondsTo(device, "isLowPower") {
		return true
	}
	return MsgSendBool(device, Sel("isLowPower"))
}

// DeviceIsHeadless returns true if the device is headless (no attached display).
func DeviceIsHeadless(device ID) bool {
	if !deviceRespondsTo(device, "isHeadless") {
		return false
	}
	return MsgSendBool(device, Sel("isHeadless"))
}

// DeviceIsRemovable returns true if the device is removable (eGPU).
func DeviceIsRemovable(device ID) bool {
	if !deviceRespondsTo(device, "isRemovable") {
		return false
	}
	return MsgSendBool(device, Sel("isRemovable"))
}

// DeviceRecommendedMaxWorkingSetSize returns the recommended max working set size.
func DeviceRecommendedMaxWorkingSetSize(device ID) uint64 {
	if !deviceRespondsTo(device, "recommendedMaxWorkingSetSize") {
		return 0
	}
	return uint64(MsgSend(device, Sel("recommendedMaxWorkingSetSize")))
}

// deviceSupportsDepth24Stencil8 reports whether the device supports the packed
// Depth24Unorm_Stencil8 format (macOS-only; discrete/Intel GPUs support it).
func deviceSupportsDepth24Stencil8(device ID) bool {
	return deviceRespondsTo(device, "isDepth24Stencil8PixelFormatSupported") &&
		MsgSendBool(device, Sel("isDepth24Stencil8PixelFormatSupported"))
}
