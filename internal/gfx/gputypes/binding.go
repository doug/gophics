package gputypes

// BindGroupLayoutDescriptor describes a bind group layout.
type BindGroupLayoutDescriptor struct {
	// Label is an optional debug label.
	Label string
	// Entries are the layout entries.
	Entries []BindGroupLayoutEntry
}

// BindGroupLayoutEntry describes a single binding in a bind group layout.
//
// Exactly one of Buffer, Sampler, Texture, or StorageTexture must be set.
type BindGroupLayoutEntry struct {
	// Binding is the binding number (must match @binding in shader).
	Binding uint32
	// Visibility specifies which shader stages can access this binding.
	Visibility ShaderStages
	// Buffer describes a buffer binding (nil if not a buffer).
	Buffer *BufferBindingLayout
	// Sampler describes a sampler binding (nil if not a sampler).
	Sampler *SamplerBindingLayout
	// Texture describes a texture binding (nil if not a texture).
	Texture *TextureBindingLayout
	// StorageTexture describes a storage texture binding (nil if not storage).
	StorageTexture *StorageTextureBindingLayout
}

// BufferBindingLayout describes a buffer binding in a bind group layout.
type BufferBindingLayout struct {
	// Type is the buffer binding type.
	Type BufferBindingType
	// HasDynamicOffset indicates if the buffer has a dynamic offset.
	HasDynamicOffset bool
	// MinBindingSize is the minimum buffer size required (0 for no constraint).
	MinBindingSize uint64
}

// SamplerBindingLayout describes a sampler binding in a bind group layout.
type SamplerBindingLayout struct {
	// Type is the sampler binding type.
	Type SamplerBindingType
}

// TextureBindingLayout describes a texture binding in a bind group layout.
type TextureBindingLayout struct {
	// SampleType is the texture sample type.
	SampleType TextureSampleType
	// ViewDimension is the texture view dimension.
	ViewDimension TextureViewDimension
	// Multisampled indicates if the texture is multisampled.
	Multisampled bool
}

// StorageTextureAccess describes storage texture access mode.
type StorageTextureAccess uint32

const (
	// StorageTextureAccessUndefined is an undefined access mode (invalid).
	StorageTextureAccessUndefined StorageTextureAccess = 0x00000000
	// StorageTextureAccessWriteOnly allows write-only access.
	StorageTextureAccessWriteOnly StorageTextureAccess = 0x00000001
	// StorageTextureAccessReadOnly allows read-only access.
	StorageTextureAccessReadOnly StorageTextureAccess = 0x00000002
	// StorageTextureAccessReadWrite allows read-write access.
	StorageTextureAccessReadWrite StorageTextureAccess = 0x00000003
)

// String returns the storage texture access name.
func (a StorageTextureAccess) String() string {
	switch a {
	case StorageTextureAccessUndefined:
		return "Undefined"
	case StorageTextureAccessWriteOnly:
		return "WriteOnly"
	case StorageTextureAccessReadOnly:
		return "ReadOnly"
	case StorageTextureAccessReadWrite:
		return "ReadWrite"
	default:
		return "Unknown"
	}
}

// StorageTextureBindingLayout describes a storage texture binding.
type StorageTextureBindingLayout struct {
	// Access specifies the storage texture access mode.
	Access StorageTextureAccess
	// Format is the texture format.
	Format TextureFormat
	// ViewDimension is the texture view dimension.
	ViewDimension TextureViewDimension
}

// BindGroupDescriptor describes a bind group.
type BindGroupDescriptor struct {
	// Label is an optional debug label.
	Label string
	// Layout is a handle to the bind group layout (implementation-specific).
	Layout uintptr
	// Entries are the bind group entries.
	Entries []BindGroupEntry
}

// BindGroupEntry describes a single binding in a bind group.
type BindGroupEntry struct {
	// Binding is the binding number.
	Binding uint32
	// Resource is the bound resource.
	Resource BindingResource
}

// BindingResource is a resource that can be bound in a bind group.
//
// Implementations include:
//   - BufferBinding for buffer resources
//   - SamplerBinding for sampler resources
//   - TextureViewBinding for texture view resources
type BindingResource interface {
	// bindingResource is a marker method to identify binding resources.
	bindingResource()
}

// BufferBinding binds a buffer range to a binding slot.
type BufferBinding struct {
	// Buffer is a handle to the buffer (implementation-specific).
	Buffer uintptr
	// Offset is the byte offset into the buffer.
	Offset uint64
	// Size is the byte size of the binding (0 for entire buffer from offset).
	Size uint64
}

// bindingResource implements BindingResource.
func (BufferBinding) bindingResource() {}

// SamplerBinding binds a sampler to a binding slot.
type SamplerBinding struct {
	// Sampler is a handle to the sampler (implementation-specific).
	Sampler uintptr
}

// bindingResource implements BindingResource.
func (SamplerBinding) bindingResource() {}

// TextureViewBinding binds a texture view to a binding slot.
type TextureViewBinding struct {
	// TextureView is a handle to the texture view (implementation-specific).
	TextureView uintptr
}

// bindingResource implements BindingResource.
func (TextureViewBinding) bindingResource() {}

// PipelineLayoutDescriptor describes a pipeline layout.
type PipelineLayoutDescriptor struct {
	// Label is an optional debug label.
	Label string
	// BindGroupLayouts are handles to bind group layouts (implementation-specific).
	BindGroupLayouts []uintptr
	// PushConstantRanges describe push constant ranges (non-standard extension).
	PushConstantRanges []PushConstantRange
}

// PushConstantRange describes a push constant range.
//
// Note: Push constants are a non-standard extension (not in WebGPU spec).
type PushConstantRange struct {
	// Stages are the shader stages that can access this range.
	Stages ShaderStages
	// Start is the start offset in bytes.
	Start uint32
	// End is the end offset in bytes.
	End uint32
}
