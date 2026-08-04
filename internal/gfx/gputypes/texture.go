package gputypes

// TextureFormat describes the format of a texture.
//
// This enum covers all texture formats defined in the WebGPU specification,
// including uncompressed, depth/stencil, and compressed formats.
type TextureFormat uint32

const (
	// TextureFormatUndefined is an undefined format (invalid).
	// webgpu.h: WGPUTextureFormat_Undefined = 0x00000000
	TextureFormatUndefined TextureFormat = 0x00000000

	// 8-bit formats
	// TextureFormatR8Unorm is a single 8-bit normalized unsigned integer.
	TextureFormatR8Unorm TextureFormat = 0x00000001
	// TextureFormatR8Snorm is a single 8-bit normalized signed integer.
	TextureFormatR8Snorm TextureFormat = 0x00000002
	// TextureFormatR8Uint is a single 8-bit unsigned integer.
	TextureFormatR8Uint TextureFormat = 0x00000003
	// TextureFormatR8Sint is a single 8-bit signed integer.
	TextureFormatR8Sint TextureFormat = 0x00000004

	// 16-bit formats
	// TextureFormatR16Unorm is a single 16-bit normalized unsigned integer.
	TextureFormatR16Unorm TextureFormat = 0x00000005
	// TextureFormatR16Snorm is a single 16-bit normalized signed integer.
	TextureFormatR16Snorm TextureFormat = 0x00000006
	// TextureFormatR16Uint is a single 16-bit unsigned integer.
	TextureFormatR16Uint TextureFormat = 0x00000007
	// TextureFormatR16Sint is a single 16-bit signed integer.
	TextureFormatR16Sint TextureFormat = 0x00000008
	// TextureFormatR16Float is a single 16-bit float.
	TextureFormatR16Float TextureFormat = 0x00000009
	// TextureFormatRG8Unorm is two 8-bit normalized unsigned integers.
	TextureFormatRG8Unorm TextureFormat = 0x0000000A
	// TextureFormatRG8Snorm is two 8-bit normalized signed integers.
	TextureFormatRG8Snorm TextureFormat = 0x0000000B
	// TextureFormatRG8Uint is two 8-bit unsigned integers.
	TextureFormatRG8Uint TextureFormat = 0x0000000C
	// TextureFormatRG8Sint is two 8-bit signed integers.
	TextureFormatRG8Sint TextureFormat = 0x0000000D

	// 32-bit formats
	// TextureFormatR32Float is a single 32-bit float.
	TextureFormatR32Float TextureFormat = 0x0000000E
	// TextureFormatR32Uint is a single 32-bit unsigned integer.
	TextureFormatR32Uint TextureFormat = 0x0000000F
	// TextureFormatR32Sint is a single 32-bit signed integer.
	TextureFormatR32Sint TextureFormat = 0x00000010
	// TextureFormatRG16Unorm is two 16-bit normalized unsigned integers.
	TextureFormatRG16Unorm TextureFormat = 0x00000011
	// TextureFormatRG16Snorm is two 16-bit normalized signed integers.
	TextureFormatRG16Snorm TextureFormat = 0x00000012
	// TextureFormatRG16Uint is two 16-bit unsigned integers.
	TextureFormatRG16Uint TextureFormat = 0x00000013
	// TextureFormatRG16Sint is two 16-bit signed integers.
	TextureFormatRG16Sint TextureFormat = 0x00000014
	// TextureFormatRG16Float is two 16-bit floats.
	TextureFormatRG16Float TextureFormat = 0x00000015
	// TextureFormatRGBA8Unorm is four 8-bit normalized unsigned integers.
	TextureFormatRGBA8Unorm TextureFormat = 0x00000016
	// TextureFormatRGBA8UnormSrgb is four 8-bit normalized unsigned integers in sRGB.
	TextureFormatRGBA8UnormSrgb TextureFormat = 0x00000017
	// TextureFormatRGBA8Snorm is four 8-bit normalized signed integers.
	TextureFormatRGBA8Snorm TextureFormat = 0x00000018
	// TextureFormatRGBA8Uint is four 8-bit unsigned integers.
	TextureFormatRGBA8Uint TextureFormat = 0x00000019
	// TextureFormatRGBA8Sint is four 8-bit signed integers.
	TextureFormatRGBA8Sint TextureFormat = 0x0000001A
	// TextureFormatBGRA8Unorm is four 8-bit normalized unsigned integers (BGRA order).
	TextureFormatBGRA8Unorm TextureFormat = 0x0000001B
	// TextureFormatBGRA8UnormSrgb is four 8-bit normalized unsigned integers in sRGB (BGRA order).
	TextureFormatBGRA8UnormSrgb TextureFormat = 0x0000001C

	// Packed 32-bit formats
	// TextureFormatRGB10A2Uint is a packed 10-10-10-2 unsigned integer format.
	TextureFormatRGB10A2Uint TextureFormat = 0x0000001D
	// TextureFormatRGB10A2Unorm is a packed 10-10-10-2 normalized unsigned format.
	TextureFormatRGB10A2Unorm TextureFormat = 0x0000001E
	// TextureFormatRG11B10Ufloat is a packed 11-11-10 unsigned float format.
	TextureFormatRG11B10Ufloat TextureFormat = 0x0000001F
	// TextureFormatRGB9E5Ufloat is a packed 9-9-9-5 unsigned float format.
	TextureFormatRGB9E5Ufloat TextureFormat = 0x00000020

	// 64-bit formats
	// TextureFormatRG32Float is two 32-bit floats.
	TextureFormatRG32Float TextureFormat = 0x00000021
	// TextureFormatRG32Uint is two 32-bit unsigned integers.
	TextureFormatRG32Uint TextureFormat = 0x00000022
	// TextureFormatRG32Sint is two 32-bit signed integers.
	TextureFormatRG32Sint TextureFormat = 0x00000023
	// TextureFormatRGBA16Unorm is four 16-bit normalized unsigned integers.
	TextureFormatRGBA16Unorm TextureFormat = 0x00000024
	// TextureFormatRGBA16Snorm is four 16-bit normalized signed integers.
	TextureFormatRGBA16Snorm TextureFormat = 0x00000025
	// TextureFormatRGBA16Uint is four 16-bit unsigned integers.
	TextureFormatRGBA16Uint TextureFormat = 0x00000026
	// TextureFormatRGBA16Sint is four 16-bit signed integers.
	TextureFormatRGBA16Sint TextureFormat = 0x00000027
	// TextureFormatRGBA16Float is four 16-bit floats.
	TextureFormatRGBA16Float TextureFormat = 0x00000028

	// 128-bit formats
	// TextureFormatRGBA32Float is four 32-bit floats.
	TextureFormatRGBA32Float TextureFormat = 0x00000029
	// TextureFormatRGBA32Uint is four 32-bit unsigned integers.
	TextureFormatRGBA32Uint TextureFormat = 0x0000002A
	// TextureFormatRGBA32Sint is four 32-bit signed integers.
	TextureFormatRGBA32Sint TextureFormat = 0x0000002B

	// Depth/stencil formats
	// TextureFormatStencil8 is an 8-bit stencil format.
	TextureFormatStencil8 TextureFormat = 0x0000002C
	// TextureFormatDepth16Unorm is a 16-bit normalized depth format.
	TextureFormatDepth16Unorm TextureFormat = 0x0000002D
	// TextureFormatDepth24Plus is a 24-bit depth format (may be 32-bit internally).
	TextureFormatDepth24Plus TextureFormat = 0x0000002E
	// TextureFormatDepth24PlusStencil8 is a 24-bit depth + 8-bit stencil format.
	TextureFormatDepth24PlusStencil8 TextureFormat = 0x0000002F
	// TextureFormatDepth32Float is a 32-bit float depth format.
	TextureFormatDepth32Float TextureFormat = 0x00000030
	// TextureFormatDepth32FloatStencil8 is a 32-bit float depth + 8-bit stencil format.
	TextureFormatDepth32FloatStencil8 TextureFormat = 0x00000031

	// BC compressed formats (requires TextureCompressionBC feature)
	// webgpu.h: BC formats start at 0x32 (after depth/stencil formats)
	// TextureFormatBC1RGBAUnorm is BC1 RGBA normalized unsigned format.
	TextureFormatBC1RGBAUnorm TextureFormat = 0x00000032
	// TextureFormatBC1RGBAUnormSrgb is BC1 RGBA normalized unsigned sRGB format.
	TextureFormatBC1RGBAUnormSrgb TextureFormat = 0x00000033
	// TextureFormatBC2RGBAUnorm is BC2 RGBA normalized unsigned format.
	TextureFormatBC2RGBAUnorm TextureFormat = 0x00000034
	// TextureFormatBC2RGBAUnormSrgb is BC2 RGBA normalized unsigned sRGB format.
	TextureFormatBC2RGBAUnormSrgb TextureFormat = 0x00000035
	// TextureFormatBC3RGBAUnorm is BC3 RGBA normalized unsigned format.
	TextureFormatBC3RGBAUnorm TextureFormat = 0x00000036
	// TextureFormatBC3RGBAUnormSrgb is BC3 RGBA normalized unsigned sRGB format.
	TextureFormatBC3RGBAUnormSrgb TextureFormat = 0x00000037
	// TextureFormatBC4RUnorm is BC4 R normalized unsigned format.
	TextureFormatBC4RUnorm TextureFormat = 0x00000038
	// TextureFormatBC4RSnorm is BC4 R normalized signed format.
	TextureFormatBC4RSnorm TextureFormat = 0x00000039
	// TextureFormatBC5RGUnorm is BC5 RG normalized unsigned format.
	TextureFormatBC5RGUnorm TextureFormat = 0x0000003A
	// TextureFormatBC5RGSnorm is BC5 RG normalized signed format.
	TextureFormatBC5RGSnorm TextureFormat = 0x0000003B
	// TextureFormatBC6HRGBUfloat is BC6H RGB unsigned float format.
	TextureFormatBC6HRGBUfloat TextureFormat = 0x0000003C
	// TextureFormatBC6HRGBFloat is BC6H RGB signed float format.
	TextureFormatBC6HRGBFloat TextureFormat = 0x0000003D
	// TextureFormatBC7RGBAUnorm is BC7 RGBA normalized unsigned format.
	TextureFormatBC7RGBAUnorm TextureFormat = 0x0000003E
	// TextureFormatBC7RGBAUnormSrgb is BC7 RGBA normalized unsigned sRGB format.
	TextureFormatBC7RGBAUnormSrgb TextureFormat = 0x0000003F

	// ETC2 compressed formats (requires TextureCompressionETC2 feature)
	// webgpu.h: ETC2 formats start at 0x40 (after BC formats)
	// TextureFormatETC2RGB8Unorm is ETC2 RGB normalized unsigned format.
	TextureFormatETC2RGB8Unorm TextureFormat = 0x00000040
	// TextureFormatETC2RGB8UnormSrgb is ETC2 RGB normalized unsigned sRGB format.
	TextureFormatETC2RGB8UnormSrgb TextureFormat = 0x00000041
	// TextureFormatETC2RGB8A1Unorm is ETC2 RGB with 1-bit alpha normalized unsigned format.
	TextureFormatETC2RGB8A1Unorm TextureFormat = 0x00000042
	// TextureFormatETC2RGB8A1UnormSrgb is ETC2 RGB with 1-bit alpha normalized unsigned sRGB format.
	TextureFormatETC2RGB8A1UnormSrgb TextureFormat = 0x00000043
	// TextureFormatETC2RGBA8Unorm is ETC2 RGBA normalized unsigned format.
	TextureFormatETC2RGBA8Unorm TextureFormat = 0x00000044
	// TextureFormatETC2RGBA8UnormSrgb is ETC2 RGBA normalized unsigned sRGB format.
	TextureFormatETC2RGBA8UnormSrgb TextureFormat = 0x00000045
	// TextureFormatEACR11Unorm is EAC R normalized unsigned format.
	TextureFormatEACR11Unorm TextureFormat = 0x00000046
	// TextureFormatEACR11Snorm is EAC R normalized signed format.
	TextureFormatEACR11Snorm TextureFormat = 0x00000047
	// TextureFormatEACRG11Unorm is EAC RG normalized unsigned format.
	TextureFormatEACRG11Unorm TextureFormat = 0x00000048
	// TextureFormatEACRG11Snorm is EAC RG normalized signed format.
	TextureFormatEACRG11Snorm TextureFormat = 0x00000049

	// ASTC compressed formats (requires TextureCompressionASTC feature)
	// webgpu.h: ASTC formats start at 0x4A (after ETC2 formats)
	// TextureFormatASTC4x4Unorm is ASTC 4x4 normalized unsigned format.
	TextureFormatASTC4x4Unorm TextureFormat = 0x0000004A
	// TextureFormatASTC4x4UnormSrgb is ASTC 4x4 normalized unsigned sRGB format.
	TextureFormatASTC4x4UnormSrgb TextureFormat = 0x0000004B
	// TextureFormatASTC5x4Unorm is ASTC 5x4 normalized unsigned format.
	TextureFormatASTC5x4Unorm TextureFormat = 0x0000004C
	// TextureFormatASTC5x4UnormSrgb is ASTC 5x4 normalized unsigned sRGB format.
	TextureFormatASTC5x4UnormSrgb TextureFormat = 0x0000004D
	// TextureFormatASTC5x5Unorm is ASTC 5x5 normalized unsigned format.
	TextureFormatASTC5x5Unorm TextureFormat = 0x0000004E
	// TextureFormatASTC5x5UnormSrgb is ASTC 5x5 normalized unsigned sRGB format.
	TextureFormatASTC5x5UnormSrgb TextureFormat = 0x0000004F
	// TextureFormatASTC6x5Unorm is ASTC 6x5 normalized unsigned format.
	TextureFormatASTC6x5Unorm TextureFormat = 0x00000050
	// TextureFormatASTC6x5UnormSrgb is ASTC 6x5 normalized unsigned sRGB format.
	TextureFormatASTC6x5UnormSrgb TextureFormat = 0x00000051
	// TextureFormatASTC6x6Unorm is ASTC 6x6 normalized unsigned format.
	TextureFormatASTC6x6Unorm TextureFormat = 0x00000052
	// TextureFormatASTC6x6UnormSrgb is ASTC 6x6 normalized unsigned sRGB format.
	TextureFormatASTC6x6UnormSrgb TextureFormat = 0x00000053
	// TextureFormatASTC8x5Unorm is ASTC 8x5 normalized unsigned format.
	TextureFormatASTC8x5Unorm TextureFormat = 0x00000054
	// TextureFormatASTC8x5UnormSrgb is ASTC 8x5 normalized unsigned sRGB format.
	TextureFormatASTC8x5UnormSrgb TextureFormat = 0x00000055
	// TextureFormatASTC8x6Unorm is ASTC 8x6 normalized unsigned format.
	TextureFormatASTC8x6Unorm TextureFormat = 0x00000056
	// TextureFormatASTC8x6UnormSrgb is ASTC 8x6 normalized unsigned sRGB format.
	TextureFormatASTC8x6UnormSrgb TextureFormat = 0x00000057
	// TextureFormatASTC8x8Unorm is ASTC 8x8 normalized unsigned format.
	TextureFormatASTC8x8Unorm TextureFormat = 0x00000058
	// TextureFormatASTC8x8UnormSrgb is ASTC 8x8 normalized unsigned sRGB format.
	TextureFormatASTC8x8UnormSrgb TextureFormat = 0x00000059
	// TextureFormatASTC10x5Unorm is ASTC 10x5 normalized unsigned format.
	TextureFormatASTC10x5Unorm TextureFormat = 0x0000005A
	// TextureFormatASTC10x5UnormSrgb is ASTC 10x5 normalized unsigned sRGB format.
	TextureFormatASTC10x5UnormSrgb TextureFormat = 0x0000005B
	// TextureFormatASTC10x6Unorm is ASTC 10x6 normalized unsigned format.
	TextureFormatASTC10x6Unorm TextureFormat = 0x0000005C
	// TextureFormatASTC10x6UnormSrgb is ASTC 10x6 normalized unsigned sRGB format.
	TextureFormatASTC10x6UnormSrgb TextureFormat = 0x0000005D
	// TextureFormatASTC10x8Unorm is ASTC 10x8 normalized unsigned format.
	TextureFormatASTC10x8Unorm TextureFormat = 0x0000005E
	// TextureFormatASTC10x8UnormSrgb is ASTC 10x8 normalized unsigned sRGB format.
	TextureFormatASTC10x8UnormSrgb TextureFormat = 0x0000005F
	// TextureFormatASTC10x10Unorm is ASTC 10x10 normalized unsigned format.
	TextureFormatASTC10x10Unorm TextureFormat = 0x00000060
	// TextureFormatASTC10x10UnormSrgb is ASTC 10x10 normalized unsigned sRGB format.
	TextureFormatASTC10x10UnormSrgb TextureFormat = 0x00000061
	// TextureFormatASTC12x10Unorm is ASTC 12x10 normalized unsigned format.
	TextureFormatASTC12x10Unorm TextureFormat = 0x00000062
	// TextureFormatASTC12x10UnormSrgb is ASTC 12x10 normalized unsigned sRGB format.
	TextureFormatASTC12x10UnormSrgb TextureFormat = 0x00000063
	// TextureFormatASTC12x12Unorm is ASTC 12x12 normalized unsigned format.
	TextureFormatASTC12x12Unorm TextureFormat = 0x00000064
	// TextureFormatASTC12x12UnormSrgb is ASTC 12x12 normalized unsigned sRGB format.
	TextureFormatASTC12x12UnormSrgb TextureFormat = 0x00000065
)

