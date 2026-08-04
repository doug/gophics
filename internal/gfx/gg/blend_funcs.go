package gg

// Per-pixel blend function implementations for per-operation blending.
//
// These implement the same algorithms as internal/blend (Porter-Duff, separable,
// non-separable HSL) but live in the gg package to avoid the import cycle
// (internal/blend imports gg for gg.RGBA).
//
// All functions operate on premultiplied alpha byte values [0,255].
//
// References:
//   - W3C Compositing and Blending Level 1: https://www.w3.org/TR/compositing-1/
//   - Porter-Duff: "Compositing Digital Images" (1984)

import "math"

// --- Math utilities ---

// mulDiv255PD multiplies two bytes and divides by 255 using fast approximation.
// (a * b + 255) >> 8, same as internal/blend.mulDiv255.
func mulDiv255PD(a, b byte) byte {
	return byte((uint16(a)*uint16(b) + 255) >> 8)
}

// addDiv255PD adds two byte values with clamping to 255.
func addDiv255PD(a, b byte) byte {
	sum := uint16(a) + uint16(b)
	if sum > 255 {
		return 255
	}
	return byte(sum)
}

// --- Porter-Duff modes ---

func blendClearFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return 0, 0, 0, 0
}

func blendSourceFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return sr, sg, sb, sa
}

func blendDestinationFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return dr, dg, db, da
}

// blendSourceOverFn: S + D * (1 - Sa)
func blendSourceOverFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invSa := 255 - sa
	return addDiv255PD(sr, mulDiv255PD(dr, invSa)),
		addDiv255PD(sg, mulDiv255PD(dg, invSa)),
		addDiv255PD(sb, mulDiv255PD(db, invSa)),
		addDiv255PD(sa, mulDiv255PD(da, invSa))
}

// blendDestinationOverFn: S * (1 - Da) + D
func blendDestinationOverFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invDa := 255 - da
	return addDiv255PD(mulDiv255PD(sr, invDa), dr),
		addDiv255PD(mulDiv255PD(sg, invDa), dg),
		addDiv255PD(mulDiv255PD(sb, invDa), db),
		addDiv255PD(mulDiv255PD(sa, invDa), da)
}

func blendSourceInFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return mulDiv255PD(sr, da), mulDiv255PD(sg, da), mulDiv255PD(sb, da), mulDiv255PD(sa, da)
}

func blendDestinationInFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return mulDiv255PD(dr, sa), mulDiv255PD(dg, sa), mulDiv255PD(db, sa), mulDiv255PD(da, sa)
}

func blendSourceOutFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invDa := 255 - da
	return mulDiv255PD(sr, invDa), mulDiv255PD(sg, invDa), mulDiv255PD(sb, invDa), mulDiv255PD(sa, invDa)
}

func blendDestinationOutFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invSa := 255 - sa
	return mulDiv255PD(dr, invSa), mulDiv255PD(dg, invSa), mulDiv255PD(db, invSa), mulDiv255PD(da, invSa)
}

func blendSourceAtopFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invSa := 255 - sa
	return addDiv255PD(mulDiv255PD(sr, da), mulDiv255PD(dr, invSa)),
		addDiv255PD(mulDiv255PD(sg, da), mulDiv255PD(dg, invSa)),
		addDiv255PD(mulDiv255PD(sb, da), mulDiv255PD(db, invSa)),
		da
}

func blendDestinationAtopFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invDa := 255 - da
	return addDiv255PD(mulDiv255PD(sr, invDa), mulDiv255PD(dr, sa)),
		addDiv255PD(mulDiv255PD(sg, invDa), mulDiv255PD(dg, sa)),
		addDiv255PD(mulDiv255PD(sb, invDa), mulDiv255PD(db, sa)),
		sa
}

func blendXorFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	invDa := 255 - da
	invSa := 255 - sa
	return addDiv255PD(mulDiv255PD(sr, invDa), mulDiv255PD(dr, invSa)),
		addDiv255PD(mulDiv255PD(sg, invDa), mulDiv255PD(dg, invSa)),
		addDiv255PD(mulDiv255PD(sb, invDa), mulDiv255PD(db, invSa)),
		addDiv255PD(mulDiv255PD(sa, invDa), mulDiv255PD(da, invSa))
}

func blendPlusFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return addDiv255PD(sr, dr), addDiv255PD(sg, dg), addDiv255PD(sb, db), addDiv255PD(sa, da)
}

func blendModulateFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return mulDiv255PD(sr, dr), mulDiv255PD(sg, dg), mulDiv255PD(sb, db), mulDiv255PD(sa, da)
}

// --- Advanced separable modes ---

