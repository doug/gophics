// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package gputypes

// PresentMode specifies how frames are presented to the display.
// Controls VSync behavior and tearing.
type PresentMode uint32

const (
	// PresentModeUndefined is an undefined present mode (invalid).
	PresentModeUndefined PresentMode = 0x00000000

	// PresentModeFifo presents frames in FIFO order with VSync.
	// No tearing. Supported on all platforms.
	// Also known as "VSync On".
	PresentModeFifo PresentMode = 0x00000001

	// PresentModeFifoRelaxed is like Fifo but allows tearing if frames
	// take longer than one vblank.
	// Also known as "Adaptive VSync".
	PresentModeFifoRelaxed PresentMode = 0x00000002

	// PresentModeImmediate presents frames immediately without waiting for vblank.
	// May cause tearing. Also known as "VSync Off".
	PresentModeImmediate PresentMode = 0x00000003

	// PresentModeMailbox uses a single-frame queue, replacing old frames.
	// No tearing. Also known as "Fast VSync".
	PresentModeMailbox PresentMode = 0x00000004
)

// String returns the string representation of PresentMode.
func (m PresentMode) String() string {
	switch m {
	case PresentModeUndefined:
		return "Undefined"
	case PresentModeFifo:
		return "Fifo"
	case PresentModeFifoRelaxed:
		return "FifoRelaxed"
	case PresentModeImmediate:
		return "Immediate"
	case PresentModeMailbox:
		return "Mailbox"
	default:
		return "Unknown"
	}
}

// CompositeAlphaMode specifies how alpha channel is handled during compositing.
type CompositeAlphaMode uint32

const (
	// CompositeAlphaModeAuto chooses Opaque or Inherit automatically.
	CompositeAlphaModeAuto CompositeAlphaMode = 0x00000000

	// CompositeAlphaModeOpaque ignores alpha, treats texture as fully opaque.
	CompositeAlphaModeOpaque CompositeAlphaMode = 0x00000001

	// CompositeAlphaModePremultiplied expects colors to be pre-multiplied by alpha.
	CompositeAlphaModePremultiplied CompositeAlphaMode = 0x00000002

	// CompositeAlphaModeUnpremultiplied has compositor multiply colors by alpha.
	CompositeAlphaModeUnpremultiplied CompositeAlphaMode = 0x00000003

	// CompositeAlphaModeInherit uses platform-specific default.
	CompositeAlphaModeInherit CompositeAlphaMode = 0x00000004
)

// String returns the string representation of CompositeAlphaMode.
func (m CompositeAlphaMode) String() string {
	switch m {
	case CompositeAlphaModeAuto:
		return "Auto"
	case CompositeAlphaModeOpaque:
		return "Opaque"
	case CompositeAlphaModePremultiplied:
		return "Premultiplied"
	case CompositeAlphaModeUnpremultiplied:
		return "Unpremultiplied"
	case CompositeAlphaModeInherit:
		return "Inherit"
	default:
		return "Unknown"
	}
}

// SurfaceConfiguration configures a surface for presentation.
type SurfaceConfiguration struct {
	// Usage specifies how the surface texture will be used.
	// TextureUsageRenderAttachment is always supported.
	Usage TextureUsage

	// Format is the texture format of the surface.
	// BGRA8Unorm and BGRA8UnormSrgb are guaranteed to be supported.
	Format TextureFormat

	// Width of the surface in pixels. Must be non-zero.
	Width uint32

	// Height of the surface in pixels. Must be non-zero.
	Height uint32

	// PresentMode controls VSync and frame presentation timing.
	PresentMode PresentMode

	// DesiredMaximumFrameLatency is the target number of frames in flight.
	// Typical values are 1-3. Default is usually 2.
	DesiredMaximumFrameLatency uint32

	// AlphaMode specifies how alpha is handled during compositing.
	AlphaMode CompositeAlphaMode

	// ViewFormats lists additional formats for texture views.
	// Only sRGB variants of the surface format are typically allowed.
	ViewFormats []TextureFormat
}

// SurfaceCapabilities describes what a surface supports with a given adapter.
type SurfaceCapabilities struct {
	// Formats lists supported texture formats. First is preferred.
	Formats []TextureFormat

	// PresentModes lists supported presentation modes.
	PresentModes []PresentMode

	// AlphaModes lists supported alpha compositing modes.
	AlphaModes []CompositeAlphaMode

	// Usages is a bitflag of supported texture usages.
	Usages TextureUsage
}

// SurfaceStatus indicates the state of a surface texture acquisition.
type SurfaceStatus uint32

const (
	// SurfaceStatusGood indicates no issues.
	SurfaceStatusGood SurfaceStatus = iota

	// SurfaceStatusSuboptimal indicates the surface works but should be reconfigured.
	SurfaceStatusSuboptimal

	// SurfaceStatusTimeout indicates texture acquisition timed out.
	SurfaceStatusTimeout

	// SurfaceStatusOutdated indicates the underlying surface changed.
	SurfaceStatusOutdated

	// SurfaceStatusLost indicates the surface was lost.
	SurfaceStatusLost

	// SurfaceStatusUnknown indicates status is unknown due to previous failure.
	SurfaceStatusUnknown
)

// String returns the string representation of SurfaceStatus.
func (s SurfaceStatus) String() string {
	switch s {
	case SurfaceStatusGood:
		return "Good"
	case SurfaceStatusSuboptimal:
		return "Suboptimal"
	case SurfaceStatusTimeout:
		return "Timeout"
	case SurfaceStatusOutdated:
		return "Outdated"
	case SurfaceStatusLost:
		return "Lost"
	case SurfaceStatusUnknown:
		return "Unknown"
	default:
		return "Unknown"
	}
}
