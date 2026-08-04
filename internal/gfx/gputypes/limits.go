package gputypes

// Limits describes the GPU resource limits.
//
// These limits define the maximum capabilities of a GPU device.
// The actual limits depend on the hardware and driver.
type Limits struct {
	// MaxTextureDimension1D is the maximum 1D texture dimension.
	MaxTextureDimension1D uint32
	// MaxTextureDimension2D is the maximum 2D texture dimension.
	MaxTextureDimension2D uint32
	// MaxTextureDimension3D is the maximum 3D texture dimension.
	MaxTextureDimension3D uint32
	// MaxTextureArrayLayers is the maximum texture array layer count.
	MaxTextureArrayLayers uint32
	// MaxBindGroups is the maximum number of bind groups.
	MaxBindGroups uint32
	// MaxBindGroupsPlusVertexBuffers is the max bind groups + vertex buffers combined.
	MaxBindGroupsPlusVertexBuffers uint32
	// MaxBindingsPerBindGroup is the max bindings per bind group.
	MaxBindingsPerBindGroup uint32
	// MaxDynamicUniformBuffersPerPipelineLayout is the max dynamic uniform buffers per pipeline layout.
	MaxDynamicUniformBuffersPerPipelineLayout uint32
	// MaxDynamicStorageBuffersPerPipelineLayout is the max dynamic storage buffers per pipeline layout.
	MaxDynamicStorageBuffersPerPipelineLayout uint32
	// MaxSampledTexturesPerShaderStage is the max sampled textures per shader stage.
	MaxSampledTexturesPerShaderStage uint32
	// MaxSamplersPerShaderStage is the max samplers per shader stage.
	MaxSamplersPerShaderStage uint32
	// MaxStorageBuffersPerShaderStage is the max storage buffers per shader stage.
	MaxStorageBuffersPerShaderStage uint32
	// MaxStorageTexturesPerShaderStage is the max storage textures per shader stage.
	MaxStorageTexturesPerShaderStage uint32
	// MaxUniformBuffersPerShaderStage is the max uniform buffers per shader stage.
	MaxUniformBuffersPerShaderStage uint32
	// MaxUniformBufferBindingSize is the max uniform buffer binding size in bytes.
	MaxUniformBufferBindingSize uint64
	// MaxStorageBufferBindingSize is the max storage buffer binding size in bytes.
	MaxStorageBufferBindingSize uint64
	// MinUniformBufferOffsetAlignment is the minimum uniform buffer offset alignment.
	MinUniformBufferOffsetAlignment uint32
	// MinStorageBufferOffsetAlignment is the minimum storage buffer offset alignment.
	MinStorageBufferOffsetAlignment uint32
	// MaxVertexBuffers is the max vertex buffers in a pipeline.
	MaxVertexBuffers uint32
	// MaxBufferSize is the max buffer size in bytes.
	MaxBufferSize uint64
	// MaxVertexAttributes is the max vertex attributes in a pipeline.
	MaxVertexAttributes uint32
	// MaxVertexBufferArrayStride is the max vertex buffer array stride.
	MaxVertexBufferArrayStride uint32
	// MaxInterStageShaderVariables is the max inter-stage shader variables.
	MaxInterStageShaderVariables uint32
	// MaxColorAttachments is the max color attachments in a render pass.
	MaxColorAttachments uint32
	// MaxColorAttachmentBytesPerSample is the max bytes per sample for color attachments.
	MaxColorAttachmentBytesPerSample uint32
	// MaxComputeWorkgroupStorageSize is the max compute workgroup storage in bytes.
	MaxComputeWorkgroupStorageSize uint32
	// MaxComputeInvocationsPerWorkgroup is the max compute invocations per workgroup.
	MaxComputeInvocationsPerWorkgroup uint32
	// MaxComputeWorkgroupSizeX is the max compute workgroup size in X dimension.
	MaxComputeWorkgroupSizeX uint32
	// MaxComputeWorkgroupSizeY is the max compute workgroup size in Y dimension.
	MaxComputeWorkgroupSizeY uint32
	// MaxComputeWorkgroupSizeZ is the max compute workgroup size in Z dimension.
	MaxComputeWorkgroupSizeZ uint32
	// MaxComputeWorkgroupsPerDimension is the max compute workgroups per dimension.
	MaxComputeWorkgroupsPerDimension uint32
	// MaxPushConstantSize is the max push constant size in bytes (non-standard extension).
	MaxPushConstantSize uint32
	// MaxNonSamplerBindings is the max non-sampler bindings.
	MaxNonSamplerBindings uint32
}

