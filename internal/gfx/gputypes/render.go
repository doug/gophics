package gputypes

// LoadOp describes the load operation for an attachment.
type LoadOp uint32

const (
	// LoadOpUndefined is an undefined load operation (invalid).
	LoadOpUndefined LoadOp = 0x00000000
	// LoadOpLoad loads the existing contents.
	LoadOpLoad LoadOp = 0x00000001
	// LoadOpClear clears the attachment to a specified value.
	LoadOpClear LoadOp = 0x00000002
)

// String returns the load operation name.
func (op LoadOp) String() string {
	switch op {
	case LoadOpUndefined:
		return "Undefined"
	case LoadOpLoad:
		return "Load"
	case LoadOpClear:
		return "Clear"
	default:
		return "Unknown"
	}
}

// StoreOp describes the store operation for an attachment.
type StoreOp uint32

const (
	// StoreOpUndefined is an undefined store operation (invalid).
	StoreOpUndefined StoreOp = 0x00000000
	// StoreOpStore stores the contents to memory.
	StoreOpStore StoreOp = 0x00000001
	// StoreOpDiscard discards the contents (for performance when not needed).
	StoreOpDiscard StoreOp = 0x00000002
)

// String returns the store operation name.
func (op StoreOp) String() string {
	switch op {
	case StoreOpUndefined:
		return "Undefined"
	case StoreOpStore:
		return "Store"
	case StoreOpDiscard:
		return "Discard"
	default:
		return "Unknown"
	}
}

// BlendFactor describes a blend factor for color blending.
type BlendFactor uint32

const (
	// BlendFactorUndefined is an undefined blend factor (invalid).
	BlendFactorUndefined BlendFactor = 0x00000000
	// BlendFactorZero uses 0.
	BlendFactorZero BlendFactor = 0x00000001
	// BlendFactorOne uses 1.
	BlendFactorOne BlendFactor = 0x00000002
	// BlendFactorSrc uses the source color.
	BlendFactorSrc BlendFactor = 0x00000003
	// BlendFactorOneMinusSrc uses (1 - source color).
	BlendFactorOneMinusSrc BlendFactor = 0x00000004
	// BlendFactorSrcAlpha uses the source alpha.
	BlendFactorSrcAlpha BlendFactor = 0x00000005
	// BlendFactorOneMinusSrcAlpha uses (1 - source alpha).
	BlendFactorOneMinusSrcAlpha BlendFactor = 0x00000006
	// BlendFactorDst uses the destination color.
	BlendFactorDst BlendFactor = 0x00000007
	// BlendFactorOneMinusDst uses (1 - destination color).
	BlendFactorOneMinusDst BlendFactor = 0x00000008
	// BlendFactorDstAlpha uses the destination alpha.
	BlendFactorDstAlpha BlendFactor = 0x00000009
	// BlendFactorOneMinusDstAlpha uses (1 - destination alpha).
	BlendFactorOneMinusDstAlpha BlendFactor = 0x0000000A
	// BlendFactorSrcAlphaSaturated uses min(source alpha, 1 - destination alpha).
	BlendFactorSrcAlphaSaturated BlendFactor = 0x0000000B
	// BlendFactorConstant uses the constant blend color.
	BlendFactorConstant BlendFactor = 0x0000000C
	// BlendFactorOneMinusConstant uses (1 - constant blend color).
	BlendFactorOneMinusConstant BlendFactor = 0x0000000D
)

// String returns the blend factor name.
func (f BlendFactor) String() string {
	switch f {
	case BlendFactorUndefined:
		return "Undefined"
	case BlendFactorZero:
		return "Zero"
	case BlendFactorOne:
		return "One"
	case BlendFactorSrc:
		return "Src"
	case BlendFactorOneMinusSrc:
		return "OneMinusSrc"
	case BlendFactorSrcAlpha:
		return "SrcAlpha"
	case BlendFactorOneMinusSrcAlpha:
		return "OneMinusSrcAlpha"
	case BlendFactorDst:
		return "Dst"
	case BlendFactorOneMinusDst:
		return "OneMinusDst"
	case BlendFactorDstAlpha:
		return "DstAlpha"
	case BlendFactorOneMinusDstAlpha:
		return "OneMinusDstAlpha"
	case BlendFactorSrcAlphaSaturated:
		return "SrcAlphaSaturated"
	case BlendFactorConstant:
		return "Constant"
	case BlendFactorOneMinusConstant:
		return "OneMinusConstant"
	default:
		return "Unknown"
	}
}