// separableBlendPD applies a per-channel blend function to premultiplied inputs.
// Standard formula: Result = (1 - Sa) * D + (1 - Da) * S + Sa * Da * B(Sc, Dc)
func separableBlendPD(sr, sg, sb, sa, dr, dg, db, da byte, chanFn func(s, d byte) byte) (byte, byte, byte, byte) {
	if sa == 0 {
		return dr, dg, db, da
	}
	if da == 0 {
		return sr, sg, sb, sa
	}

	// Unpremultiply.
	sur := byte((uint16(sr) * 255) / uint16(sa))
	sug := byte((uint16(sg) * 255) / uint16(sa))
	sub := byte((uint16(sb) * 255) / uint16(sa))
	dur := byte((uint16(dr) * 255) / uint16(da))
	dug := byte((uint16(dg) * 255) / uint16(da))
	dub := byte((uint16(db) * 255) / uint16(da))

	blendR := chanFn(sur, dur)
	blendG := chanFn(sug, dug)
	blendB := chanFn(sub, dub)

	invSa := 255 - sa
	invDa := 255 - da
	finalA := addDiv255PD(sa, mulDiv255PD(da, invSa))

	finalR := addDiv255PD(mulDiv255PD(dr, invSa), mulDiv255PD(sr, invDa))
	finalG := addDiv255PD(mulDiv255PD(dg, invSa), mulDiv255PD(sg, invDa))
	finalB := addDiv255PD(mulDiv255PD(db, invSa), mulDiv255PD(sb, invDa))

	saDa := mulDiv255PD(sa, da)
	finalR = addDiv255PD(finalR, mulDiv255PD(saDa, blendR))
	finalG = addDiv255PD(finalG, mulDiv255PD(saDa, blendG))
	finalB = addDiv255PD(finalB, mulDiv255PD(saDa, blendB))

	return finalR, finalG, finalB, finalA
}

func blendMultiplyFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, mulDiv255PD)
}

func blendScreenFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		return 255 - mulDiv255PD(255-s, 255-d)
	})
}

func blendOverlayFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if d <= 128 {
			return mulDiv255PD(2*d, s)
		}
		return 255 - mulDiv255PD(2*(255-d), 255-s)
	})
}

func blendDarkenFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s < d {
			return s
		}
		return d
	})
}

func blendLightenFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s > d {
			return s
		}
		return d
	})
}

func blendColorDodgeFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s == 255 {
			return 255
		}
		result := (uint16(d) * 255) / uint16(255-s)
		if result > 255 {
			return 255
		}
		return byte(result)
	})
}

func blendColorBurnFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s == 0 {
			return 0
		}
		result := (uint16(255-d) * 255) / uint16(s)
		if result > 255 {
			return 0
		}
		return 255 - byte(result)
	})
}

func blendHardLightFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s <= 128 {
			return mulDiv255PD(2*s, d)
		}
		return 255 - mulDiv255PD(2*(255-s), 255-d)
	})
}

func blendSoftLightFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		sf := float64(s) / 255.0
		df := float64(d) / 255.0
		var result float64
		if sf <= 0.5 {
			result = df - (1-2*sf)*df*(1-df)
		} else {
			var dx float64
			if df <= 0.25 {
				dx = ((16*df-12)*df + 4) * df
			} else {
				dx = math.Sqrt(df)
			}
			result = df + (2*sf-1)*(dx-df)
		}
		if result < 0 {
			return 0
		}
		if result > 1 {
			return 255
		}
		return byte(result*255.0 + 0.5)
	})
}

func blendDifferenceFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		if s > d {
			return s - d
		}
		return d - s
	})
}

func blendExclusionFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return separableBlendPD(sr, sg, sb, sa, dr, dg, db, da, func(s, d byte) byte {
		sum := uint16(s) + uint16(d)
		product := mulDiv255PD(s, d)
		diff := sum - 2*uint16(product)
		if diff > 255 {
			return 255
		}
		return byte(diff)
	})
}

// --- Non-separable HSL modes ---

func blendHueFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return nonSeparableBlendPD(sr, sg, sb, sa, dr, dg, db, da, hslBlendHuePD)
}

func blendSaturationFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return nonSeparableBlendPD(sr, sg, sb, sa, dr, dg, db, da, hslBlendSaturationPD)
}

func blendColorFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return nonSeparableBlendPD(sr, sg, sb, sa, dr, dg, db, da, hslBlendColorPD)
}

func blendLuminosityFn(sr, sg, sb, sa, dr, dg, db, da byte) (byte, byte, byte, byte) {
	return nonSeparableBlendPD(sr, sg, sb, sa, dr, dg, db, da, hslBlendLuminosityPD)
}