// DefaultLimits returns the default WebGPU limits.
//
// These are the minimum guaranteed limits per the WebGPU specification.
// Most hardware supports significantly higher limits.
func DefaultLimits() Limits {
	return Limits{
		MaxTextureDimension1D:                     8192,
		MaxTextureDimension2D:                     8192,
		MaxTextureDimension3D:                     2048,
		MaxTextureArrayLayers:                     256,
		MaxBindGroups:                             4,
		MaxBindGroupsPlusVertexBuffers:            24,
		MaxBindingsPerBindGroup:                   1000,
		MaxDynamicUniformBuffersPerPipelineLayout: 8,
		MaxDynamicStorageBuffersPerPipelineLayout: 4,
		MaxSampledTexturesPerShaderStage:          16,
		MaxSamplersPerShaderStage:                 16,
		MaxStorageBuffersPerShaderStage:           8,
		MaxStorageTexturesPerShaderStage:          4,
		MaxUniformBuffersPerShaderStage:           12,
		MaxUniformBufferBindingSize:               65536,     // 64 KiB
		MaxStorageBufferBindingSize:               134217728, // 128 MiB
		MinUniformBufferOffsetAlignment:           256,
		MinStorageBufferOffsetAlignment:           256,
		MaxVertexBuffers:                          8,
		MaxBufferSize:                             268435456, // 256 MiB
		MaxVertexAttributes:                       16,
		MaxVertexBufferArrayStride:                2048,
		MaxInterStageShaderVariables:              16,
		MaxColorAttachments:                       8,
		MaxColorAttachmentBytesPerSample:          32,
		MaxComputeWorkgroupStorageSize:            16384, // 16 KiB
		MaxComputeInvocationsPerWorkgroup:         256,
		MaxComputeWorkgroupSizeX:                  256,
		MaxComputeWorkgroupSizeY:                  256,
		MaxComputeWorkgroupSizeZ:                  64,
		MaxComputeWorkgroupsPerDimension:          65535,
		MaxPushConstantSize:                       0, // Not in WebGPU spec
		MaxNonSamplerBindings:                     1000000,
	}
}

// DownlevelLimits returns more conservative limits for older hardware.
//
// Use these limits for maximum compatibility with older GPUs or mobile devices.
func DownlevelLimits() Limits {
	return Limits{
		MaxTextureDimension1D:                     2048,
		MaxTextureDimension2D:                     2048,
		MaxTextureDimension3D:                     256,
		MaxTextureArrayLayers:                     256,
		MaxBindGroups:                             4,
		MaxBindGroupsPlusVertexBuffers:            24,
		MaxBindingsPerBindGroup:                   64,
		MaxDynamicUniformBuffersPerPipelineLayout: 8,
		MaxDynamicStorageBuffersPerPipelineLayout: 4,
		MaxSampledTexturesPerShaderStage:          16,
		MaxSamplersPerShaderStage:                 16,
		MaxStorageBuffersPerShaderStage:           4,
		MaxStorageTexturesPerShaderStage:          4,
		MaxUniformBuffersPerShaderStage:           12,
		MaxUniformBufferBindingSize:               16384,     // 16 KiB
		MaxStorageBufferBindingSize:               134217728, // 128 MiB
		MinUniformBufferOffsetAlignment:           256,
		MinStorageBufferOffsetAlignment:           256,
		MaxVertexBuffers:                          8,
		MaxBufferSize:                             268435456, // 256 MiB
		MaxVertexAttributes:                       16,
		MaxVertexBufferArrayStride:                2048,
		MaxInterStageShaderVariables:              16,
		MaxColorAttachments:                       8,
		MaxColorAttachmentBytesPerSample:          32,
		MaxComputeWorkgroupStorageSize:            16352,
		MaxComputeInvocationsPerWorkgroup:         256,
		MaxComputeWorkgroupSizeX:                  256,
		MaxComputeWorkgroupSizeY:                  256,
		MaxComputeWorkgroupSizeZ:                  64,
		MaxComputeWorkgroupsPerDimension:          65535,
		MaxPushConstantSize:                       0,
		MaxNonSamplerBindings:                     1000000,
	}
}
