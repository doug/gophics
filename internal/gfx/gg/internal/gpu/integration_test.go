//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/wgpu/core"
)

// =============================================================================
// Memory Integration Tests
// =============================================================================

// TestMemoryIntegration tests texture allocation under load.
func TestMemoryIntegration(t *testing.T) {
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB:       64,
		EvictionThreshold: 0.8,
	})
	defer mm.Close()

	// Allocate multiple textures
	var textures []*GPUTexture
	for i := 0; i < 20; i++ {
		tex, err := mm.AllocTexture(TextureConfig{
			Width:  256,
			Height: 256,
			Format: TextureFormatRGBA8,
			Label:  "test-texture",
		})
		if err != nil {
			t.Logf("Allocation %d failed: %v (may be budget limit)", i, err)
			break
		}
		textures = append(textures, tex)
	}

	if len(textures) == 0 {
		t.Fatal("Should have allocated at least one texture")
	}

	t.Logf("Allocated %d textures", len(textures))

	// Check stats
	stats := mm.Stats()
	t.Logf("Memory stats: %s", stats.String())

	if stats.TextureCount != len(textures) {
		t.Errorf("TextureCount = %d, want %d", stats.TextureCount, len(textures))
	}

	// Touch some textures (simulating use)
	for i := 0; i < len(textures) && i < 5; i++ {
		mm.TouchTexture(textures[i])
	}

	// Free half
	for i := 0; i < len(textures)/2; i++ {
		if err := mm.FreeTexture(textures[i]); err != nil {
			t.Errorf("FreeTexture failed: %v", err)
		}
	}

	// Check stats again
	stats = mm.Stats()
	expectedCount := len(textures) - len(textures)/2
	if stats.TextureCount != expectedCount {
		t.Errorf("After free TextureCount = %d, want %d",
			stats.TextureCount, expectedCount)
	}
}

// TestMemoryWithEviction tests that LRU eviction works correctly.
func TestMemoryWithEviction(t *testing.T) {
	// Small budget to force eviction
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB:       16,
		EvictionThreshold: 0.5, // 8 MB threshold
	})
	defer mm.Close()

	// Track allocations
	allocatedCount := 0

	// Allocate until we hit budget - with LRU eviction, older allocations
	// may be evicted to make room for new ones
	for i := 0; i < 20; i++ {
		_, err := mm.AllocTexture(TextureConfig{
			Width:  512,
			Height: 512,
			Format: TextureFormatRGBA8, // 1 MB each
		})
		if err != nil {
			t.Logf("Allocation %d failed (expected at budget): %v", i, err)
			break
		}
		allocatedCount++
	}

	stats := mm.Stats()
	t.Logf("After filling: %s", stats.String())

	// Verify that the used memory never exceeds budget
	if stats.UsedBytes > stats.TotalBytes {
		t.Errorf("Used bytes %d exceeds total budget %d",
			stats.UsedBytes, stats.TotalBytes)
	}

	// Verify we have reasonable texture count (16 max due to 1MB each in 16MB budget)
	if stats.TextureCount > 16 {
		t.Errorf("Texture count %d exceeds budget capacity of 16", stats.TextureCount)
	}

	// Log eviction stats
	if stats.EvictionCount > 0 {
		t.Logf("Eviction occurred: %d textures evicted", stats.EvictionCount)
	}
}

// DeviceID is a helper type to get zero value for tests.
type DeviceID struct{}

// Zero returns a zero-value core.DeviceID.
func (d *DeviceID) Zero() core.DeviceID {
	var id core.DeviceID
	return id
}
