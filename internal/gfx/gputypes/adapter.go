package gputypes

// DeviceType identifies the type of GPU device.
type DeviceType uint8

const (
	// DeviceTypeOther is an unknown or other device type.
	DeviceTypeOther DeviceType = iota
	// DeviceTypeIntegratedGPU is integrated into the CPU (shared memory).
	DeviceTypeIntegratedGPU
	// DeviceTypeDiscreteGPU is a separate GPU with dedicated memory.
	DeviceTypeDiscreteGPU
	// DeviceTypeVirtualGPU is a virtual GPU (e.g., in a VM).
	DeviceTypeVirtualGPU
	// DeviceTypeCPU is software rendering on the CPU.
	DeviceTypeCPU
)

// String returns the device type name.
func (d DeviceType) String() string {
	switch d {
	case DeviceTypeOther:
		return "Other"
	case DeviceTypeIntegratedGPU:
		return "IntegratedGPU"
	case DeviceTypeDiscreteGPU:
		return "DiscreteGPU"
	case DeviceTypeVirtualGPU:
		return "VirtualGPU"
	case DeviceTypeCPU:
		return "CPU"
	default:
		return "Unknown"
	}
}

// Backend identifies the graphics API backend.
type Backend uint8

const (
	// BackendEmpty represents no backend (invalid state).
	BackendEmpty Backend = iota
	// BackendVulkan uses the Vulkan API (cross-platform).
	BackendVulkan
	// BackendMetal uses Apple's Metal API (macOS, iOS).
	BackendMetal
	// BackendDX12 uses Microsoft DirectX 12 (Windows).
	BackendDX12
	// BackendGL uses OpenGL/OpenGL ES (legacy fallback).
	BackendGL
	// BackendBrowserWebGPU uses the browser's native WebGPU (WASM).
	BackendBrowserWebGPU
)

// String returns the backend name.
func (b Backend) String() string {
	switch b {
	case BackendEmpty:
		return "Empty"
	case BackendVulkan:
		return "Vulkan"
	case BackendMetal:
		return "Metal"
	case BackendDX12:
		return "DX12"
	case BackendGL:
		return "GL"
	case BackendBrowserWebGPU:
		return "BrowserWebGPU"
	default:
		return "Unknown"
	}
}

// Backends is a set of backend flags.
type Backends uint8

const (
	// BackendsNone includes no backends.
	BackendsNone Backends = 0
	// BackendsVulkan includes Vulkan.
	BackendsVulkan Backends = 1 << BackendVulkan
	// BackendsMetal includes Metal.
	BackendsMetal Backends = 1 << BackendMetal
	// BackendsDX12 includes DirectX 12.
	BackendsDX12 Backends = 1 << BackendDX12
	// BackendsGL includes OpenGL/OpenGL ES.
	BackendsGL Backends = 1 << BackendGL
	// BackendsBrowserWebGPU includes browser WebGPU.
	BackendsBrowserWebGPU Backends = 1 << BackendBrowserWebGPU

	// BackendsPrimary includes all primary backends (Vulkan, Metal, DX12, BrowserWebGPU).
	BackendsPrimary = BackendsVulkan | BackendsMetal | BackendsDX12 | BackendsBrowserWebGPU
	// BackendsSecondary includes fallback backends (GL only).
	BackendsSecondary = BackendsGL
	// BackendsAll includes all backends.
	BackendsAll = BackendsPrimary | BackendsSecondary
)

// Contains checks if the backend set contains a specific backend.
func (b Backends) Contains(backend Backend) bool {
	if backend == BackendEmpty {
		return false
	}
	return b&(1<<backend) != 0
}

// AdapterInfo contains information about a GPU adapter.
type AdapterInfo struct {
	// Name is the human-readable name of the adapter (e.g., "NVIDIA GeForce RTX 4090").
	Name string
	// Vendor is the adapter vendor name (e.g., "NVIDIA", "AMD", "Intel").
	Vendor string
	// VendorID is the PCI vendor ID.
	VendorID uint32
	// DeviceID is the PCI device ID.
	DeviceID uint32
	// DeviceType indicates the type of GPU (discrete, integrated, etc.).
	DeviceType DeviceType
	// Driver is the driver version string.
	Driver string
	// DriverInfo is additional driver information.
	DriverInfo string
	// Backend is the graphics API backend in use.
	Backend Backend
}

// PowerPreference specifies power consumption preference for adapter selection.
type PowerPreference uint8

const (
	// PowerPreferenceNone has no preference (system default).
	PowerPreferenceNone PowerPreference = iota
	// PowerPreferenceLowPower prefers low power consumption (typically integrated GPU).
	PowerPreferenceLowPower
	// PowerPreferenceHighPerformance prefers high performance (typically discrete GPU).
	PowerPreferenceHighPerformance
)

