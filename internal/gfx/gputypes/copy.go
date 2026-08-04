// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package gputypes

// ImageSubresourceRange describes a range of subresources within a texture.
//
// Used for partial texture operations and views.
type ImageSubresourceRange struct {
	// Aspect of the texture to access.
	// Color textures must use TextureAspectAll.
	Aspect TextureAspect

	// BaseMipLevel is the first mip level in the range.
	BaseMipLevel uint32

	// MipLevelCount is the number of mip levels.
	// If nil, includes all remaining mip levels (at least 1).
	MipLevelCount *uint32

	// BaseArrayLayer is the first array layer in the range.
	BaseArrayLayer uint32

	// ArrayLayerCount is the number of array layers.
	// If nil, includes all remaining layers (at least 1).
	ArrayLayerCount *uint32
}

// IsFullResource checks if this range covers the entire texture resource.
func (r ImageSubresourceRange) IsFullResource(mipLevelCount, arrayLayerCount uint32) bool {
	if r.BaseMipLevel != 0 || r.BaseArrayLayer != 0 {
		return false
	}

	// Check mip level coverage
	if r.MipLevelCount != nil && *r.MipLevelCount != mipLevelCount {
		return false
	}

	// Check array layer coverage
	if r.ArrayLayerCount != nil && *r.ArrayLayerCount != arrayLayerCount {
		return false
	}

	return true
}
