package gputypes

import "testing"

func TestTextureFormat_BlockCopySize(t *testing.T) {
	tests := []struct {
		name   string
		format TextureFormat
		want   uint32
	}{
		// Unknown / undefined → 0
		{"Undefined", TextureFormatUndefined, 0},
		{"Unknown value", TextureFormat(0xFFFF), 0},

		// 1 byte per texel
		{"R8Unorm", TextureFormatR8Unorm, 1},
		{"R8Snorm", TextureFormatR8Snorm, 1},
		{"R8Uint", TextureFormatR8Uint, 1},
		{"R8Sint", TextureFormatR8Sint, 1},
		{"Stencil8", TextureFormatStencil8, 1},

		// 2 bytes per texel
		{"R16Unorm", TextureFormatR16Unorm, 2},
		{"R16Snorm", TextureFormatR16Snorm, 2},
		{"R16Uint", TextureFormatR16Uint, 2},
		{"R16Sint", TextureFormatR16Sint, 2},
		{"R16Float", TextureFormatR16Float, 2},
		{"RG8Unorm", TextureFormatRG8Unorm, 2},
		{"RG8Snorm", TextureFormatRG8Snorm, 2},
		{"RG8Uint", TextureFormatRG8Uint, 2},
		{"RG8Sint", TextureFormatRG8Sint, 2},
		{"Depth16Unorm", TextureFormatDepth16Unorm, 2},

		// 4 bytes per texel
		{"R32Float", TextureFormatR32Float, 4},
		{"R32Uint", TextureFormatR32Uint, 4},
		{"R32Sint", TextureFormatR32Sint, 4},
		{"RG16Unorm", TextureFormatRG16Unorm, 4},
		{"RG16Snorm", TextureFormatRG16Snorm, 4},
		{"RG16Uint", TextureFormatRG16Uint, 4},
		{"RG16Sint", TextureFormatRG16Sint, 4},
		{"RG16Float", TextureFormatRG16Float, 4},
		{"RGBA8Unorm", TextureFormatRGBA8Unorm, 4},
		{"RGBA8UnormSrgb", TextureFormatRGBA8UnormSrgb, 4},
		{"RGBA8Snorm", TextureFormatRGBA8Snorm, 4},
		{"RGBA8Uint", TextureFormatRGBA8Uint, 4},
		{"RGBA8Sint", TextureFormatRGBA8Sint, 4},
		{"BGRA8Unorm", TextureFormatBGRA8Unorm, 4},
		{"BGRA8UnormSrgb", TextureFormatBGRA8UnormSrgb, 4},
		{"RGB10A2Uint", TextureFormatRGB10A2Uint, 4},
		{"RGB10A2Unorm", TextureFormatRGB10A2Unorm, 4},
		{"RG11B10Ufloat", TextureFormatRG11B10Ufloat, 4},
		{"RGB9E5Ufloat", TextureFormatRGB9E5Ufloat, 4},
		{"Depth32Float", TextureFormatDepth32Float, 4},

		// 8 bytes per texel
		{"RG32Float", TextureFormatRG32Float, 8},
		{"RG32Uint", TextureFormatRG32Uint, 8},
		{"RG32Sint", TextureFormatRG32Sint, 8},
		{"RGBA16Unorm", TextureFormatRGBA16Unorm, 8},
		{"RGBA16Snorm", TextureFormatRGBA16Snorm, 8},
		{"RGBA16Uint", TextureFormatRGBA16Uint, 8},
		{"RGBA16Sint", TextureFormatRGBA16Sint, 8},
		{"RGBA16Float", TextureFormatRGBA16Float, 8},

		// 16 bytes per texel
		{"RGBA32Float", TextureFormatRGBA32Float, 16},
		{"RGBA32Uint", TextureFormatRGBA32Uint, 16},
		{"RGBA32Sint", TextureFormatRGBA32Sint, 16},

		// Depth/stencil implementation-defined → 0
		{"Depth24Plus", TextureFormatDepth24Plus, 0},
		{"Depth24PlusStencil8", TextureFormatDepth24PlusStencil8, 0},
		{"Depth32FloatStencil8", TextureFormatDepth32FloatStencil8, 0},

		// BC compressed — 8 bytes per 4x4 block
		{"BC1RGBAUnorm", TextureFormatBC1RGBAUnorm, 8},
		{"BC1RGBAUnormSrgb", TextureFormatBC1RGBAUnormSrgb, 8},
		{"BC4RUnorm", TextureFormatBC4RUnorm, 8},
		{"BC4RSnorm", TextureFormatBC4RSnorm, 8},

		// BC compressed — 16 bytes per 4x4 block
		{"BC2RGBAUnorm", TextureFormatBC2RGBAUnorm, 16},
		{"BC2RGBAUnormSrgb", TextureFormatBC2RGBAUnormSrgb, 16},
		{"BC3RGBAUnorm", TextureFormatBC3RGBAUnorm, 16},
		{"BC3RGBAUnormSrgb", TextureFormatBC3RGBAUnormSrgb, 16},
		{"BC5RGUnorm", TextureFormatBC5RGUnorm, 16},
		{"BC5RGSnorm", TextureFormatBC5RGSnorm, 16},
		{"BC6HRGBUfloat", TextureFormatBC6HRGBUfloat, 16},
		{"BC6HRGBFloat", TextureFormatBC6HRGBFloat, 16},
		{"BC7RGBAUnorm", TextureFormatBC7RGBAUnorm, 16},
		{"BC7RGBAUnormSrgb", TextureFormatBC7RGBAUnormSrgb, 16},

		// ETC2/EAC compressed — 8 bytes per 4x4 block
		{"ETC2RGB8Unorm", TextureFormatETC2RGB8Unorm, 8},
		{"ETC2RGB8UnormSrgb", TextureFormatETC2RGB8UnormSrgb, 8},
		{"ETC2RGB8A1Unorm", TextureFormatETC2RGB8A1Unorm, 8},
		{"ETC2RGB8A1UnormSrgb", TextureFormatETC2RGB8A1UnormSrgb, 8},
		{"EACR11Unorm", TextureFormatEACR11Unorm, 8},
		{"EACR11Snorm", TextureFormatEACR11Snorm, 8},

		// ETC2/EAC compressed — 16 bytes per 4x4 block
		{"ETC2RGBA8Unorm", TextureFormatETC2RGBA8Unorm, 16},
		{"ETC2RGBA8UnormSrgb", TextureFormatETC2RGBA8UnormSrgb, 16},
		{"EACRG11Unorm", TextureFormatEACRG11Unorm, 16},
		{"EACRG11Snorm", TextureFormatEACRG11Snorm, 16},

		// ASTC compressed — all 16 bytes per block
		{"ASTC4x4Unorm", TextureFormatASTC4x4Unorm, 16},
		{"ASTC4x4UnormSrgb", TextureFormatASTC4x4UnormSrgb, 16},
		{"ASTC5x4Unorm", TextureFormatASTC5x4Unorm, 16},
		{"ASTC5x4UnormSrgb", TextureFormatASTC5x4UnormSrgb, 16},
		{"ASTC5x5Unorm", TextureFormatASTC5x5Unorm, 16},
		{"ASTC5x5UnormSrgb", TextureFormatASTC5x5UnormSrgb, 16},
		{"ASTC6x5Unorm", TextureFormatASTC6x5Unorm, 16},
		{"ASTC6x5UnormSrgb", TextureFormatASTC6x5UnormSrgb, 16},
		{"ASTC6x6Unorm", TextureFormatASTC6x6Unorm, 16},
		{"ASTC6x6UnormSrgb", TextureFormatASTC6x6UnormSrgb, 16},
		{"ASTC8x5Unorm", TextureFormatASTC8x5Unorm, 16},
		{"ASTC8x5UnormSrgb", TextureFormatASTC8x5UnormSrgb, 16},
		{"ASTC8x6Unorm", TextureFormatASTC8x6Unorm, 16},
		{"ASTC8x6UnormSrgb", TextureFormatASTC8x6UnormSrgb, 16},
		{"ASTC8x8Unorm", TextureFormatASTC8x8Unorm, 16},
		{"ASTC8x8UnormSrgb", TextureFormatASTC8x8UnormSrgb, 16},
		{"ASTC10x5Unorm", TextureFormatASTC10x5Unorm, 16},
		{"ASTC10x5UnormSrgb", TextureFormatASTC10x5UnormSrgb, 16},
		{"ASTC10x6Unorm", TextureFormatASTC10x6Unorm, 16},
		{"ASTC10x6UnormSrgb", TextureFormatASTC10x6UnormSrgb, 16},
		{"ASTC10x8Unorm", TextureFormatASTC10x8Unorm, 16},
		{"ASTC10x8UnormSrgb", TextureFormatASTC10x8UnormSrgb, 16},
		{"ASTC10x10Unorm", TextureFormatASTC10x10Unorm, 16},
		{"ASTC10x10UnormSrgb", TextureFormatASTC10x10UnormSrgb, 16},
		{"ASTC12x10Unorm", TextureFormatASTC12x10Unorm, 16},
		{"ASTC12x10UnormSrgb", TextureFormatASTC12x10UnormSrgb, 16},
		{"ASTC12x12Unorm", TextureFormatASTC12x12Unorm, 16},
		{"ASTC12x12UnormSrgb", TextureFormatASTC12x12UnormSrgb, 16},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.format.BlockCopySize()
			if got != tt.want {
				t.Errorf("TextureFormat(%s).BlockCopySize() = %d, want %d",
					tt.format, got, tt.want)
			}
		})
	}
}