// String returns the name of the texture format.
func (f TextureFormat) String() string {
	switch f {
	case TextureFormatUndefined:
		return "Undefined"
	case TextureFormatR8Unorm:
		return "R8Unorm"
	case TextureFormatR8Snorm:
		return "R8Snorm"
	case TextureFormatR8Uint:
		return "R8Uint"
	case TextureFormatR8Sint:
		return "R8Sint"
	case TextureFormatR16Unorm:
		return "R16Unorm"
	case TextureFormatR16Snorm:
		return "R16Snorm"
	case TextureFormatR16Uint:
		return "R16Uint"
	case TextureFormatR16Sint:
		return "R16Sint"
	case TextureFormatR16Float:
		return "R16Float"
	case TextureFormatRG8Unorm:
		return "RG8Unorm"
	case TextureFormatRG8Snorm:
		return "RG8Snorm"
	case TextureFormatRG8Uint:
		return "RG8Uint"
	case TextureFormatRG8Sint:
		return "RG8Sint"
	case TextureFormatR32Float:
		return "R32Float"
	case TextureFormatR32Uint:
		return "R32Uint"
	case TextureFormatR32Sint:
		return "R32Sint"
	case TextureFormatRG16Unorm:
		return "RG16Unorm"
	case TextureFormatRG16Snorm:
		return "RG16Snorm"
	case TextureFormatRG16Uint:
		return "RG16Uint"
	case TextureFormatRG16Sint:
		return "RG16Sint"
	case TextureFormatRG16Float:
		return "RG16Float"
	case TextureFormatRGBA8Unorm:
		return "RGBA8Unorm"
	case TextureFormatRGBA8UnormSrgb:
		return "RGBA8UnormSrgb"
	case TextureFormatRGBA8Snorm:
		return "RGBA8Snorm"
	case TextureFormatRGBA8Uint:
		return "RGBA8Uint"
	case TextureFormatRGBA8Sint:
		return "RGBA8Sint"
	case TextureFormatBGRA8Unorm:
		return "BGRA8Unorm"
	case TextureFormatBGRA8UnormSrgb:
		return "BGRA8UnormSrgb"
	case TextureFormatRGB9E5Ufloat:
		return "RGB9E5Ufloat"
	case TextureFormatRGB10A2Uint:
		return "RGB10A2Uint"
	case TextureFormatRGB10A2Unorm:
		return "RGB10A2Unorm"
	case TextureFormatRG11B10Ufloat:
		return "RG11B10Ufloat"
	case TextureFormatRG32Float:
		return "RG32Float"
	case TextureFormatRG32Uint:
		return "RG32Uint"
	case TextureFormatRG32Sint:
		return "RG32Sint"
	case TextureFormatRGBA16Unorm:
		return "RGBA16Unorm"
	case TextureFormatRGBA16Snorm:
		return "RGBA16Snorm"
	case TextureFormatRGBA16Uint:
		return "RGBA16Uint"
	case TextureFormatRGBA16Sint:
		return "RGBA16Sint"
	case TextureFormatRGBA16Float:
		return "RGBA16Float"
	case TextureFormatRGBA32Float:
		return "RGBA32Float"
	case TextureFormatRGBA32Uint:
		return "RGBA32Uint"
	case TextureFormatRGBA32Sint:
		return "RGBA32Sint"
	case TextureFormatStencil8:
		return "Stencil8"
	case TextureFormatDepth16Unorm:
		return "Depth16Unorm"
	case TextureFormatDepth24Plus:
		return "Depth24Plus"
	case TextureFormatDepth24PlusStencil8:
		return "Depth24PlusStencil8"
	case TextureFormatDepth32Float:
		return "Depth32Float"
	case TextureFormatDepth32FloatStencil8:
		return "Depth32FloatStencil8"
	case TextureFormatBC1RGBAUnorm:
		return "BC1RGBAUnorm"
	case TextureFormatBC1RGBAUnormSrgb:
		return "BC1RGBAUnormSrgb"
	case TextureFormatBC2RGBAUnorm:
		return "BC2RGBAUnorm"
	case TextureFormatBC2RGBAUnormSrgb:
		return "BC2RGBAUnormSrgb"
	case TextureFormatBC3RGBAUnorm:
		return "BC3RGBAUnorm"
	case TextureFormatBC3RGBAUnormSrgb:
		return "BC3RGBAUnormSrgb"
	case TextureFormatBC4RUnorm:
		return "BC4RUnorm"
	case TextureFormatBC4RSnorm:
		return "BC4RSnorm"
	case TextureFormatBC5RGUnorm:
		return "BC5RGUnorm"
	case TextureFormatBC5RGSnorm:
		return "BC5RGSnorm"
	case TextureFormatBC6HRGBUfloat:
		return "BC6HRGBUfloat"
	case TextureFormatBC6HRGBFloat:
		return "BC6HRGBFloat"
	case TextureFormatBC7RGBAUnorm:
		return "BC7RGBAUnorm"
	case TextureFormatBC7RGBAUnormSrgb:
		return "BC7RGBAUnormSrgb"
	case TextureFormatETC2RGB8Unorm:
		return "ETC2RGB8Unorm"
	case TextureFormatETC2RGB8UnormSrgb:
		return "ETC2RGB8UnormSrgb"
	case TextureFormatETC2RGB8A1Unorm:
		return "ETC2RGB8A1Unorm"
	case TextureFormatETC2RGB8A1UnormSrgb:
		return "ETC2RGB8A1UnormSrgb"
	case TextureFormatETC2RGBA8Unorm:
		return "ETC2RGBA8Unorm"
	case TextureFormatETC2RGBA8UnormSrgb:
		return "ETC2RGBA8UnormSrgb"
	case TextureFormatEACR11Unorm:
		return "EACR11Unorm"
	case TextureFormatEACR11Snorm:
		return "EACR11Snorm"
	case TextureFormatEACRG11Unorm:
		return "EACRG11Unorm"
	case TextureFormatEACRG11Snorm:
		return "EACRG11Snorm"
	case TextureFormatASTC4x4Unorm:
		return "ASTC4x4Unorm"
	case TextureFormatASTC4x4UnormSrgb:
		return "ASTC4x4UnormSrgb"
	case TextureFormatASTC5x4Unorm:
		return "ASTC5x4Unorm"
	case TextureFormatASTC5x4UnormSrgb:
		return "ASTC5x4UnormSrgb"
	case TextureFormatASTC5x5Unorm:
		return "ASTC5x5Unorm"
	case TextureFormatASTC5x5UnormSrgb:
		return "ASTC5x5UnormSrgb"
	case TextureFormatASTC6x5Unorm:
		return "ASTC6x5Unorm"
	case TextureFormatASTC6x5UnormSrgb:
		return "ASTC6x5UnormSrgb"
	case TextureFormatASTC6x6Unorm:
		return "ASTC6x6Unorm"
	case TextureFormatASTC6x6UnormSrgb:
		return "ASTC6x6UnormSrgb"
	case TextureFormatASTC8x5Unorm:
		return "ASTC8x5Unorm"
	case TextureFormatASTC8x5UnormSrgb:
		return "ASTC8x5UnormSrgb"
	case TextureFormatASTC8x6Unorm:
		return "ASTC8x6Unorm"
	case TextureFormatASTC8x6UnormSrgb:
		return "ASTC8x6UnormSrgb"
	case TextureFormatASTC8x8Unorm:
		return "ASTC8x8Unorm"
	case TextureFormatASTC8x8UnormSrgb:
		return "ASTC8x8UnormSrgb"
	case TextureFormatASTC10x5Unorm:
		return "ASTC10x5Unorm"
	case TextureFormatASTC10x5UnormSrgb:
		return "ASTC10x5UnormSrgb"
	case TextureFormatASTC10x6Unorm:
		return "ASTC10x6Unorm"
	case TextureFormatASTC10x6UnormSrgb:
		return "ASTC10x6UnormSrgb"
	case TextureFormatASTC10x8Unorm:
		return "ASTC10x8Unorm"
	case TextureFormatASTC10x8UnormSrgb:
		return "ASTC10x8UnormSrgb"
	case TextureFormatASTC10x10Unorm:
		return "ASTC10x10Unorm"
	case TextureFormatASTC10x10UnormSrgb:
		return "ASTC10x10UnormSrgb"
	case TextureFormatASTC12x10Unorm:
		return "ASTC12x10Unorm"
	case TextureFormatASTC12x10UnormSrgb:
		return "ASTC12x10UnormSrgb"
	case TextureFormatASTC12x12Unorm:
		return "ASTC12x12Unorm"
	case TextureFormatASTC12x12UnormSrgb:
		return "ASTC12x12UnormSrgb"
	default:
		return "Unknown"
	}
}

