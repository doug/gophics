// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build ios

package metal

// On iOS these MTLDevice introspection selectors (isLowPower / isHeadless /
// isRemovable / recommendedMaxWorkingSetSize / isDepth24Stencil8PixelFormatSupported)
// are macOS-only. The concrete iOS device class (e.g. AGXG14Device) implements them
// as throwing stubs, so respondsToSelector: returns YES yet the send still raises
// "unrecognized selector sent to instance" and terminates the app. A runtime guard
// therefore cannot protect against them — the only safe approach is to never emit
// the selectors on iOS. The answers are fixed by the platform anyway:
//
//   - every iOS GPU is integrated/low-power         -> IsLowPower  = true
//   - an iOS GPU always drives the built-in display -> IsHeadless  = false
//   - an iOS GPU is soldered on, never an eGPU       -> IsRemovable = false
//   - working-set size is not queryable              -> 0 (unknown)
//   - iOS uses Depth32Float_Stencil8, not the packed Depth24Unorm_Stencil8

// DeviceIsLowPower returns true — all iOS GPUs are integrated.
func DeviceIsLowPower(device ID) bool { return true }

// DeviceIsHeadless returns false — an iOS GPU always drives the built-in display.
func DeviceIsHeadless(device ID) bool { return false }

// DeviceIsRemovable returns false — iOS GPUs are never removable (no eGPU).
func DeviceIsRemovable(device ID) bool { return false }

// DeviceRecommendedMaxWorkingSetSize returns 0 (unknown) — not queryable on iOS.
func DeviceRecommendedMaxWorkingSetSize(device ID) uint64 { return 0 }

// deviceSupportsDepth24Stencil8 returns false — iOS uses Depth32Float_Stencil8.
func deviceSupportsDepth24Stencil8(device ID) bool { return false }