// BlendOperation describes a blend operation.
type BlendOperation uint32

const (
	// BlendOperationUndefined is an undefined blend operation (invalid).
	BlendOperationUndefined BlendOperation = 0x00000000
	// BlendOperationAdd computes src + dst.
	BlendOperationAdd BlendOperation = 0x00000001
	// BlendOperationSubtract computes src - dst.
	BlendOperationSubtract BlendOperation = 0x00000002
	// BlendOperationReverseSubtract computes dst - src.
	BlendOperationReverseSubtract BlendOperation = 0x00000003
	// BlendOperationMin computes min(src, dst).
	BlendOperationMin BlendOperation = 0x00000004
	// BlendOperationMax computes max(src, dst).
	BlendOperationMax BlendOperation = 0x00000005
)

// String returns the blend operation name.
func (op BlendOperation) String() string {
	switch op {
	case BlendOperationUndefined:
		return "Undefined"
	case BlendOperationAdd:
		return "Add"
	case BlendOperationSubtract:
		return "Subtract"
	case BlendOperationReverseSubtract:
		return "ReverseSubtract"
	case BlendOperationMin:
		return "Min"
	case BlendOperationMax:
		return "Max"
	default:
		return "Unknown"
	}
}

// BlendComponent describes blending for a single color component (RGB or alpha).
type BlendComponent struct {
	// SrcFactor is the source blend factor.
	SrcFactor BlendFactor
	// DstFactor is the destination blend factor.
	DstFactor BlendFactor
	// Operation is the blend operation.
	Operation BlendOperation
}

// UsesConstant returns true if this blend component uses the blend constant
// color (BlendFactorConstant or BlendFactorOneMinusConstant).
// Matches Rust wgpu-types BlendComponent::uses_constant().
func (bc BlendComponent) UsesConstant() bool {
	return bc.SrcFactor == BlendFactorConstant ||
		bc.SrcFactor == BlendFactorOneMinusConstant ||
		bc.DstFactor == BlendFactorConstant ||
		bc.DstFactor == BlendFactorOneMinusConstant
}

// BlendState describes color blending for a render target.
type BlendState struct {
	// Color describes RGB channel blending.
	Color BlendComponent
	// Alpha describes alpha channel blending.
	Alpha BlendComponent
}

// BlendStateReplace returns a blend state that replaces the destination.
func BlendStateReplace() BlendState {
	return BlendState{
		Color: BlendComponent{
			SrcFactor: BlendFactorOne,
			DstFactor: BlendFactorZero,
			Operation: BlendOperationAdd,
		},
		Alpha: BlendComponent{
			SrcFactor: BlendFactorOne,
			DstFactor: BlendFactorZero,
			Operation: BlendOperationAdd,
		},
	}
}

// BlendStateAlpha returns a standard alpha blending state.
//
// This is the most common blend state for transparent rendering.
func BlendStateAlpha() BlendState {
	return BlendState{
		Color: BlendComponent{
			SrcFactor: BlendFactorSrcAlpha,
			DstFactor: BlendFactorOneMinusSrcAlpha,
			Operation: BlendOperationAdd,
		},
		Alpha: BlendComponent{
			SrcFactor: BlendFactorOne,
			DstFactor: BlendFactorOneMinusSrcAlpha,
			Operation: BlendOperationAdd,
		},
	}
}

// BlendStatePremultiplied returns a blend state for premultiplied alpha.
func BlendStatePremultiplied() BlendState {
	return BlendState{
		Color: BlendComponent{
			SrcFactor: BlendFactorOne,
			DstFactor: BlendFactorOneMinusSrcAlpha,
			Operation: BlendOperationAdd,
		},
		Alpha: BlendComponent{
			SrcFactor: BlendFactorOne,
			DstFactor: BlendFactorOneMinusSrcAlpha,
			Operation: BlendOperationAdd,
		},
	}
}