// IsDepthStencil returns true if this is a depth or stencil format.
func (f TextureFormat) IsDepthStencil() bool {
	switch f {
	case TextureFormatStencil8,
		TextureFormatDepth16Unorm,
		TextureFormatDepth24Plus,
		TextureFormatDepth24PlusStencil8,
		TextureFormatDepth32Float,
		TextureFormatDepth32FloatStencil8:
		return true
	default:
		return false
	}
}

// HasDepth returns true if this format has a depth component.
func (f TextureFormat) HasDepth() bool {
	switch f {
	case TextureFormatDepth16Unorm,
		TextureFormatDepth24Plus,
		TextureFormatDepth24PlusStencil8,
		TextureFormatDepth32Float,
		TextureFormatDepth32FloatStencil8:
		return true
	default:
		return false
	}
}

// HasStencil returns true if this format has a stencil component.
func (f TextureFormat) HasStencil() bool {
	switch f {
	case TextureFormatStencil8,
		TextureFormatDepth24PlusStencil8,
		TextureFormatDepth32FloatStencil8:
		return true
	default:
		return false
	}
}

// IsSrgb returns true if this is an sRGB format.
func (f TextureFormat) IsSrgb() bool {
	switch f {
	case TextureFormatRGBA8UnormSrgb,
		TextureFormatBGRA8UnormSrgb,
		TextureFormatBC1RGBAUnormSrgb,
		TextureFormatBC2RGBAUnormSrgb,
		TextureFormatBC3RGBAUnormSrgb,
		TextureFormatBC7RGBAUnormSrgb,
		TextureFormatETC2RGB8UnormSrgb,
		TextureFormatETC2RGB8A1UnormSrgb,
		TextureFormatETC2RGBA8UnormSrgb,
		TextureFormatASTC4x4UnormSrgb,
		TextureFormatASTC5x4UnormSrgb,
		TextureFormatASTC5x5UnormSrgb,
		TextureFormatASTC6x5UnormSrgb,
		TextureFormatASTC6x6UnormSrgb,
		TextureFormatASTC8x5UnormSrgb,
		TextureFormatASTC8x6UnormSrgb,
		TextureFormatASTC8x8UnormSrgb,
		TextureFormatASTC10x5UnormSrgb,
		TextureFormatASTC10x6UnormSrgb,
		TextureFormatASTC10x8UnormSrgb,
		TextureFormatASTC10x10UnormSrgb,
		TextureFormatASTC12x10UnormSrgb,
		TextureFormatASTC12x12UnormSrgb:
		return true
	default:
		return false
	}
}

