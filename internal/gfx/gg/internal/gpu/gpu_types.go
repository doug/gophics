//go:build !nogpu

package gpu

import (
	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gpucontext"
)

// GPUTextureDrawCommand represents a GPU-to-GPU texture compositing command.
// Unlike ImageDrawCommand (CPU pixel upload), this draws a pre-existing GPU
// texture view directly — zero CPU readback, zero re-upload.
// Follows the Skia GrSurfaceProxyView direct-bind pattern.
type GPUTextureDrawCommand struct {
	View           gpucontext.TextureView // type-safe, asserted to *wgpu.TextureView internally
	DstX, DstY     float32
	DstW, DstH     float32
	Opacity        float32
	ViewportWidth  uint32
	ViewportHeight uint32
	// FROST-BLUR (VULKAN-VERIFY): the frosted-glass backdrop blur reuses this
	// composite draw (no separate pipeline) via the textured-quad shader. Both
	// are 0 for every normal image/sprite/layer draw, which then takes the
	// shader's plain single-sample path unchanged. See appendBackdropBlur and
	// textured_quad.wgsl.
	Saturation           float32 // >0 boosts saturation of the sampled color (glass vibrancy); 0 = leave as-is
	BlurStepX, BlurStepY float32 // per-tap UV offset for a separable Gaussian pass; 0/0 = no blur
}

// extractConvexPolygon checks if a path is a single closed contour made entirely
// of line segments that form a convex polygon. If so, it returns the polygon
// points. If the path contains curves, multiple subpaths, or is not convex,
// it returns nil, false.
//
// This enables Tier 2a (convex fast-path) for paths like triangles, pentagons,
// and other convex shapes that don't need stencil-then-cover.
func extractConvexPolygon(path *gg.Path) ([]gg.Point, bool) {
	if path.NumVerbs() < 3 {
		return nil, false
	}

	var points []gg.Point
	moveCount := 0
	closed := false
	hasCurves := false

	path.Iterate(func(verb gg.PathVerb, coords []float64) {
		if hasCurves {
			return
		}
		switch verb {
		case gg.MoveTo:
			moveCount++
			if moveCount > 1 {
				hasCurves = true // abuse flag for early exit
				return
			}
			points = append(points, gg.Pt(coords[0], coords[1]))
		case gg.LineTo:
			points = append(points, gg.Pt(coords[0], coords[1]))
		case gg.QuadTo, gg.CubicTo:
			hasCurves = true
		case gg.Close:
			closed = true
		}
	})

	if hasCurves || !closed || moveCount != 1 || len(points) < 3 {
		return nil, false
	}

	if !IsConvex(points) {
		return nil, false
	}

	return points, true
}

// convertPathVerbsToStroke and strokeResultToPath are also shared utilities
// used by both SDFAccelerator and GPURenderContext.
// They remain in their original location (vello_accelerator.go or similar).