// ColorWriteMask describes which color channels to write.
//
// This is a bit flag type.
type ColorWriteMask uint32

const (
	// ColorWriteMaskNone writes no channels.
	ColorWriteMaskNone ColorWriteMask = 0x00000000
	// ColorWriteMaskRed writes the red channel.
	ColorWriteMaskRed ColorWriteMask = 0x00000001
	// ColorWriteMaskGreen writes the green channel.
	ColorWriteMaskGreen ColorWriteMask = 0x00000002
	// ColorWriteMaskBlue writes the blue channel.
	ColorWriteMaskBlue ColorWriteMask = 0x00000004
	// ColorWriteMaskAlpha writes the alpha channel.
	ColorWriteMaskAlpha ColorWriteMask = 0x00000008
	// ColorWriteMaskAll writes all channels.
	ColorWriteMaskAll ColorWriteMask = 0x0000000F
)

// ColorTargetState describes a color target in a render pipeline.
type ColorTargetState struct {
	// Format is the texture format of the target.
	Format TextureFormat
	// Blend describes color blending (nil for no blending).
	Blend *BlendState
	// WriteMask specifies which color channels to write.
	WriteMask ColorWriteMask
}

// PrimitiveTopology describes how vertices form primitives.
//
// The zero value is [PrimitiveTopologyTriangleList], which matches the
// WebGPU spec default ( https://gpuweb.github.io/gpuweb/#enumdef-gpuprimitivetopology ).
// This means `PrimitiveState{}` is a fully valid WebGPU-spec-default primitive
// assembly configuration — no normalization pass is needed anywhere.
//
// This is intentionally better than Rust wgpu's approach: Rust relies on
// `#[derive(Default)] #[default]` annotations to achieve the same result.
// We get it for free from Go's zero initialization rules.
type PrimitiveTopology uint32

const (
	// PrimitiveTopologyTriangleList renders groups of 3 vertices as triangles.
	// This is the zero value and WebGPU spec default.
	PrimitiveTopologyTriangleList PrimitiveTopology = 0
	// PrimitiveTopologyPointList renders each vertex as a point.
	PrimitiveTopologyPointList PrimitiveTopology = 1
	// PrimitiveTopologyLineList renders pairs of vertices as lines.
	PrimitiveTopologyLineList PrimitiveTopology = 2
	// PrimitiveTopologyLineStrip renders connected lines.
	PrimitiveTopologyLineStrip PrimitiveTopology = 3
	// PrimitiveTopologyTriangleStrip renders connected triangles.
	PrimitiveTopologyTriangleStrip PrimitiveTopology = 4
)

// String returns the topology name.
func (t PrimitiveTopology) String() string {
	switch t {
	case PrimitiveTopologyTriangleList:
		return "TriangleList"
	case PrimitiveTopologyPointList:
		return "PointList"
	case PrimitiveTopologyLineList:
		return "LineList"
	case PrimitiveTopologyLineStrip:
		return "LineStrip"
	case PrimitiveTopologyTriangleStrip:
		return "TriangleStrip"
	default:
		return "Unknown"
	}
}

// FrontFace describes the front face winding order.
//
// The zero value is [FrontFaceCCW], matching the WebGPU spec default
// ( https://gpuweb.github.io/gpuweb/#enumdef-gpufrontface ).
type FrontFace uint32

const (
	// FrontFaceCCW treats counter-clockwise vertices as front-facing.
	// This is the zero value and WebGPU spec default.
	FrontFaceCCW FrontFace = 0
	// FrontFaceCW treats clockwise vertices as front-facing.
	FrontFaceCW FrontFace = 1
)

// String returns the front face name.
func (f FrontFace) String() string {
	switch f {
	case FrontFaceCCW:
		return "CCW"
	case FrontFaceCW:
		return "CW"
	default:
		return "Unknown"
	}
}

// CullMode describes which faces to cull.
//
// The zero value is [CullModeNone], matching the WebGPU spec default
// ( https://gpuweb.github.io/gpuweb/#enumdef-gpucullmode ).
type CullMode uint32