// BlockCopySize returns the number of bytes occupied per texel block for this format.
//
// For uncompressed formats, one block equals one texel.
// For compressed formats (BC, ETC2, ASTC), one block covers multiple texels
// (e.g. 4x4 for BC/ETC2, variable for ASTC), but the returned value is
// the byte size of that block.
//
// Returns 0 for formats whose copy size is implementation-defined:
//   - Depth24Plus
//   - Depth24PlusStencil8
//   - Depth32FloatStencil8
//
// Returns 0 for unknown or invalid formats (including TextureFormatUndefined).
//
// Reference: Rust wgpu-types TextureFormat::block_copy_size().
func (f TextureFormat) BlockCopySize() uint32 {
	switch f {
	// 8-bit formats (1 byte per texel)
	case TextureFormatR8Unorm,
		TextureFormatR8Snorm,
		TextureFormatR8Uint,
		TextureFormatR8Sint,
		TextureFormatStencil8:
		return 1

	// 16-bit formats (2 bytes per texel)
	case TextureFormatR16Unorm,
		TextureFormatR16Snorm,
		TextureFormatR16Uint,
		TextureFormatR16Sint,
		TextureFormatR16Float,
		TextureFormatRG8Unorm,
		TextureFormatRG8Snorm,
		TextureFormatRG8Uint,
		TextureFormatRG8Sint,
		TextureFormatDepth16Unorm:
		return 2

	// 32-bit formats (4 bytes per texel)
	case TextureFormatR32Float,
		TextureFormatR32Uint,
		TextureFormatR32Sint,
		TextureFormatRG16Unorm,
		TextureFormatRG16Snorm,
		TextureFormatRG16Uint,
		TextureFormatRG16Sint,
		TextureFormatRG16Float,
		TextureFormatRGBA8Unorm,
		TextureFormatRGBA8UnormSrgb,
		TextureFormatRGBA8Snorm,
		TextureFormatRGBA8Uint,
		TextureFormatRGBA8Sint,
		TextureFormatBGRA8Unorm,
		TextureFormatBGRA8UnormSrgb,
		TextureFormatRGB10A2Uint,
		TextureFormatRGB10A2Unorm,
		TextureFormatRG11B10Ufloat,
		TextureFormatRGB9E5Ufloat,
		TextureFormatDepth32Float:
		return 4

	// 64-bit formats (8 bytes per texel)
	case TextureFormatRG32Float,
		TextureFormatRG32Uint,
		TextureFormatRG32Sint,
		TextureFormatRGBA16Unorm,
		TextureFormatRGBA16Snorm,
		TextureFormatRGBA16Uint,
		TextureFormatRGBA16Sint,
		TextureFormatRGBA16Float:
		return 8

	// 128-bit formats (16 bytes per texel)
	case TextureFormatRGBA32Float,
		TextureFormatRGBA32Uint,
		TextureFormatRGBA32Sint:
		return 16

	// Depth/stencil formats with implementation-defined copy size
	case TextureFormatDepth24Plus,
		TextureFormatDepth24PlusStencil8,
		TextureFormatDepth32FloatStencil8:
		return 0

	// BC compressed formats — 8 bytes per 4x4 block
	case TextureFormatBC1RGBAUnorm,
		TextureFormatBC1RGBAUnormSrgb,
		TextureFormatBC4RUnorm,
		TextureFormatBC4RSnorm:
		return 8

	// BC compressed formats — 16 bytes per 4x4 block
	case TextureFormatBC2RGBAUnorm,
		TextureFormatBC2RGBAUnormSrgb,
		TextureFormatBC3RGBAUnorm,
		TextureFormatBC3RGBAUnormSrgb,
		TextureFormatBC5RGUnorm,
		TextureFormatBC5RGSnorm,
		TextureFormatBC6HRGBUfloat,
		TextureFormatBC6HRGBFloat,
		TextureFormatBC7RGBAUnorm,
		TextureFormatBC7RGBAUnormSrgb:
		return 16

	// ETC2/EAC compressed formats — 8 bytes per 4x4 block
	case TextureFormatETC2RGB8Unorm,
		TextureFormatETC2RGB8UnormSrgb,
		TextureFormatETC2RGB8A1Unorm,
		TextureFormatETC2RGB8A1UnormSrgb,
		TextureFormatEACR11Unorm,
		TextureFormatEACR11Snorm:
		return 8

	// ETC2/EAC compressed formats — 16 bytes per 4x4 block
	case TextureFormatETC2RGBA8Unorm,
		TextureFormatETC2RGBA8UnormSrgb,
		TextureFormatEACRG11Unorm,
		TextureFormatEACRG11Snorm:
		return 16

	// ASTC compressed formats — all 16 bytes per block (variable block dimensions)
	case TextureFormatASTC4x4Unorm,
		TextureFormatASTC4x4UnormSrgb,
		TextureFormatASTC5x4Unorm,
		TextureFormatASTC5x4UnormSrgb,
		TextureFormatASTC5x5Unorm,
		TextureFormatASTC5x5UnormSrgb,
		TextureFormatASTC6x5Unorm,
		TextureFormatASTC6x5UnormSrgb,
		TextureFormatASTC6x6Unorm,
		TextureFormatASTC6x6UnormSrgb,
		TextureFormatASTC8x5Unorm,
		TextureFormatASTC8x5UnormSrgb,
		TextureFormatASTC8x6Unorm,
		TextureFormatASTC8x6UnormSrgb,
		TextureFormatASTC8x8Unorm,
		TextureFormatASTC8x8UnormSrgb,
		TextureFormatASTC10x5Unorm,
		TextureFormatASTC10x5UnormSrgb,
		TextureFormatASTC10x6Unorm,
		TextureFormatASTC10x6UnormSrgb,
		TextureFormatASTC10x8Unorm,
		TextureFormatASTC10x8UnormSrgb,
		TextureFormatASTC10x10Unorm,
		TextureFormatASTC10x10UnormSrgb,
		TextureFormatASTC12x10Unorm,
		TextureFormatASTC12x10UnormSrgb,
		TextureFormatASTC12x12Unorm,
		TextureFormatASTC12x12UnormSrgb:
		return 16

	default:
		return 0
	}
}

