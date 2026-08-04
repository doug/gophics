package gg

// paintBlendMode represents a per-operation blend mode stored in Paint.
//
// Values mirror internal/blend.BlendMode exactly (same uint8 values).
// Defined here to avoid an import cycle (internal/blend imports gg for RGBA).
//
// The 29 modes follow the W3C Compositing and Blending Level 1 specification:
//   - 14 Porter-Duff compositing operators (0-13)
//   - 11 advanced separable modes (14-24)
//   - 4 non-separable HSL modes (25-28)
type paintBlendMode = uint8

// paintBlendMode constants matching internal/blend.BlendMode values.
const (
	// Porter-Duff modes.
	blendModeClear           paintBlendMode = 0
	blendModeSource          paintBlendMode = 1
	blendModeDestination     paintBlendMode = 2
	blendModeSourceOver      paintBlendMode = 3 // default
	blendModeDestinationOver paintBlendMode = 4
	blendModeSourceIn        paintBlendMode = 5
	blendModeDestinationIn   paintBlendMode = 6
	blendModeSourceOut       paintBlendMode = 7
	blendModeDestinationOut  paintBlendMode = 8
	blendModeSourceAtop      paintBlendMode = 9
	blendModeDestinationAtop paintBlendMode = 10
	blendModeXor             paintBlendMode = 11
	blendModePlus            paintBlendMode = 12
	blendModeModulate        paintBlendMode = 13

	// Advanced separable modes.
	blendModeMultiply   paintBlendMode = 14
	blendModeScreen     paintBlendMode = 15
	blendModeOverlay    paintBlendMode = 16
	blendModeDarken     paintBlendMode = 17
	blendModeLighten    paintBlendMode = 18
	blendModeColorDodge paintBlendMode = 19
	blendModeColorBurn  paintBlendMode = 20
	blendModeHardLight  paintBlendMode = 21
	blendModeSoftLight  paintBlendMode = 22
	blendModeDifference paintBlendMode = 23
	blendModeExclusion  paintBlendMode = 24

	// Non-separable HSL modes.
	blendModeHue        paintBlendMode = 25
	blendModeSaturation paintBlendMode = 26
	blendModeColor      paintBlendMode = 27
	blendModeLuminosity paintBlendMode = 28
)

// blendFunc is the per-pixel blend function signature.
// All values are premultiplied alpha, 0-255.
type blendFunc func(sr, sg, sb, sa, dr, dg, db, da byte) (r, g, b, a byte)

// blendFuncTable is a lookup table indexed by paintBlendMode (0-28).
// Initialized once; accessed read-only at runtime. Using an array instead of
// a switch reduces cyclomatic complexity and provides O(1) lookup.
var blendFuncTable = [29]blendFunc{
	blendModeClear:           blendClearFn,
	blendModeSource:          blendSourceFn,
	blendModeDestination:     blendDestinationFn,
	blendModeSourceOver:      blendSourceOverFn,
	blendModeDestinationOver: blendDestinationOverFn,
	blendModeSourceIn:        blendSourceInFn,
	blendModeDestinationIn:   blendDestinationInFn,
	blendModeSourceOut:       blendSourceOutFn,
	blendModeDestinationOut:  blendDestinationOutFn,
	blendModeSourceAtop:      blendSourceAtopFn,
	blendModeDestinationAtop: blendDestinationAtopFn,
	blendModeXor:             blendXorFn,
	blendModePlus:            blendPlusFn,
	blendModeModulate:        blendModulateFn,
	blendModeMultiply:        blendMultiplyFn,
	blendModeScreen:          blendScreenFn,
	blendModeOverlay:         blendOverlayFn,
	blendModeDarken:          blendDarkenFn,
	blendModeLighten:         blendLightenFn,
	blendModeColorDodge:      blendColorDodgeFn,
	blendModeColorBurn:       blendColorBurnFn,
	blendModeHardLight:       blendHardLightFn,
	blendModeSoftLight:       blendSoftLightFn,
	blendModeDifference:      blendDifferenceFn,
	blendModeExclusion:       blendExclusionFn,
	blendModeHue:             blendHueFn,
	blendModeSaturation:      blendSaturationFn,
	blendModeColor:           blendColorFn,
	blendModeLuminosity:      blendLuminosityFn,
}

// getBlendFunc returns the blend function for the given mode.
// Returns the SourceOver function for unknown modes.
// This lookup is called ONCE per fill/stroke operation (not per pixel).
func getBlendFunc(mode paintBlendMode) blendFunc {
	if int(mode) < len(blendFuncTable) {
		return blendFuncTable[mode]
	}
	return blendSourceOverFn
}

// lookupBlendFuncForPaint looks up the blend function for a paint's blend mode.
// Returns nil when the mode is SourceOver (callers should use the fast path).
// This avoids function pointer overhead for the common case.
func lookupBlendFuncForPaint(paint *Paint) blendFunc {
	if paint.blendMode == blendModeSourceOver {
		return nil
	}
	return getBlendFunc(paint.blendMode)
}