const (
	// CullModeNone culls no faces. This is the zero value and WebGPU spec default.
	CullModeNone CullMode = 0
	// CullModeFront culls front faces.
	CullModeFront CullMode = 1
	// CullModeBack culls back faces.
	CullModeBack CullMode = 2
)

// String returns the cull mode name.
func (m CullMode) String() string {
	switch m {
	case CullModeNone:
		return "None"
	case CullModeFront:
		return "Front"
	case CullModeBack:
		return "Back"
	default:
		return "Unknown"
	}
}

// PrimitiveState describes primitive assembly state.
type PrimitiveState struct {
	// Topology is the primitive topology.
	Topology PrimitiveTopology
	// StripIndexFormat is the index format for strip topologies (nil for non-strip).
	StripIndexFormat *IndexFormat
	// FrontFace is the front face winding order.
	FrontFace FrontFace
	// CullMode specifies which faces to cull.
	CullMode CullMode
	// UnclippedDepth enables unclipped depth (requires feature).
	UnclippedDepth bool
}

// DefaultPrimitiveState returns a primitive state with WebGPU spec defaults.
//
// This is equivalent to `PrimitiveState{}` — the zero value of PrimitiveState
// is already the WebGPU spec default because all enum fields
// ([PrimitiveTopology], [FrontFace], [CullMode]) have their zero value
// defined as the spec default. This function is provided for explicit
// call-site documentation and parity with other Default*State helpers.
//
// See Rust wgpu's `PrimitiveState::default()` for the equivalent pattern.
func DefaultPrimitiveState() PrimitiveState {
	return PrimitiveState{}
}

// MultisampleState describes multisampling state.
type MultisampleState struct {
	// Count is the number of samples per pixel (1, 2, 4, 8, or 16).
	Count uint32
	// Mask is the sample mask (all bits set = all samples).
	Mask uint64
	// AlphaToCoverageEnabled enables alpha-to-coverage.
	AlphaToCoverageEnabled bool
}

// DefaultMultisampleState returns a multisample state with no multisampling.
func DefaultMultisampleState() MultisampleState {
	return MultisampleState{
		Count:                  1,
		Mask:                   0xFFFFFFFF,
		AlphaToCoverageEnabled: false,
	}
}

// StencilOperation describes a stencil operation.
type StencilOperation uint32

const (
	// StencilOperationUndefined is an undefined stencil operation (invalid).
	StencilOperationUndefined StencilOperation = 0x00000000
	// StencilOperationKeep keeps the current value.
	StencilOperationKeep StencilOperation = 0x00000001
	// StencilOperationZero sets to zero.
	StencilOperationZero StencilOperation = 0x00000002
	// StencilOperationReplace replaces with reference value.
	StencilOperationReplace StencilOperation = 0x00000003
	// StencilOperationInvert inverts all bits.
	StencilOperationInvert StencilOperation = 0x00000004
	// StencilOperationIncrementClamp increments and clamps to maximum.
	StencilOperationIncrementClamp StencilOperation = 0x00000005
	// StencilOperationDecrementClamp decrements and clamps to zero.
	StencilOperationDecrementClamp StencilOperation = 0x00000006
	// StencilOperationIncrementWrap increments and wraps to zero.
	StencilOperationIncrementWrap StencilOperation = 0x00000007
	// StencilOperationDecrementWrap decrements and wraps to maximum.
	StencilOperationDecrementWrap StencilOperation = 0x00000008
)

// String returns the stencil operation name.
func (op StencilOperation) String() string {
	switch op {
	case StencilOperationUndefined:
		return "Undefined"
	case StencilOperationKeep:
		return "Keep"
	case StencilOperationZero:
		return "Zero"
	case StencilOperationReplace:
		return "Replace"
	case StencilOperationInvert:
		return "Invert"
	case StencilOperationIncrementClamp:
		return "IncrementClamp"
	case StencilOperationDecrementClamp:
		return "DecrementClamp"
	case StencilOperationIncrementWrap:
		return "IncrementWrap"
	case StencilOperationDecrementWrap:
		return "DecrementWrap"
	default:
		return "Unknown"
	}
}