// nonSeparableBlendPD is a helper for HSL blend modes.
func nonSeparableBlendPD(
	sr, sg, sb, sa, dr, dg, db, da byte,
	fn func(sr, sg, sb, dr, dg, db float32) (float32, float32, float32),
) (byte, byte, byte, byte) {
	if sa == 0 {
		return dr, dg, db, da
	}
	if da == 0 {
		return sr, sg, sb, sa
	}

	// Unpremultiply to float [0,1].
	sur := float32(sr) / float32(sa)
	sug := float32(sg) / float32(sa)
	sub := float32(sb) / float32(sa)
	dur := float32(dr) / float32(da)
	dug := float32(dg) / float32(da)
	dub := float32(db) / float32(da)

	blendR, blendG, blendB := fn(sur, sug, sub, dur, dug, dub)

	invSa := 255 - sa
	invDa := 255 - da
	saf := float32(sa) / 255.0
	daf := float32(da) / 255.0

	finalA := addDiv255PD(sa, mulDiv255PD(da, invSa))

	finalR := addDiv255PD(mulDiv255PD(dr, invSa), mulDiv255PD(sr, invDa))
	finalG := addDiv255PD(mulDiv255PD(dg, invSa), mulDiv255PD(sg, invDa))
	finalB := addDiv255PD(mulDiv255PD(db, invSa), mulDiv255PD(sb, invDa))

	saDa := saf * daf
	blendContribR := byte(math.Round(float64(blendR * saDa * 255.0)))
	blendContribG := byte(math.Round(float64(blendG * saDa * 255.0)))
	blendContribB := byte(math.Round(float64(blendB * saDa * 255.0)))

	finalR = addDiv255PD(finalR, blendContribR)
	finalG = addDiv255PD(finalG, blendContribG)
	finalB = addDiv255PD(finalB, blendContribB)

	return finalR, finalG, finalB, finalA
}

// HSL helper functions (mirrored from internal/blend/hsl.go).

func lumPD(r, g, b float32) float32 { return 0.30*r + 0.59*g + 0.11*b }

func satPD(r, g, b float32) float32 {
	mx := r
	if g > mx {
		mx = g
	}
	if b > mx {
		mx = b
	}
	mn := r
	if g < mn {
		mn = g
	}
	if b < mn {
		mn = b
	}
	return mx - mn
}

func clipColorPD(r, g, b float32) (float32, float32, float32) {
	l := lumPD(r, g, b)
	mn := r
	if g < mn {
		mn = g
	}
	if b < mn {
		mn = b
	}
	mx := r
	if g > mx {
		mx = g
	}
	if b > mx {
		mx = b
	}
	if mn < 0 {
		d := l - mn
		if d != 0 {
			r = l + (r-l)*l/d
			g = l + (g-l)*l/d
			b = l + (b-l)*l/d
		}
	}
	if mx > 1 {
		d := mx - l
		if d != 0 {
			r = l + (r-l)*(1-l)/d
			g = l + (g-l)*(1-l)/d
			b = l + (b-l)*(1-l)/d
		}
	}
	return r, g, b
}

func setLumPD(r, g, b, l float32) (float32, float32, float32) {
	d := l - lumPD(r, g, b)
	return clipColorPD(r+d, g+d, b+d)
}

func setSatPD(r, g, b, s float32) (float32, float32, float32) {
	// Sort to find min, mid, max pointers.
	minP, midP, maxP := &r, &g, &b
	if *minP > *midP {
		minP, midP = midP, minP
	}
	if *midP > *maxP {
		midP, maxP = maxP, midP
	}
	if *minP > *midP {
		minP, midP = midP, minP
	}
	if *maxP > *minP {
		*midP = ((*midP - *minP) * s) / (*maxP - *minP)
		*maxP = s
		*minP = 0
	}
	return r, g, b
}

func hslBlendHuePD(sr, sg, sb, dr, dg, db float32) (float32, float32, float32) {
	r, g, b := setSatPD(sr, sg, sb, satPD(dr, dg, db))
	return setLumPD(r, g, b, lumPD(dr, dg, db))
}

func hslBlendSaturationPD(sr, sg, sb, dr, dg, db float32) (float32, float32, float32) {
	r, g, b := setSatPD(dr, dg, db, satPD(sr, sg, sb))
	return setLumPD(r, g, b, lumPD(dr, dg, db))
}

func hslBlendColorPD(sr, sg, sb, dr, dg, db float32) (float32, float32, float32) {
	return setLumPD(sr, sg, sb, lumPD(dr, dg, db))
}

func hslBlendLuminosityPD(sr, sg, sb, dr, dg, db float32) (float32, float32, float32) {
	return setLumPD(dr, dg, db, lumPD(sr, sg, sb))
}
