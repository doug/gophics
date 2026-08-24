//go:build !(js && wasm)

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import (
	"fmt"
	"unsafe"

	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu/hal"
	"github.com/doug/gophics/internal/gfx/wgpu/hal/vulkan/vk"
)

func validateAndroidSurfaceSupport(hasWSIQueries, hasCreateCommand bool) error {
	if !hasWSIQueries || !hasCreateCommand {
		return fmt.Errorf("vulkan: Android surface WSI is unavailable")
	}
	return nil
}

func validateAndroidSurfaceRequest(window uintptr) error {
	if window == 0 {
		return fmt.Errorf("vulkan: Android ANativeWindow must be non-null")
	}
	return nil
}

func validateAndroidSDKVersion(sdk uint32) error {
	if sdk < 29 {
		return fmt.Errorf("vulkan: Android API 29 or newer is required, device reports %d", sdk)
	}
	return nil
}

// validateAndroidInstanceFlags admits every flag, including Debug.
//
// It used to reject Debug outright, on the grounds that "debug callbacks are
// unsupported on Android" — which is true and was the wrong conclusion. The
// restriction is real: goffi cannot build C-callable callbacks on Android, so
// vkCreateDebugUtilsMessengerEXT can never be used there. But this package
// never creates a messenger on any platform; Instance.debugMessenger is
// declared and never assigned. What the Debug flag actually turns on is
// VK_LAYER_KHRONOS_validation and VK_EXT_debug_utils, and the validation layer
// on Android writes to logcat by itself and needs no callback at all.
//
// So the guard was refusing to create an instance in order to protect against a
// callback nothing registers, and the cost was that validation layers — the
// first tool anyone reaches for — were unavailable on the one platform whose
// GPU drivers most need them. A text-rendering bug on Adreno was diagnosed by
// bisecting shaders by hand for exactly this reason.
//
// If a messenger is ever added, gate that on the platform rather than gating
// the whole instance here.
func validateAndroidInstanceFlags(gputypes.InstanceFlags) error { return nil }

func setAndroidSurfaceNativeWindow(createInfo *vk.AndroidSurfaceCreateInfoKHR, window uintptr) {
	// Window is generated as *vk.ANativeWindow but contains a raw C pointer.
	// Store its integer bits in-place without manufacturing a Go pointer.
	*(*uintptr)(unsafe.Pointer(&createInfo.Window)) = window
}

func mapAndroidSurfaceCreateError(result vk.Result) error {
	switch result {
	case vk.ErrorSurfaceLostKhr, vk.ErrorInitializationFailed:
		return fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR failed: %w", hal.ErrSurfaceLost)
	case vk.ErrorNativeWindowInUseKhr:
		return fmt.Errorf("vulkan: vkCreateAndroidSurfaceKHR failed: native window is already in use")
	default:
		return mapVulkanResult("vkCreateAndroidSurfaceKHR", result)
	}
}
