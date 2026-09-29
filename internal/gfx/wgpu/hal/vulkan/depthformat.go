//go:build !(js && wasm)

package vulkan

import (
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu/hal"
	"github.com/doug/gophics/internal/gfx/wgpu/hal/vulkan/vk"
)

// WebGPU's depth24plus formats promise "at least 24 bits of depth", not a
// particular layout, because no single Vulkan format is available everywhere.
// VK_FORMAT_D24_UNORM_S8_UINT is optional in Vulkan and Apple GPUs do not have
// it, so a Vulkan device layered on Metal (MoltenVK, and the Android emulator's
// host-GPU mode, which rides on it) rejects the image with
// VK_ERROR_VALIDATION_FAILED_EXT; the renderer then cannot create its
// depth/stencil target at all. The spec requires one of D24S8 or D32S8 to be
// supported, so fall back to the 32-bit float variant, as wgpu does.
//
// The choice has to be made per physical device and used by every call site
// that names a depth format — the image, its views, the pipeline's render
// pass and the render pass it draws in must all agree — so it lives on the
// Device rather than in the device-independent textureFormatToVk table.
type depthFormats struct {
	depth24PlusStencil8 vk.Format
	depth24Plus         vk.Format
}

func queryDepthFormats(inst *Instance, pd vk.PhysicalDevice) depthFormats {
	return depthFormats{
		depth24PlusStencil8: firstDepthAttachmentFormat(inst, pd, vk.FormatD24UnormS8Uint, vk.FormatD32SfloatS8Uint),
		depth24Plus:         firstDepthAttachmentFormat(inst, pd, vk.FormatX8D24UnormPack32, vk.FormatD32Sfloat),
	}
}

// firstDepthAttachmentFormat returns the first candidate usable as an
// optimally tiled depth/stencil attachment, or the first candidate when none
// report support, which keeps the old behavior on a driver that answers the
// query badly.
func firstDepthAttachmentFormat(inst *Instance, pd vk.PhysicalDevice, candidates ...vk.Format) vk.Format {
	for _, f := range candidates {
		var props vk.FormatProperties
		inst.cmds.GetPhysicalDeviceFormatProperties(pd, f, &props)
		if props.OptimalTilingFeatures&vk.FormatFeatureFlags(vk.FormatFeatureDepthStencilAttachmentBit) != 0 {
			if f != candidates[0] {
				hal.Logger().Info("vulkan: depth format fallback", "wanted", candidates[0], "using", f)
			}
			return f
		}
	}
	return candidates[0]
}

// resolve maps a WebGPU format to the Vulkan format this device uses for it.
func (df depthFormats) resolve(format gputypes.TextureFormat) vk.Format {
	switch {
	case format == gputypes.TextureFormatDepth24PlusStencil8 && df.depth24PlusStencil8 != vk.FormatUndefined:
		return df.depth24PlusStencil8
	case format == gputypes.TextureFormatDepth24Plus && df.depth24Plus != vk.FormatUndefined:
		return df.depth24Plus
	}
	return textureFormatToVk(format)
}

// vkFormat is textureFormatToVk with this device's depth format choices.
func (d *Device) vkFormat(format gputypes.TextureFormat) vk.Format {
	return d.depth.resolve(format)
}
