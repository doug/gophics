package gputypes

// Feature represents a WebGPU feature flag.
//
// Features are optional capabilities that may not be available on all GPUs.
// Check Features.Contains() to determine if a feature is supported.
type Feature uint64

const (
	// FeatureDepthClipControl enables depth clip control (unclipped depth).
	FeatureDepthClipControl Feature = 1 << iota
	// FeatureDepth32FloatStencil8 enables the depth32float-stencil8 texture format.
	FeatureDepth32FloatStencil8
	// FeatureTextureCompressionBC enables BC texture compression formats.
	FeatureTextureCompressionBC
	// FeatureTextureCompressionETC2 enables ETC2 texture compression formats.
	FeatureTextureCompressionETC2
	// FeatureTextureCompressionASTC enables ASTC texture compression formats.
	FeatureTextureCompressionASTC
	// FeatureIndirectFirstInstance enables first instance parameter in indirect draws.
	FeatureIndirectFirstInstance
	// FeatureShaderF16 enables f16 (half-precision float) in shaders.
	FeatureShaderF16
	// FeatureRG11B10UfloatRenderable enables RG11B10Ufloat as a render target format.
	FeatureRG11B10UfloatRenderable
	// FeatureBGRA8UnormStorage enables BGRA8Unorm as a storage texture format.
	FeatureBGRA8UnormStorage
	// FeatureFloat32Filterable enables filtering of float32 textures.
	FeatureFloat32Filterable
	// FeatureTimestampQuery enables timestamp queries.
	FeatureTimestampQuery
	// FeaturePipelineStatisticsQuery enables pipeline statistics queries.
	FeaturePipelineStatisticsQuery
	// FeatureMultiDrawIndirect enables multi-draw indirect commands.
	FeatureMultiDrawIndirect
	// FeatureMultiDrawIndirectCount enables multi-draw indirect with count.
	FeatureMultiDrawIndirectCount
	// FeaturePushConstants enables push constants (non-standard extension).
	FeaturePushConstants
	// FeatureTextureAdapterSpecificFormatFeatures enables adapter-specific texture formats.
	FeatureTextureAdapterSpecificFormatFeatures
	// FeatureShaderFloat64 enables f64 (double-precision float) in shaders.
	FeatureShaderFloat64
	// FeatureVertexAttribute64bit enables 64-bit vertex attributes.
	FeatureVertexAttribute64bit
	// FeatureSubgroupOperations enables subgroup operations in shaders.
	FeatureSubgroupOperations
	// FeatureSubgroupBarrier enables subgroup barriers in shaders.
	FeatureSubgroupBarrier
)

// String returns the feature name.
func (f Feature) String() string {
	switch f {
	case FeatureDepthClipControl:
		return "DepthClipControl"
	case FeatureDepth32FloatStencil8:
		return "Depth32FloatStencil8"
	case FeatureTextureCompressionBC:
		return "TextureCompressionBC"
	case FeatureTextureCompressionETC2:
		return "TextureCompressionETC2"
	case FeatureTextureCompressionASTC:
		return "TextureCompressionASTC"
	case FeatureIndirectFirstInstance:
		return "IndirectFirstInstance"
	case FeatureShaderF16:
		return "ShaderF16"
	case FeatureRG11B10UfloatRenderable:
		return "RG11B10UfloatRenderable"
	case FeatureBGRA8UnormStorage:
		return "BGRA8UnormStorage"
	case FeatureFloat32Filterable:
		return "Float32Filterable"
	case FeatureTimestampQuery:
		return "TimestampQuery"
	case FeaturePipelineStatisticsQuery:
		return "PipelineStatisticsQuery"
	case FeatureMultiDrawIndirect:
		return "MultiDrawIndirect"
	case FeatureMultiDrawIndirectCount:
		return "MultiDrawIndirectCount"
	case FeaturePushConstants:
		return "PushConstants"
	case FeatureTextureAdapterSpecificFormatFeatures:
		return "TextureAdapterSpecificFormatFeatures"
	case FeatureShaderFloat64:
		return "ShaderFloat64"
	case FeatureVertexAttribute64bit:
		return "VertexAttribute64bit"
	case FeatureSubgroupOperations:
		return "SubgroupOperations"
	case FeatureSubgroupBarrier:
		return "SubgroupBarrier"
	default:
		return "Unknown"
	}
}

// Features is a set of feature flags.
type Features uint64

// Contains checks if the feature set contains a specific feature.
func (f Features) Contains(feature Feature) bool {
	return f&Features(feature) != 0
}

// ContainsAll checks if the feature set contains all specified features.
func (f Features) ContainsAll(other Features) bool {
	return f&other == other
}

// Insert adds a feature to the set.
func (f *Features) Insert(feature Feature) {
	*f |= Features(feature)
}

// Remove removes a feature from the set.
func (f *Features) Remove(feature Feature) {
	*f &^= Features(feature)
}

// Intersect returns features common to both sets.
func (f Features) Intersect(other Features) Features {
	return f & other
}

// Union returns all features from both sets.
func (f Features) Union(other Features) Features {
	return f | other
}

// IsEmpty returns true if no features are set.
func (f Features) IsEmpty() bool {
	return f == 0
}

// Count returns the number of features in the set.
func (f Features) Count() int {
	count := 0
	for v := f; v != 0; v &= v - 1 {
		count++
	}
	return count
}
