// Copyright 2025 The GoGPU Authors
// SPDX-License-Identifier: MIT

//go:build windows && !(js && wasm)

package allbackends

import (
	// Windows-specific HAL backend imports.

	// Vulkan backend - primary backend on Windows.
	_ "github.com/doug/gophics/internal/gfx/wgpu/hal/vulkan"

	// DX12 backend - alternative high-performance backend.
	_ "github.com/doug/gophics/internal/gfx/wgpu/hal/dx12"

	// OpenGL ES backend - fallback for systems without Vulkan/DX12.
	_ "github.com/doug/gophics/internal/gfx/wgpu/hal/gles"
)