// String returns the power preference name.
func (p PowerPreference) String() string {
	switch p {
	case PowerPreferenceNone:
		return "None"
	case PowerPreferenceLowPower:
		return "LowPower"
	case PowerPreferenceHighPerformance:
		return "HighPerformance"
	default:
		return "Unknown"
	}
}

// RequestAdapterOptions controls adapter selection.
type RequestAdapterOptions struct {
	// PowerPreference indicates power consumption preference.
	PowerPreference PowerPreference
	// ForceFallbackAdapter forces the use of a fallback (software) adapter.
	ForceFallbackAdapter bool
	// CompatibleSurface is a handle to a surface the adapter must support (0 if none).
	CompatibleSurface uintptr
}

// MemoryHints provides memory allocation hints for device creation.
type MemoryHints uint8

const (
	// MemoryHintsPerformance optimizes for performance (may use more memory).
	MemoryHintsPerformance MemoryHints = iota
	// MemoryHintsMemoryUsage optimizes for low memory usage.
	MemoryHintsMemoryUsage
)

// String returns the memory hints name.
func (h MemoryHints) String() string {
	switch h {
	case MemoryHintsPerformance:
		return "Performance"
	case MemoryHintsMemoryUsage:
		return "MemoryUsage"
	default:
		return "Unknown"
	}
}

// DeviceDescriptor describes how to create a GPU device.
type DeviceDescriptor struct {
	// Label is an optional debug label.
	Label string
	// RequiredFeatures lists features the device must support.
	RequiredFeatures []Feature
	// RequiredLimits specifies limits the device must meet.
	RequiredLimits Limits
	// MemoryHints provides memory allocation hints.
	MemoryHints MemoryHints
}

// DefaultDeviceDescriptor returns a device descriptor with default settings.
func DefaultDeviceDescriptor() DeviceDescriptor {
	return DeviceDescriptor{
		RequiredFeatures: nil,
		RequiredLimits:   DefaultLimits(),
		MemoryHints:      MemoryHintsPerformance,
	}
}

// Dx12ShaderCompiler specifies the shader compiler for DX12 backend.
type Dx12ShaderCompiler uint8

const (
	// Dx12ShaderCompilerFxc uses the legacy FXC compiler.
	Dx12ShaderCompilerFxc Dx12ShaderCompiler = iota
	// Dx12ShaderCompilerDxc uses the modern DXC compiler.
	Dx12ShaderCompilerDxc
)

// String returns the DX12 shader compiler name.
func (c Dx12ShaderCompiler) String() string {
	switch c {
	case Dx12ShaderCompilerFxc:
		return "FXC"
	case Dx12ShaderCompilerDxc:
		return "DXC"
	default:
		return "Unknown"
	}
}

// GLBackend specifies the OpenGL backend flavor.
type GLBackend uint8

const (
	// GLBackendGL uses desktop OpenGL.
	GLBackendGL GLBackend = iota
	// GLBackendGLES uses OpenGL ES.
	GLBackendGLES
)

// String returns the GL backend name.
func (b GLBackend) String() string {
	switch b {
	case GLBackendGL:
		return "GL"
	case GLBackendGLES:
		return "GLES"
	default:
		return "Unknown"
	}
}

// InstanceFlags controls GPU instance behavior.
type InstanceFlags uint8

const (
	// InstanceFlagsNone uses default behavior.
	InstanceFlagsNone InstanceFlags = 0
	// InstanceFlagsDebug enables debug layers when available.
	InstanceFlagsDebug InstanceFlags = 1 << iota
	// InstanceFlagsValidation enables validation layers.
	InstanceFlagsValidation
	// InstanceFlagsGPUBasedValidation enables GPU-based validation (slower).
	InstanceFlagsGPUBasedValidation
	// InstanceFlagsDiscardHalLabels discards HAL debug labels.
	InstanceFlagsDiscardHalLabels
)

// InstanceDescriptor describes how to create a GPU instance.
type InstanceDescriptor struct {
	// Backends specifies which backends to enable.
	Backends Backends
	// Flags controls instance behavior (debug, validation, etc.).
	Flags InstanceFlags
	// Dx12ShaderCompiler specifies the DX12 shader compiler.
	Dx12ShaderCompiler Dx12ShaderCompiler
	// GLBackend specifies the OpenGL backend flavor.
	GLBackend GLBackend
}

// DefaultInstanceDescriptor returns an instance descriptor with default settings.
func DefaultInstanceDescriptor() InstanceDescriptor {
	return InstanceDescriptor{
		Backends:           BackendsPrimary,
		Flags:              InstanceFlagsNone,
		Dx12ShaderCompiler: Dx12ShaderCompilerDxc,
		GLBackend:          GLBackendGL,
	}
}
