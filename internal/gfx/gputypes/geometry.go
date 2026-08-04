package gputypes

// Extent3D describes a 3D size.
//
// It is used for texture dimensions and copy operations.
// For 2D textures, DepthOrArrayLayers represents the array layer count.
// For 3D textures, it represents the depth.
type Extent3D struct {
	// Width is the size in the X dimension (must be > 0).
	Width uint32
	// Height is the size in the Y dimension (must be > 0).
	Height uint32
	// DepthOrArrayLayers is the size in Z or array layer count (must be > 0).
	DepthOrArrayLayers uint32
}

// NewExtent2D creates an Extent3D for a 2D texture with 1 layer.
func NewExtent2D(width, height uint32) Extent3D {
	return Extent3D{
		Width:              width,
		Height:             height,
		DepthOrArrayLayers: 1,
	}
}

// NewExtent3D creates an Extent3D for a 3D texture.
func NewExtent3D(width, height, depth uint32) Extent3D {
	return Extent3D{
		Width:              width,
		Height:             height,
		DepthOrArrayLayers: depth,
	}
}

// Origin3D describes a 3D origin point.
//
// It is used to specify the starting point for texture copy operations.
type Origin3D struct {
	// X is the X coordinate.
	X uint32
	// Y is the Y coordinate.
	Y uint32
	// Z is the Z coordinate (or array layer for 2D array textures).
	Z uint32
}

// OriginZero is the origin at (0, 0, 0).
var OriginZero = Origin3D{X: 0, Y: 0, Z: 0}