// TextureDimension describes texture dimensions.
type TextureDimension uint32

const (
	// TextureDimensionUndefined is an undefined texture dimension (invalid).
	TextureDimensionUndefined TextureDimension = 0x00000000
	// TextureDimension1D is a 1D texture.
	TextureDimension1D TextureDimension = 0x00000001
	// TextureDimension2D is a 2D texture.
	TextureDimension2D TextureDimension = 0x00000002
	// TextureDimension3D is a 3D texture.
	TextureDimension3D TextureDimension = 0x00000003
)

// String returns the dimension name.
func (d TextureDimension) String() string {
	switch d {
	case TextureDimensionUndefined:
		return "Undefined"
	case TextureDimension1D:
		return "1D"
	case TextureDimension2D:
		return "2D"
	case TextureDimension3D:
		return "3D"
	default:
		return "Unknown"
	}
}

// TextureViewDimension describes a texture view dimension.
type TextureViewDimension uint32

const (
	// TextureViewDimensionUndefined uses the same dimension as the texture.
	TextureViewDimensionUndefined TextureViewDimension = 0x00000000
	// TextureViewDimension1D is a 1D texture view.
	TextureViewDimension1D TextureViewDimension = 0x00000001
	// TextureViewDimension2D is a 2D texture view.
	TextureViewDimension2D TextureViewDimension = 0x00000002
	// TextureViewDimension2DArray is a 2D array texture view.
	TextureViewDimension2DArray TextureViewDimension = 0x00000003
	// TextureViewDimensionCube is a cube texture view.
	TextureViewDimensionCube TextureViewDimension = 0x00000004
	// TextureViewDimensionCubeArray is a cube array texture view.
	TextureViewDimensionCubeArray TextureViewDimension = 0x00000005
	// TextureViewDimension3D is a 3D texture view.
	TextureViewDimension3D TextureViewDimension = 0x00000006
)

