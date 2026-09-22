// Package image provides image buffer management for gogpu/gg.
package image

import (
	"encoding/binary"
	"math"
)

// Rect represents a rectangular region in pixel coordinates.
type Rect struct {
	X, Y          int // Top-left corner
	Width, Height int // Dimensions
}

// BlendMode defines how source pixels are blended with destination pixels.
type BlendMode uint8

const (
	// BlendNormal performs standard alpha blending (source over destination).
	BlendNormal BlendMode = iota

	// BlendMultiply multiplies source and destination colors.
	// Result is always darker or equal. Formula: dst * src
	BlendMultiply

	// BlendScreen performs inverse multiply for lighter results.
	// Formula: 1 - (1-dst) * (1-src)
	BlendScreen

	// BlendOverlay combines multiply and screen based on destination brightness.
	// Dark areas are multiplied, bright areas are screened.
	BlendOverlay
)

const unknownBlendMode = "Unknown"

// String returns a string representation of the blend mode.
func (b BlendMode) String() string {
	switch b {
	case BlendNormal:
		return "Normal"
	case BlendMultiply:
		return "Multiply"
	case BlendScreen:
		return "Screen"
	case BlendOverlay:
		return "Overlay"
	default:
		return unknownBlendMode
	}
}

// DrawParams specifies parameters for the DrawImage operation.
type DrawParams struct {
	// SrcRect defines the source rectangle to sample from.
	// If nil, the entire source image is used.
	SrcRect *Rect

	// DstRect defines the destination rectangle to draw into.
	DstRect Rect

	// Transform is an optional affine transformation applied to source coordinates.
	// If nil, identity transform is used.
	Transform *Affine

	// Interp specifies the interpolation mode for sampling.
	Interp InterpolationMode

	// Opacity controls the overall transparency of the source image (0.0 to 1.0).
	// 1.0 means fully opaque, 0.0 means fully transparent.
	Opacity float64

	// BlendMode specifies how to blend source and destination pixels.
	BlendMode BlendMode
}

// DrawImage draws the source image onto the destination image using the specified parameters.
//
// The operation performs the following steps:
//  1. For each pixel in the destination rectangle
//  2. Apply inverse transformation to find source coordinates
//  3. Sample source image using specified interpolation
//  4. Apply opacity to the sampled color
//  5. Blend with destination using specified blend mode
//
// The destination image is modified in place.
func DrawImage(dst, src *ImageBuf, params DrawParams) {
	// Use entire source if no source rect specified
	srcRect := params.SrcRect
	if srcRect == nil {
		w, h := src.Bounds()
		srcRect = &Rect{X: 0, Y: 0, Width: w, Height: h}
	}

	// Use identity transform if none specified
	transform := params.Transform
	if transform == nil {
		identity := Identity()
		transform = &identity
	}

	// Compute inverse transform for mapping dst -> src
	invTransform, ok := transform.Invert()
	if !ok {
		// Singular matrix, cannot draw
		return
	}

	// Clamp opacity to valid range
	opacity := math.Max(0.0, math.Min(1.0, params.Opacity))

	// Get destination bounds
	dstWidth, dstHeight := dst.Bounds()

	// Clamp destination rectangle to image bounds
	dstRect := params.DstRect
	if dstRect.X < 0 {
		dstRect.Width += dstRect.X
		dstRect.X = 0
	}
	if dstRect.Y < 0 {
		dstRect.Height += dstRect.Y
		dstRect.Y = 0
	}
	if dstRect.X+dstRect.Width > dstWidth {
		dstRect.Width = dstWidth - dstRect.X
	}
	if dstRect.Y+dstRect.Height > dstHeight {
		dstRect.Height = dstHeight - dstRect.Y
	}

	// Nothing to draw if clamped rectangle is empty
	if dstRect.Width <= 0 || dstRect.Height <= 0 {
		return
	}

	// Unscaled, untransformed, nearest-sampled draw of a matching-size
	// source: walk the raw rows instead of inverse-mapping and sampling every
	// pixel. Layer compositing (PopLayer) is this case at full-surface size
	// every time. Nearest only: the bilinear and bicubic samplers reach the
	// texel through a float round trip ((dx+0.5)/w*w-0.5) that can land a
	// hair below the integer and truncate one level low, so they are not
	// bit-identical to a raw copy and keep the general path.
	if params.DstRect == dstRect && transform.IsIdentity() && params.Interp == InterpNearest &&
		canDrawUnscaled(dst, src, *srcRect, dstRect) {
		drawImageUnscaled(dst, src, *srcRect, dstRect, opacity, params.BlendMode)
		return
	}

	drawImageTransformed(dst, src, *srcRect, dstRect, invTransform, opacity, params.Interp, params.BlendMode)
}

