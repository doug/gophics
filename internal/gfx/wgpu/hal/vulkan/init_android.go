//go:build android && arm64

// Copyright 2026 The GoGPU Authors
// SPDX-License-Identifier: MIT

package vulkan

import "github.com/doug/gossamer/internal/gfx/wgpu/hal"

func init() {
	hal.RegisterBackend(Backend{})
}