// String returns the view dimension name.
func (d TextureViewDimension) String() string {
	switch d {
	case TextureViewDimensionUndefined:
		return "Undefined"
	case TextureViewDimension1D:
		return "1D"
	case TextureViewDimension2D:
		return "2D"
	case TextureViewDimension2DArray:
		return "2DArray"
	case TextureViewDimensionCube:
		return "Cube"
	case TextureViewDimensionCubeArray:
		return "CubeArray"
	case TextureViewDimension3D:
		return "3D"
	default:
		return "Unknown"
	}
}

// TextureAspect describes which aspects of a texture to access.
type TextureAspect uint32

const (
	// TextureAspectUndefined is an undefined texture aspect (invalid).
	TextureAspectUndefined TextureAspect = 0x00000000
	// TextureAspectAll accesses all aspects (default).
	TextureAspectAll TextureAspect = 0x00000001
	// TextureAspectStencilOnly accesses only the stencil aspect.
	TextureAspectStencilOnly TextureAspect = 0x00000002
	// TextureAspectDepthOnly accesses only the depth aspect.
	TextureAspectDepthOnly TextureAspect = 0x00000003
)

// String returns the aspect name.
func (a TextureAspect) String() string {
	switch a {
	case TextureAspectUndefined:
		return "Undefined"
	case TextureAspectAll:
		return "All"
	case TextureAspectStencilOnly:
		return "StencilOnly"
	case TextureAspectDepthOnly:
		return "DepthOnly"
	default:
		return "Unknown"
	}
}