// canDrawUnscaled reports whether drawImageUnscaled may take dst/src: the
// rectangles are the same size and inside their images, and both buffers
// share a 4-byte-per-pixel layout with alpha in the last byte so a raw
// byte read is what GetRGBA would return (channel order does not matter:
// every blend mode here treats the three colour channels alike).
func canDrawUnscaled(dst, src *ImageBuf, srcRect, dstRect Rect) bool {
	if src.format != dst.format || src.bpp != 4 || !src.format.HasAlpha() {
		return false
	}
	if srcRect.Width != dstRect.Width || srcRect.Height != dstRect.Height {
		return false
	}
	srcW, srcH := src.Bounds()
	return srcRect.X >= 0 && srcRect.Y >= 0 &&
		srcRect.X+srcRect.Width <= srcW && srcRect.Y+srcRect.Height <= srcH
}

// drawImageUnscaled blends srcRect of src onto dstRect of dst pixel for
// pixel. It produces exactly what drawImageTransformed produces for an
// identity transform with nearest sampling (same blend arithmetic on the
// same bytes), only without the per-pixel transform, sampling, and accessor
// overhead. Pixels with zero
// source alpha are skipped, as blend leaves the destination unchanged for
// them — a layer is mostly transparent, so this is most of the work.
func drawImageUnscaled(dst, src *ImageBuf, srcRect, dstRect Rect, opacity float64, mode BlendMode) {
	if opacity <= 0 {
		return // every source alpha scales to 0: blend returns dst unchanged
	}
	rowBytes := dstRect.Width * 4
	// Alpha bytes of two adjacent pixels, viewed as one little-endian word.
	const alphaPair = 0xFF000000FF000000
	for y := 0; y < dstRect.Height; y++ {
		so := (srcRect.Y+y)*src.stride + srcRect.X*4
		do := (dstRect.Y+y)*dst.stride + dstRect.X*4
		s := src.data[so : so+rowBytes : so+rowBytes]
		d := dst.data[do : do+rowBytes : do+rowBytes]
		for x := 0; x+3 < len(s); x += 4 {
			// Step over transparent runs two pixels at a time: on a
			// mostly-empty layer the scan, not the blend, is the cost.
			for x+7 < len(s) && binary.LittleEndian.Uint64(s[x:x+8])&alphaPair == 0 {
				x += 8
			}
			if x+3 >= len(s) {
				break
			}
			srcA := s[x+3]
			if srcA == 0 {
				continue
			}
			if opacity < 1.0 {
				srcA = uint8(float64(srcA) * opacity)
			}
			d[x], d[x+1], d[x+2], d[x+3] = blend(s[x], s[x+1], s[x+2], srcA, d[x], d[x+1], d[x+2], d[x+3], mode)
		}
	}
}

// drawImageTransformed is the general path: inverse-map every destination
// pixel into source space and sample it.
func drawImageTransformed(dst, src *ImageBuf, srcRect, dstRect Rect, invTransform Affine, opacity float64, interp InterpolationMode, mode BlendMode) {
	// Draw each pixel in the destination rectangle
	for dy := 0; dy < dstRect.Height; dy++ {
		for dx := 0; dx < dstRect.Width; dx++ {
			// Destination pixel coordinates (absolute in destination image)
			dstX := dstRect.X + dx
			dstY := dstRect.Y + dy

			// Normalized position within destination rectangle [0, 1]
			// Add 0.5 to sample from pixel centers
			u := (float64(dx) + 0.5) / float64(dstRect.Width)
			v := (float64(dy) + 0.5) / float64(dstRect.Height)

			// Apply inverse transform to find where this maps in source space
			// The transform is meant to map from destination rect coords to source rect coords
			srcRelX, srcRelY := invTransform.TransformPoint(u*float64(dstRect.Width), v*float64(dstRect.Height))

			// Map to source rect space
			srcX := float64(srcRect.X) + srcRelX
			srcY := float64(srcRect.Y) + srcRelY

			// Check if we're outside the source rectangle
			if srcX < float64(srcRect.X) || srcX > float64(srcRect.X+srcRect.Width) ||
				srcY < float64(srcRect.Y) || srcY > float64(srcRect.Y+srcRect.Height) {
				continue
			}

			// Convert to normalized coordinates for the entire source image [0, 1]
			srcWidth, srcHeight := src.Bounds()
			sampleU := srcX / float64(srcWidth)
			sampleV := srcY / float64(srcHeight)

			// Sample source image
			srcR, srcG, srcB, srcA := Sample(src, sampleU, sampleV, interp)

			// Apply opacity
			if opacity < 1.0 {
				srcA = uint8(float64(srcA) * opacity)
			}

			// Get destination pixel
			dstR, dstG, dstB, dstA := dst.GetRGBA(dstX, dstY)

			// Blend and write result
			r, g, b, a := blend(srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA, mode)
			_ = dst.SetRGBA(dstX, dstY, r, g, b, a)
		}
	}
}

