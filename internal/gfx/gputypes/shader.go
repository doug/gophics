package gputypes

// ShaderStage represents a shader stage.
//
// This is a bit flag type. Multiple stages can be combined with bitwise OR.
type ShaderStage uint32

const (
	// ShaderStageNone represents no shader stage.
	ShaderStageNone ShaderStage = 0x00000000
	// ShaderStageVertex is the vertex shader stage.
	ShaderStageVertex ShaderStage = 0x00000001
	// ShaderStageFragment is the fragment (pixel) shader stage.
	ShaderStageFragment ShaderStage = 0x00000002
	// ShaderStageCompute is the compute shader stage.
	ShaderStageCompute ShaderStage = 0x00000004
)

// ShaderStages is an alias for ShaderStage for clarity when used as a flag set.
type ShaderStages = ShaderStage

// Common shader stage combinations.
const (
	// ShaderStagesVertexFragment includes vertex and fragment stages.
	ShaderStagesVertexFragment = ShaderStageVertex | ShaderStageFragment
	// ShaderStagesAll includes all stages (vertex, fragment, and compute).
	ShaderStagesAll = ShaderStageVertex | ShaderStageFragment | ShaderStageCompute
)

// String returns the shader stage name(s).
func (s ShaderStage) String() string {
	if s == ShaderStageNone {
		return "None"
	}

	result := ""
	if s&ShaderStageVertex != 0 {
		result += "Vertex"
	}
	if s&ShaderStageFragment != 0 {
		if result != "" {
			result += "|"
		}
		result += "Fragment"
	}
	if s&ShaderStageCompute != 0 {
		if result != "" {
			result += "|"
		}
		result += "Compute"
	}
	if result == "" {
		return "Unknown"
	}
	return result
}

// Contains returns true if the stage set contains the given stage.
func (s ShaderStage) Contains(stage ShaderStage) bool {
	return s&stage == stage
}

// ShaderModuleDescriptor describes a shader module.
type ShaderModuleDescriptor struct {
	// Label is an optional debug label.
	Label string
	// Source is the shader source code.
	Source ShaderSource
}

// ShaderSource represents shader source code.
//
// Implementations include:
//   - ShaderSourceWGSL for WGSL source code
//   - ShaderSourceSPIRV for SPIR-V bytecode
//   - ShaderSourceGLSL for GLSL source code
type ShaderSource interface {
	// shaderSource is a marker method to identify shader sources.
	shaderSource()
}

// ShaderSourceWGSL is WGSL (WebGPU Shading Language) shader source.
type ShaderSourceWGSL struct {
	// Code is the WGSL source code.
	Code string
}

// shaderSource implements ShaderSource.
func (ShaderSourceWGSL) shaderSource() {}

// ShaderSourceSPIRV is SPIR-V bytecode shader source.
type ShaderSourceSPIRV struct {
	// Code is the SPIR-V bytecode as 32-bit words.
	Code []uint32
}

// shaderSource implements ShaderSource.
func (ShaderSourceSPIRV) shaderSource() {}

// ShaderSourceGLSL is GLSL shader source.
//
// Note: GLSL support is backend-dependent and may not be available on all platforms.
type ShaderSourceGLSL struct {
	// Code is the GLSL source code.
	Code string
	// Stage is the shader stage this GLSL code is for.
	Stage ShaderStage
	// Defines is a map of preprocessor defines.
	Defines map[string]string
}

// shaderSource implements ShaderSource.
func (ShaderSourceGLSL) shaderSource() {}

// ProgrammableStage describes a programmable shader stage in a pipeline.
type ProgrammableStage struct {
	// Module is a handle to the shader module (implementation-specific).
	Module uintptr
	// EntryPoint is the entry point function name.
	EntryPoint string
	// Constants are pipeline-overridable constants.
	Constants map[string]float64
}