// TextureUsage describes how a texture can be used.
//
// This is a bit flag type. Combine multiple usages with bitwise OR.
type TextureUsage uint64

const (
	// TextureUsageNone indicates no usage (invalid for most operations).
	TextureUsageNone TextureUsage = 0x0000000000000000
	// TextureUsageCopySrc allows the texture to be a copy source.
	TextureUsageCopySrc TextureUsage = 0x0000000000000001
	// TextureUsageCopyDst allows the texture to be a copy destination.
	TextureUsageCopyDst TextureUsage = 0x0000000000000002
	// TextureUsageTextureBinding allows the texture to be bound as a sampled texture.
	TextureUsageTextureBinding TextureUsage = 0x0000000000000004
	// TextureUsageStorageBinding allows the texture to be bound as a storage texture.
	TextureUsageStorageBinding TextureUsage = 0x0000000000000008
	// TextureUsageRenderAttachment allows the texture to be used as a render attachment.
	TextureUsageRenderAttachment TextureUsage = 0x0000000000000010
)

// textureUsageAll is a mask of all valid texture usage flags.
const textureUsageAll = TextureUsageCopySrc | TextureUsageCopyDst |
	TextureUsageTextureBinding | TextureUsageStorageBinding |
	TextureUsageRenderAttachment