// StencilFaceState describes stencil operations for a face.
type StencilFaceState struct {
	// Compare is the comparison function.
	Compare CompareFunction
	// FailOp is the operation on stencil test failure.
	FailOp StencilOperation
	// DepthFailOp is the operation on depth test failure.
	DepthFailOp StencilOperation
	// PassOp is the operation on both tests passing.
	PassOp StencilOperation
}

// DefaultStencilFaceState returns a stencil face state that always passes and keeps.
func DefaultStencilFaceState() StencilFaceState {
	return StencilFaceState{
		Compare:     CompareFunctionAlways,
		FailOp:      StencilOperationKeep,
		DepthFailOp: StencilOperationKeep,
		PassOp:      StencilOperationKeep,
	}
}

// DepthStencilState describes depth and stencil state.
type DepthStencilState struct {
	// Format is the depth/stencil texture format.
	Format TextureFormat
	// DepthWriteEnabled enables depth writing.
	DepthWriteEnabled bool
	// DepthCompare is the depth comparison function.
	DepthCompare CompareFunction
	// StencilFront describes front face stencil state.
	StencilFront StencilFaceState
	// StencilBack describes back face stencil state.
	StencilBack StencilFaceState
	// StencilReadMask is the mask for stencil reads.
	StencilReadMask uint32
	// StencilWriteMask is the mask for stencil writes.
	StencilWriteMask uint32
	// DepthBias is the constant depth bias.
	DepthBias int32
	// DepthBiasSlopeScale is the slope-based depth bias.
	DepthBiasSlopeScale float32
	// DepthBiasClamp is the maximum depth bias.
	DepthBiasClamp float32
}

// DefaultDepthStencilState returns a depth-stencil state with depth testing enabled.
func DefaultDepthStencilState(format TextureFormat) DepthStencilState {
	return DepthStencilState{
		Format:              format,
		DepthWriteEnabled:   true,
		DepthCompare:        CompareFunctionLess,
		StencilFront:        DefaultStencilFaceState(),
		StencilBack:         DefaultStencilFaceState(),
		StencilReadMask:     0xFFFFFFFF,
		StencilWriteMask:    0xFFFFFFFF,
		DepthBias:           0,
		DepthBiasSlopeScale: 0,
		DepthBiasClamp:      0,
	}
}

// RenderPassColorAttachment describes a color attachment for a render pass.
type RenderPassColorAttachment struct {
	// View is the texture view to render to (implementation-specific handle).
	View uintptr
	// ResolveTarget is the texture view for multisample resolve (0 if none).
	ResolveTarget uintptr
	// LoadOp describes how to load the attachment.
	LoadOp LoadOp
	// StoreOp describes how to store the attachment.
	StoreOp StoreOp
	// ClearValue is the clear color (used when LoadOp is Clear).
	ClearValue Color
}

// RenderPassDepthStencilAttachment describes a depth-stencil attachment.
type RenderPassDepthStencilAttachment struct {
	// View is the texture view (implementation-specific handle).
	View uintptr
	// DepthLoadOp describes how to load depth.
	DepthLoadOp LoadOp
	// DepthStoreOp describes how to store depth.
	DepthStoreOp StoreOp
	// DepthClearValue is the clear depth value.
	DepthClearValue float32
	// DepthReadOnly indicates if depth is read-only.
	DepthReadOnly bool
	// StencilLoadOp describes how to load stencil.
	StencilLoadOp LoadOp
	// StencilStoreOp describes how to store stencil.
	StencilStoreOp StoreOp
	// StencilClearValue is the clear stencil value.
	StencilClearValue uint32
	// StencilReadOnly indicates if stencil is read-only.
	StencilReadOnly bool
}

// RenderPassDescriptor describes a render pass.
type RenderPassDescriptor struct {
	// Label is an optional debug label.
	Label string
	// ColorAttachments are the color attachments.
	ColorAttachments []RenderPassColorAttachment
	// DepthStencilAttachment is the depth-stencil attachment (nil if none).
	DepthStencilAttachment *RenderPassDepthStencilAttachment
}
