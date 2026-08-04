//go:build !nogpu

package gpu

import (
	"fmt"

	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// GPUInfo contains information about the selected GPU.
type GPUInfo struct {
	// Name is the GPU name (e.g., "NVIDIA GeForce RTX 3080").
	Name string
	// Vendor is the GPU vendor.
	Vendor string
	// DeviceType is the type of GPU (discrete, integrated, etc.).
	DeviceType gputypes.DeviceType
	// Backend is the graphics API in use (Vulkan, Metal, DX12, WebGPU).
	Backend gputypes.Backend
	// Driver is the driver version string.
	Driver string
}

// String returns a human-readable description of the GPU.
func (g *GPUInfo) String() string {
	return fmt.Sprintf("%s (%s, %s)", g.Name, g.DeviceType, g.Backend)
}

// getGPUInfo retrieves information about the GPU adapter.
func getGPUInfo(adapter *wgpu.Adapter) (*GPUInfo, error) {
	if adapter == nil {
		return nil, fmt.Errorf("nil adapter")
	}
	info := adapter.Info()
	return &GPUInfo{
		Name:       info.Name,
		Vendor:     info.Vendor,
		DeviceType: info.DeviceType,
		Backend:    info.Backend,
		Driver:     info.Driver,
	}, nil
}

// logGPUInfo logs information about the selected GPU.
func logGPUInfo(adapter *wgpu.Adapter) {
	info, err := getGPUInfo(adapter)
	if err != nil {
		slogger().Warn("failed to get GPU info", "err", err)
		return
	}
	slogger().Info("GPU selected", "gpu", info.String(), "driver", info.Driver)
}

// CheckDeviceLimits logs the device's basic limits for diagnostics.
func CheckDeviceLimits(device *wgpu.Device) error {
	if device == nil {
		return fmt.Errorf("nil device")
	}
	limits := device.Limits()
	slogger().Debug("device limits",
		"maxTexture2D", limits.MaxTextureDimension2D,
		"maxBuffer", limits.MaxBufferSize)
	return nil
}