// Contains returns true if the usage includes the given flag.
func (u TextureUsage) Contains(flag TextureUsage) bool {
	return u&flag == flag
}

// ContainsUnknownBits returns true if the usage contains any unknown flags.
func (u TextureUsage) ContainsUnknownBits() bool {
	return u&^textureUsageAll != 0
}

// TextureDescriptor describes a texture.
type TextureDescriptor struct {
	// Label is an optional debug label.
	Label string
	// Size is the texture size.
	Size Extent3D
	// MipLevelCount is the number of mip levels (1 for no mipmapping).
	MipLevelCount uint32
	// SampleCount is the number of samples (1 for non-multisampled).
	SampleCount uint32
	// Dimension is the texture dimension.
	Dimension TextureDimension
	// Format is the texture format.
	Format TextureFormat
	// Usage describes how the texture will be used.
	Usage TextureUsage
	// ViewFormats lists compatible view formats (optional).
	ViewFormats []TextureFormat
}

// TextureViewDescriptor describes a texture view.
type TextureViewDescriptor struct {
	// Label is an optional debug label.
	Label string
	// Format is the view format (defaults to texture format if Undefined).
	Format TextureFormat
	// Dimension is the view dimension (defaults to match texture if Undefined).
	Dimension TextureViewDimension
	// Aspect specifies which aspect to view.
	Aspect TextureAspect
	// BaseMipLevel is the first mip level accessible to the view.
	BaseMipLevel uint32
	// MipLevelCount is the number of mip levels accessible (0 for all remaining).
	MipLevelCount uint32
	// BaseArrayLayer is the first array layer accessible to the view.
	BaseArrayLayer uint32
	// ArrayLayerCount is the number of array layers accessible (0 for all remaining).
	ArrayLayerCount uint32
}

// TextureSampleType describes the sample type of a texture.
type TextureSampleType uint32

const (
	// TextureSampleTypeUndefined is an undefined sample type (invalid).
	TextureSampleTypeUndefined TextureSampleType = 0x00000000
	// TextureSampleTypeFloat samples as filterable floating-point.
	TextureSampleTypeFloat TextureSampleType = 0x00000001
	// TextureSampleTypeUnfilterableFloat samples as unfilterable floating-point.
	TextureSampleTypeUnfilterableFloat TextureSampleType = 0x00000002
	// TextureSampleTypeDepth samples as depth comparison.
	TextureSampleTypeDepth TextureSampleType = 0x00000003
	// TextureSampleTypeSint samples as signed integer.
	TextureSampleTypeSint TextureSampleType = 0x00000004
	// TextureSampleTypeUint samples as unsigned integer.
	TextureSampleTypeUint TextureSampleType = 0x00000005
)

// String returns the sample type name.
func (t TextureSampleType) String() string {
	switch t {
	case TextureSampleTypeUndefined:
		return "Undefined"
	case TextureSampleTypeFloat:
		return "Float"
	case TextureSampleTypeUnfilterableFloat:
		return "UnfilterableFloat"
	case TextureSampleTypeDepth:
		return "Depth"
	case TextureSampleTypeSint:
		return "Sint"
	case TextureSampleTypeUint:
		return "Uint"
	default:
		return "Unknown"
	}
}

// ImageCopyTexture describes a texture copy source or destination.
type ImageCopyTexture struct {
	// Texture is a handle to the texture (implementation-specific).
	Texture uintptr
	// MipLevel is the mip level to copy.
	MipLevel uint32
	// Origin is the origin of the copy region in the texture.
	Origin Origin3D
	// Aspect is the aspect of the texture to copy.
	Aspect TextureAspect
}

// TextureDataLayout describes the layout of texture data in memory.
type TextureDataLayout struct {
	// Offset is the offset in bytes from the start of the data.
	Offset uint64
	// BytesPerRow is the number of bytes per row of texture data.
	// Must be a multiple of 256 for buffer-to-texture copies.
	BytesPerRow uint32
	// RowsPerImage is the number of rows per image for 3D textures.
	RowsPerImage uint32
}
