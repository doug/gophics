package paint

import "github.com/doug/gophics/internal/gfx/gg"

// BlendMode says how a shape's colour combines with what is already on the
// canvas. BlendNormal — plain source-over alpha compositing — is the default
// and what every draw call used before blend modes were exposed.
//
// The set is the W3C Compositing and Blending Level 1 list, which is also
// Flutter's BlendMode and Compose's BlendMode, and it is exactly what the
// rasterizer underneath implements: every mode here was checked against a
// real fill, and no mode the substrate cannot do is named. The separable and
// non-separable *blend* modes (Multiply through Luminosity) describe how
// colours mix; the Porter-Duff *compositing* operators (Clear through Xor)
// describe which of the two shapes survives where they do and do not overlap.
type BlendMode uint8

// Blend modes. The numbering is gophics's own — the substrate's enum has gaps
// and a legacy prefix, and pinning the public values to it would make a
// vendored-fork update an API break.
const (
	// BlendNormal draws the source over the destination (the default).
	BlendNormal BlendMode = iota

	// Separable blend modes: each channel mixes independently.

	// BlendMultiply multiplies the two colours — always darker, and the
	// mode to reach for when tinting or shading something already painted.
	BlendMultiply
	// BlendScreen is multiply's inverse — always lighter; glows and
	// additive highlights that must not clip to white.
	BlendScreen
	// BlendOverlay multiplies dark destinations and screens light ones,
	// which raises contrast without flattening either end.
	BlendOverlay
	// BlendDarken keeps the darker of the two per channel.
	BlendDarken
	// BlendLighten keeps the lighter of the two per channel.
	BlendLighten
	// BlendColorDodge brightens the destination in proportion to the source.
	BlendColorDodge
	// BlendColorBurn darkens the destination in proportion to the source.
	BlendColorBurn
	// BlendHardLight is Overlay with the roles swapped (source decides).
	BlendHardLight
	// BlendSoftLight is a gentler HardLight — the usual choice for shading
	// an illustration without posterising it.
	BlendSoftLight
	// BlendDifference is the absolute per-channel difference; equal colours
	// cancel to black, which makes it the cheap "did these two frames
	// differ" overlay.
	BlendDifference
	// BlendExclusion is Difference with lower contrast in the mid-tones.
	BlendExclusion

	// Non-separable blend modes: these work on the colour as a whole, so
	// they can be used to recolour without disturbing brightness.

	// BlendHue takes the source's hue and the destination's saturation and
	// luminosity.
	BlendHue
	// BlendSaturation takes the source's saturation only.
	BlendSaturation
	// BlendColor takes the source's hue and saturation, keeping the
	// destination's luminosity — a recolour that preserves shading.
	BlendColor
	// BlendLuminosity takes the source's luminosity only, keeping the
	// destination's colour — the other half of BlendColor.
	BlendLuminosity

	// Porter-Duff compositing operators.

	// BlendPlus adds the two colours and clamps, for additive light.
	BlendPlus
	// BlendModulate multiplies in premultiplied space (Skia's modulate).
	BlendModulate
	// BlendClear erases to transparent wherever the shape covers.
	BlendClear
	// BlendSrc replaces the destination with the source, alpha included —
	// how to punch an exact colour (including transparency) into a surface.
	BlendSrc
	// BlendDst keeps the destination; the source is discarded. A no-op,
	// useful as an animated "off" value so a caller need not branch.
	BlendDst
	// BlendDstOver draws the source *behind* what is already there.
	BlendDstOver
	// BlendSrcIn keeps the source only where the destination is opaque —
	// masking a fill by previously drawn alpha.
	BlendSrcIn
	// BlendDstIn keeps the destination only where the source is opaque.
	BlendDstIn
	// BlendSrcOut keeps the source only where the destination is clear.
	BlendSrcOut
	// BlendDstOut keeps the destination only where the source is clear —
	// knocking a hole in painted content.
	BlendDstOut
	// BlendSrcAtop draws the source over the destination but clipped to the
	// destination's alpha, leaving the overall silhouette untouched.
	BlendSrcAtop
	// BlendDstAtop is SrcAtop with the roles swapped.
	BlendDstAtop
	// BlendXor keeps each where the other is not; the overlap clears.
	BlendXor

	numBlendModes
)

// gg maps a gophics mode onto the substrate's. Unknown values fall back to
// Normal rather than panicking: a BlendMode arrives from app code, animation
// interpolation and (one day) a deserialised theme, and a stray value should
// paint plainly, not take the frame down.
func (m BlendMode) gg() gg.BlendMode {
	switch m {
	case BlendMultiply:
		return gg.BlendMultiply
	case BlendScreen:
		return gg.BlendScreen
	case BlendOverlay:
		return gg.BlendOverlay
	case BlendDarken:
		return gg.BlendDarken
	case BlendLighten:
		return gg.BlendLighten
	case BlendColorDodge:
		return gg.BlendColorDodge
	case BlendColorBurn:
		return gg.BlendColorBurn
	case BlendHardLight:
		return gg.BlendHardLight
	case BlendSoftLight:
		return gg.BlendSoftLight
	case BlendDifference:
		return gg.BlendDifference
	case BlendExclusion:
		return gg.BlendExclusion
	case BlendHue:
		return gg.BlendHue
	case BlendSaturation:
		return gg.BlendSaturation
	case BlendColor:
		return gg.BlendColor
	case BlendLuminosity:
		return gg.BlendLuminosity
	case BlendPlus:
		return gg.BlendPlus
	case BlendModulate:
		return gg.BlendModulate
	case BlendClear:
		return gg.BlendClear
	case BlendSrc:
		return gg.BlendSource
	case BlendDst:
		return gg.BlendDestination
	case BlendDstOver:
		return gg.BlendDestinationOver
	case BlendSrcIn:
		return gg.BlendSourceIn
	case BlendDstIn:
		return gg.BlendDestinationIn
	case BlendSrcOut:
		return gg.BlendSourceOut
	case BlendDstOut:
		return gg.BlendDestinationOut
	case BlendSrcAtop:
		return gg.BlendSourceAtop
	case BlendDstAtop:
		return gg.BlendDestinationAtop
	case BlendXor:
		return gg.BlendXor
	default:
		return gg.BlendNormal
	}
}

