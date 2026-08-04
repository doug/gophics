package gputypes

// VertexFormat describes a vertex attribute format.
//
// The format name follows the pattern: TypeSizex[Count]
// where Type is the data type, Size is the bit width, and Count is the number of components.
type VertexFormat uint32

const (
	// VertexFormatUndefined is an undefined vertex format (invalid).
	VertexFormatUndefined VertexFormat = 0x00000000
	// VertexFormatUint8x2 is two 8-bit unsigned integers.
	VertexFormatUint8x2 VertexFormat = 0x00000001
	// VertexFormatUint8x4 is four 8-bit unsigned integers.
	VertexFormatUint8x4 VertexFormat = 0x00000002
	// VertexFormatSint8x2 is two 8-bit signed integers.
	VertexFormatSint8x2 VertexFormat = 0x00000003
	// VertexFormatSint8x4 is four 8-bit signed integers.
	VertexFormatSint8x4 VertexFormat = 0x00000004
	// VertexFormatUnorm8x2 is two 8-bit normalized unsigned integers [0.0, 1.0].
	VertexFormatUnorm8x2 VertexFormat = 0x00000005
	// VertexFormatUnorm8x4 is four 8-bit normalized unsigned integers [0.0, 1.0].
	VertexFormatUnorm8x4 VertexFormat = 0x00000006
	// VertexFormatSnorm8x2 is two 8-bit normalized signed integers [-1.0, 1.0].
	VertexFormatSnorm8x2 VertexFormat = 0x00000007
	// VertexFormatSnorm8x4 is four 8-bit normalized signed integers [-1.0, 1.0].
	VertexFormatSnorm8x4 VertexFormat = 0x00000008
	// VertexFormatUint16x2 is two 16-bit unsigned integers.
	VertexFormatUint16x2 VertexFormat = 0x00000009
	// VertexFormatUint16x4 is four 16-bit unsigned integers.
	VertexFormatUint16x4 VertexFormat = 0x0000000A
	// VertexFormatSint16x2 is two 16-bit signed integers.
	VertexFormatSint16x2 VertexFormat = 0x0000000B
	// VertexFormatSint16x4 is four 16-bit signed integers.
	VertexFormatSint16x4 VertexFormat = 0x0000000C
	// VertexFormatUnorm16x2 is two 16-bit normalized unsigned integers [0.0, 1.0].
	VertexFormatUnorm16x2 VertexFormat = 0x0000000D
	// VertexFormatUnorm16x4 is four 16-bit normalized unsigned integers [0.0, 1.0].
	VertexFormatUnorm16x4 VertexFormat = 0x0000000E
	// VertexFormatSnorm16x2 is two 16-bit normalized signed integers [-1.0, 1.0].
	VertexFormatSnorm16x2 VertexFormat = 0x0000000F
	// VertexFormatSnorm16x4 is four 16-bit normalized signed integers [-1.0, 1.0].
	VertexFormatSnorm16x4 VertexFormat = 0x00000010
	// VertexFormatFloat16x2 is two 16-bit floats.
	VertexFormatFloat16x2 VertexFormat = 0x00000011
	// VertexFormatFloat16x4 is four 16-bit floats.
	VertexFormatFloat16x4 VertexFormat = 0x00000012
	// VertexFormatFloat32 is a single 32-bit float.
	VertexFormatFloat32 VertexFormat = 0x00000013
	// VertexFormatFloat32x2 is two 32-bit floats (vec2).
	VertexFormatFloat32x2 VertexFormat = 0x00000014
	// VertexFormatFloat32x3 is three 32-bit floats (vec3).
	VertexFormatFloat32x3 VertexFormat = 0x00000015
	// VertexFormatFloat32x4 is four 32-bit floats (vec4).
	VertexFormatFloat32x4 VertexFormat = 0x00000016
	// VertexFormatUint32 is a single 32-bit unsigned integer.
	VertexFormatUint32 VertexFormat = 0x00000017
	// VertexFormatUint32x2 is two 32-bit unsigned integers.
	VertexFormatUint32x2 VertexFormat = 0x00000018
	// VertexFormatUint32x3 is three 32-bit unsigned integers.
	VertexFormatUint32x3 VertexFormat = 0x00000019
	// VertexFormatUint32x4 is four 32-bit unsigned integers.
	VertexFormatUint32x4 VertexFormat = 0x0000001A
	// VertexFormatSint32 is a single 32-bit signed integer.
	VertexFormatSint32 VertexFormat = 0x0000001B
	// VertexFormatSint32x2 is two 32-bit signed integers.
	VertexFormatSint32x2 VertexFormat = 0x0000001C
	// VertexFormatSint32x3 is three 32-bit signed integers.
	VertexFormatSint32x3 VertexFormat = 0x0000001D
	// VertexFormatSint32x4 is four 32-bit signed integers.
	VertexFormatSint32x4 VertexFormat = 0x0000001E
	// VertexFormatUnorm1010102 is a packed 10-10-10-2 normalized unsigned format.
	VertexFormatUnorm1010102 VertexFormat = 0x0000001F
)

