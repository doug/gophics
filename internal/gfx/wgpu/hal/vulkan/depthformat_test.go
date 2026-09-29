//go:build !(js && wasm)

package vulkan

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu/hal/vulkan/vk"
)

// A device without D24S8 (every Apple GPU, so MoltenVK and the Android
// emulator's host-GPU mode) must get D32S8 for depth24plus-stencil8 from every
// call site, and every other format must map exactly as the static table does.
func TestDepthFormatsResolveFallsBack(t *testing.T) {
	apple := depthFormats{depth24PlusStencil8: vk.FormatD32SfloatS8Uint, depth24Plus: vk.FormatD32Sfloat}
	if got := apple.resolve(gputypes.TextureFormatDepth24PlusStencil8); got != vk.FormatD32SfloatS8Uint {
		t.Errorf("Depth24PlusStencil8 → %v, want D32_SFLOAT_S8_UINT", got)
	}
	if got := apple.resolve(gputypes.TextureFormatDepth24Plus); got != vk.FormatD32Sfloat {
		t.Errorf("Depth24Plus → %v, want D32_SFLOAT", got)
	}
	for _, f := range []gputypes.TextureFormat{
		gputypes.TextureFormatRGBA8Unorm,
		gputypes.TextureFormatBGRA8Unorm,
		gputypes.TextureFormatDepth32Float,
		gputypes.TextureFormatDepth32FloatStencil8,
		gputypes.TextureFormatStencil8,
	} {
		if got, want := apple.resolve(f), textureFormatToVk(f); got != want {
			t.Errorf("%v → %v, want the table's %v", f, got, want)
		}
	}
}

// A zero depthFormats — a Device built without querying, as some tests do —
// keeps the static mapping rather than handing out FormatUndefined.
func TestDepthFormatsZeroValueUsesTable(t *testing.T) {
	var zero depthFormats
	for _, f := range []gputypes.TextureFormat{
		gputypes.TextureFormatDepth24PlusStencil8,
		gputypes.TextureFormatDepth24Plus,
	} {
		if got, want := zero.resolve(f), textureFormatToVk(f); got != want {
			t.Errorf("%v → %v, want %v", f, got, want)
		}
	}
}