var blendNames = [numBlendModes]string{
	BlendNormal:     "Normal",
	BlendMultiply:   "Multiply",
	BlendScreen:     "Screen",
	BlendOverlay:    "Overlay",
	BlendDarken:     "Darken",
	BlendLighten:    "Lighten",
	BlendColorDodge: "ColorDodge",
	BlendColorBurn:  "ColorBurn",
	BlendHardLight:  "HardLight",
	BlendSoftLight:  "SoftLight",
	BlendDifference: "Difference",
	BlendExclusion:  "Exclusion",
	BlendHue:        "Hue",
	BlendSaturation: "Saturation",
	BlendColor:      "Color",
	BlendLuminosity: "Luminosity",
	BlendPlus:       "Plus",
	BlendModulate:   "Modulate",
	BlendClear:      "Clear",
	BlendSrc:        "Src",
	BlendDst:        "Dst",
	BlendDstOver:    "DstOver",
	BlendSrcIn:      "SrcIn",
	BlendDstIn:      "DstIn",
	BlendSrcOut:     "SrcOut",
	BlendDstOut:     "DstOut",
	BlendSrcAtop:    "SrcAtop",
	BlendDstAtop:    "DstAtop",
	BlendXor:        "Xor",
}

// String names the mode, for diagnostics and golden-test failure messages.
func (m BlendMode) String() string {
	if m >= numBlendModes {
		return "BlendMode(?)"
	}
	return blendNames[m]
}

// Blender is the opt-in Canvas capability for blend modes. A Canvas that can
// blend offers a mode as *pushed and popped state*, the same shape as
// PushClip, PushOpacity and PushTransform: there is no per-draw-call variant
// of any other piece of paint state here, and adding a mode argument to each
// of Canvas's eleven draw methods would double the interface while still
// leaving DrawMarks and DrawSprite awkward. Nesting is a stack — the inner
// mode wins until its PopBlend, then the outer one resumes — so a widget can
// set a mode around a subtree without knowing what its parent did.
//
// The mode applies *per shape*, not to the group as a whole: two overlapping
// shapes drawn inside one PushBlend blend against each other as well as
// against the backdrop. That is what the rasterizer underneath offers for the
// full mode list (its group-compositing path implements only four of them),
// and it is the more useful half: a single multiplied tint or a knocked-out
// hole is the common case, and group isolation is available another way by
// wrapping the subtree in PushOpacity — with the caveat that an isolated
// group's contents then blend against the group's own transparent pixels
// rather than the backdrop, exactly as Flutter's saveLayer does.
//
// It is a capability rather than part of Canvas because Canvas is implemented
// outside this package, so a method added to it breaks every backend. nil from
// Blends is the real answer "this canvas cannot blend", and a caller that cares
// should check rather than paint something that silently composites normally.
//
// The canvas a widget is handed during a real frame is the scene recorder, and
// it does blend: it records the mode as a push/pop pair in the display list and
// the replay target supplies the destination, exactly as it supplies the pixels
// every other recorded op is missing. A recorder answering nil here was the
// first shape of this capability and it made the whole feature unreachable from
// application code — blending worked offscreen and nowhere an app could use it.
// What answers nil now is a Canvas that genuinely cannot blend: a third-party
// backend that has not implemented it.
type Blender interface {
	// PushBlend makes m the blend mode for subsequent draws; balance every
	// PushBlend with PopBlend.
	PushBlend(m BlendMode)
	// PopBlend restores the mode in force before the matching PushBlend.
	PopBlend()
}

// Blends returns c's blend-mode capability, or nil when c cannot blend. The
// painter canvas, the GPU canvas over it, and the scene recorder — which is
// what a widget's Draw is handed during a frame — all can.
//
//	if b := paint.Blends(c); b != nil {
//		b.PushBlend(paint.BlendMultiply)
//		c.FillRect(r, tint)
//		b.PopBlend()
//	} else {
//		c.FillRect(r, tint.WithAlpha(0.5)) // plain approximation
//	}
func Blends(c Canvas) Blender {
	b, _ := c.(Blender)
	return b
}

// PushBlend implements Blender. The mode lives in gg's paint state, which its
// own Push/Pop deliberately does not save (they cover the matrix, clip and
// mask), so this canvas keeps the stack itself rather than leaning on a
// save/restore that would not fire.
func (c *ggCanvas) PushBlend(m BlendMode) {
	c.blendStack = append(c.blendStack, c.blend)
	c.blend = m
	c.dc.SetBlendMode(m.gg())
}

// PopBlend implements Blender. An unbalanced pop resets to Normal instead of
// panicking: a dropped PopBlend is a bug, but one that leaves every later
// shape in the frame multiplied is far harder to recognise than one that
// leaves a single shape unblended.
func (c *ggCanvas) PopBlend() {
	if n := len(c.blendStack); n > 0 {
		c.blend = c.blendStack[n-1]
		c.blendStack = c.blendStack[:n-1]
	} else {
		c.blend = BlendNormal
	}
	c.dc.SetBlendMode(c.blend.gg())
}