// String returns the vertex format name.
func (f VertexFormat) String() string {
	switch f {
	case VertexFormatUndefined:
		return "Undefined"
	case VertexFormatUint8x2:
		return "Uint8x2"
	case VertexFormatUint8x4:
		return "Uint8x4"
	case VertexFormatSint8x2:
		return "Sint8x2"
	case VertexFormatSint8x4:
		return "Sint8x4"
	case VertexFormatUnorm8x2:
		return "Unorm8x2"
	case VertexFormatUnorm8x4:
		return "Unorm8x4"
	case VertexFormatSnorm8x2:
		return "Snorm8x2"
	case VertexFormatSnorm8x4:
		return "Snorm8x4"
	case VertexFormatUint16x2:
		return "Uint16x2"
	case VertexFormatUint16x4:
		return "Uint16x4"
	case VertexFormatSint16x2:
		return "Sint16x2"
	case VertexFormatSint16x4:
		return "Sint16x4"
	case VertexFormatUnorm16x2:
		return "Unorm16x2"
	case VertexFormatUnorm16x4:
		return "Unorm16x4"
	case VertexFormatSnorm16x2:
		return "Snorm16x2"
	case VertexFormatSnorm16x4:
		return "Snorm16x4"
	case VertexFormatFloat16x2:
		return "Float16x2"
	case VertexFormatFloat16x4:
		return "Float16x4"
	case VertexFormatFloat32:
		return "Float32"
	case VertexFormatFloat32x2:
		return "Float32x2"
	case VertexFormatFloat32x3:
		return "Float32x3"
	case VertexFormatFloat32x4:
		return "Float32x4"
	case VertexFormatUint32:
		return "Uint32"
	case VertexFormatUint32x2:
		return "Uint32x2"
	case VertexFormatUint32x3:
		return "Uint32x3"
	case VertexFormatUint32x4:
		return "Uint32x4"
	case VertexFormatSint32:
		return "Sint32"
	case VertexFormatSint32x2:
		return "Sint32x2"
	case VertexFormatSint32x3:
		return "Sint32x3"
	case VertexFormatSint32x4:
		return "Sint32x4"
	case VertexFormatUnorm1010102:
		return "Unorm1010102"
	default:
		return "Unknown"
	}
}

// Size returns the byte size of the vertex format.
func (f VertexFormat) Size() uint64 {
	switch f {
	case VertexFormatUint8x2, VertexFormatSint8x2, VertexFormatUnorm8x2, VertexFormatSnorm8x2:
		return 2
	case VertexFormatUint8x4, VertexFormatSint8x4, VertexFormatUnorm8x4, VertexFormatSnorm8x4,
		VertexFormatUint16x2, VertexFormatSint16x2, VertexFormatUnorm16x2, VertexFormatSnorm16x2,
		VertexFormatFloat16x2, VertexFormatFloat32, VertexFormatUint32, VertexFormatSint32,
		VertexFormatUnorm1010102:
		return 4
	case VertexFormatUint16x4, VertexFormatSint16x4, VertexFormatUnorm16x4, VertexFormatSnorm16x4,
		VertexFormatFloat16x4, VertexFormatFloat32x2, VertexFormatUint32x2, VertexFormatSint32x2:
		return 8
	case VertexFormatFloat32x3, VertexFormatUint32x3, VertexFormatSint32x3:
		return 12
	case VertexFormatFloat32x4, VertexFormatUint32x4, VertexFormatSint32x4:
		return 16
	default:
		return 0
	}
}

// VertexStepMode describes how vertex data is stepped.
type VertexStepMode uint32

const (
	// VertexStepModeUndefined is an undefined step mode (invalid).
	VertexStepModeUndefined VertexStepMode = 0x00000000
	// VertexStepModeVertexBufferNotUsed indicates the buffer is not used.
	VertexStepModeVertexBufferNotUsed VertexStepMode = 0x00000001
	// VertexStepModeVertex steps once per vertex.
	VertexStepModeVertex VertexStepMode = 0x00000002
	// VertexStepModeInstance steps once per instance.
	VertexStepModeInstance VertexStepMode = 0x00000003
)

// String returns the step mode name.
func (m VertexStepMode) String() string {
	switch m {
	case VertexStepModeUndefined:
		return "Undefined"
	case VertexStepModeVertexBufferNotUsed:
		return "VertexBufferNotUsed"
	case VertexStepModeVertex:
		return "Vertex"
	case VertexStepModeInstance:
		return "Instance"
	default:
		return "Unknown"
	}
}

// VertexAttribute describes a vertex attribute in a vertex buffer layout.
type VertexAttribute struct {
	// Format is the attribute format.
	Format VertexFormat
	// Offset is the byte offset within the vertex buffer stride.
	Offset uint64
	// ShaderLocation is the @location in the shader.
	ShaderLocation uint32
}

// VertexBufferLayout describes the layout of a vertex buffer.
type VertexBufferLayout struct {
	// ArrayStride is the stride between vertices in bytes.
	ArrayStride uint64
	// StepMode describes how the buffer is stepped.
	StepMode VertexStepMode
	// Attributes are the vertex attributes.
	Attributes []VertexAttribute
}

// VertexState describes the vertex stage of a render pipeline.
type VertexState struct {
	// Module is a handle to the shader module (implementation-specific).
	Module uintptr
	// EntryPoint is the vertex shader entry point function name.
	EntryPoint string
	// Constants are pipeline-overridable constants.
	Constants map[string]float64
	// Buffers are the vertex buffer layouts.
	Buffers []VertexBufferLayout
}

// FragmentState describes the fragment stage of a render pipeline.
type FragmentState struct {
	// Module is a handle to the shader module (implementation-specific).
	Module uintptr
	// EntryPoint is the fragment shader entry point function name.
	EntryPoint string
	// Constants are pipeline-overridable constants.
	Constants map[string]float64
	// Targets are the color target states.
	Targets []ColorTargetState
}