// TestBlockCopySizeCoversAllFormats verifies that every defined TextureFormat
// constant is covered by BlockCopySize (returns a known value, not falling
// through to default for valid formats).
func TestBlockCopySizeCoversAllFormats(t *testing.T) {
	// All defined formats and their expected non-zero or explicitly-zero results.
	// This test ensures we don't accidentally miss a format in the switch.
	allFormats := []TextureFormat{
		TextureFormatR8Unorm, TextureFormatR8Snorm, TextureFormatR8Uint, TextureFormatR8Sint,
		TextureFormatR16Unorm, TextureFormatR16Snorm, TextureFormatR16Uint, TextureFormatR16Sint, TextureFormatR16Float,
		TextureFormatRG8Unorm, TextureFormatRG8Snorm, TextureFormatRG8Uint, TextureFormatRG8Sint,
		TextureFormatR32Float, TextureFormatR32Uint, TextureFormatR32Sint,
		TextureFormatRG16Unorm, TextureFormatRG16Snorm, TextureFormatRG16Uint, TextureFormatRG16Sint, TextureFormatRG16Float,
		TextureFormatRGBA8Unorm, TextureFormatRGBA8UnormSrgb, TextureFormatRGBA8Snorm, TextureFormatRGBA8Uint, TextureFormatRGBA8Sint,
		TextureFormatBGRA8Unorm, TextureFormatBGRA8UnormSrgb,
		TextureFormatRGB10A2Uint, TextureFormatRGB10A2Unorm, TextureFormatRG11B10Ufloat, TextureFormatRGB9E5Ufloat,
		TextureFormatRG32Float, TextureFormatRG32Uint, TextureFormatRG32Sint,
		TextureFormatRGBA16Unorm, TextureFormatRGBA16Snorm, TextureFormatRGBA16Uint, TextureFormatRGBA16Sint, TextureFormatRGBA16Float,
		TextureFormatRGBA32Float, TextureFormatRGBA32Uint, TextureFormatRGBA32Sint,
		TextureFormatStencil8, TextureFormatDepth16Unorm,
		TextureFormatDepth24Plus, TextureFormatDepth24PlusStencil8,
		TextureFormatDepth32Float, TextureFormatDepth32FloatStencil8,
		TextureFormatBC1RGBAUnorm, TextureFormatBC1RGBAUnormSrgb,
		TextureFormatBC2RGBAUnorm, TextureFormatBC2RGBAUnormSrgb,
		TextureFormatBC3RGBAUnorm, TextureFormatBC3RGBAUnormSrgb,
		TextureFormatBC4RUnorm, TextureFormatBC4RSnorm,
		TextureFormatBC5RGUnorm, TextureFormatBC5RGSnorm,
		TextureFormatBC6HRGBUfloat, TextureFormatBC6HRGBFloat,
		TextureFormatBC7RGBAUnorm, TextureFormatBC7RGBAUnormSrgb,
		TextureFormatETC2RGB8Unorm, TextureFormatETC2RGB8UnormSrgb,
		TextureFormatETC2RGB8A1Unorm, TextureFormatETC2RGB8A1UnormSrgb,
		TextureFormatETC2RGBA8Unorm, TextureFormatETC2RGBA8UnormSrgb,
		TextureFormatEACR11Unorm, TextureFormatEACR11Snorm,
		TextureFormatEACRG11Unorm, TextureFormatEACRG11Snorm,
		TextureFormatASTC4x4Unorm, TextureFormatASTC4x4UnormSrgb,
		TextureFormatASTC5x4Unorm, TextureFormatASTC5x4UnormSrgb,
		TextureFormatASTC5x5Unorm, TextureFormatASTC5x5UnormSrgb,
		TextureFormatASTC6x5Unorm, TextureFormatASTC6x5UnormSrgb,
		TextureFormatASTC6x6Unorm, TextureFormatASTC6x6UnormSrgb,
		TextureFormatASTC8x5Unorm, TextureFormatASTC8x5UnormSrgb,
		TextureFormatASTC8x6Unorm, TextureFormatASTC8x6UnormSrgb,
		TextureFormatASTC8x8Unorm, TextureFormatASTC8x8UnormSrgb,
		TextureFormatASTC10x5Unorm, TextureFormatASTC10x5UnormSrgb,
		TextureFormatASTC10x6Unorm, TextureFormatASTC10x6UnormSrgb,
		TextureFormatASTC10x8Unorm, TextureFormatASTC10x8UnormSrgb,
		TextureFormatASTC10x10Unorm, TextureFormatASTC10x10UnormSrgb,
		TextureFormatASTC12x10Unorm, TextureFormatASTC12x10UnormSrgb,
		TextureFormatASTC12x12Unorm, TextureFormatASTC12x12UnormSrgb,
	}

	// Implementation-defined formats that legitimately return 0.
	implDefined := map[TextureFormat]bool{
		TextureFormatDepth24Plus:          true,
		TextureFormatDepth24PlusStencil8:  true,
		TextureFormatDepth32FloatStencil8: true,
	}

	for _, f := range allFormats {
		size := f.BlockCopySize()
		if size == 0 && !implDefined[f] {
			t.Errorf("TextureFormat(%s) returned 0 but is not implementation-defined — likely missing from switch", f)
		}
	}
}