// blend blends source and destination colors using the specified blend mode.
func blend(srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA uint8, mode BlendMode) (r, g, b, a byte) {
	if srcA == 0 {
		// Fully transparent source, return destination unchanged
		return dstR, dstG, dstB, dstA
	}

	if mode == BlendNormal {
		// Standard alpha blending (source over destination)
		return blendNormal(srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA)
	}

	// For other blend modes, first blend the colors, then apply alpha
	var blendedR, blendedG, blendedB uint8

	switch mode {
	case BlendMultiply:
		blendedR, blendedG, blendedB = blendMultiply(srcR, srcG, srcB, dstR, dstG, dstB)
	case BlendScreen:
		blendedR, blendedG, blendedB = blendScreen(srcR, srcG, srcB, dstR, dstG, dstB)
	case BlendOverlay:
		blendedR, blendedG, blendedB = blendOverlay(srcR, srcG, srcB, dstR, dstG, dstB)
	default:
		blendedR, blendedG, blendedB = srcR, srcG, srcB
	}

	// Apply alpha blending to the blended result
	return blendNormal(blendedR, blendedG, blendedB, srcA, dstR, dstG, dstB, dstA)
}

// blendNormal performs standard alpha blending (source over destination).
func blendNormal(srcR, srcG, srcB, srcA, dstR, dstG, dstB, dstA uint8) (r, g, b, a byte) {
	if srcA == 255 {
		// Fully opaque source, just return source
		return srcR, srcG, srcB, 255
	}

	if dstA == 0 {
		// Transparent destination, just return source
		return srcR, srcG, srcB, srcA
	}

	// Porter-Duff "source over" formula
	// out_a = src_a + dst_a * (1 - src_a)
	// out_c = (src_c * src_a + dst_c * dst_a * (1 - src_a)) / out_a

	srcAlpha := float64(srcA) / 255.0
	dstAlpha := float64(dstA) / 255.0

	outAlpha := srcAlpha + dstAlpha*(1-srcAlpha)

	if outAlpha == 0 {
		return 0, 0, 0, 0
	}

	r = uint8((float64(srcR)*srcAlpha + float64(dstR)*dstAlpha*(1-srcAlpha)) / outAlpha)
	g = uint8((float64(srcG)*srcAlpha + float64(dstG)*dstAlpha*(1-srcAlpha)) / outAlpha)
	b = uint8((float64(srcB)*srcAlpha + float64(dstB)*dstAlpha*(1-srcAlpha)) / outAlpha)
	a = uint8(outAlpha * 255.0)

	return r, g, b, a
}

// blendMultiply multiplies source and destination colors.
func blendMultiply(srcR, srcG, srcB, dstR, dstG, dstB uint8) (r, g, b byte) {
	r = uint8((int(srcR) * int(dstR)) / 255)
	g = uint8((int(srcG) * int(dstG)) / 255)
	b = uint8((int(srcB) * int(dstB)) / 255)
	return r, g, b
}

// blendScreen performs screen blending for lighter results.
func blendScreen(srcR, srcG, srcB, dstR, dstG, dstB uint8) (r, g, b byte) {
	// Formula: 1 - (1-src) * (1-dst) = src + dst - src*dst
	r = uint8(255 - (255-int(srcR))*(255-int(dstR))/255)
	g = uint8(255 - (255-int(srcG))*(255-int(dstG))/255)
	b = uint8(255 - (255-int(srcB))*(255-int(dstB))/255)
	return r, g, b
}

// blendOverlay combines multiply and screen based on destination brightness.
func blendOverlay(srcR, srcG, srcB, dstR, dstG, dstB uint8) (r, g, b byte) {
	r = overlayChannel(srcR, dstR)
	g = overlayChannel(srcG, dstG)
	b = overlayChannel(srcB, dstB)
	return r, g, b
}

// overlayChannel applies overlay blending to a single channel.
func overlayChannel(src, dst uint8) uint8 {
	// If dst < 0.5: 2 * src * dst
	// Else: 1 - 2 * (1-src) * (1-dst)
	if dst < 128 {
		return uint8((2 * int(src) * int(dst)) / 255)
	}
	return uint8(255 - (2*(255-int(src))*(255-int(dst)))/255)
}
