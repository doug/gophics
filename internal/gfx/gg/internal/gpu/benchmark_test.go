//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gg/scene"
)

// Memory Manager Benchmarks
// =============================================================================

// BenchmarkMemoryAllocation benchmarks texture allocation.
func BenchmarkMemoryAllocation(b *testing.B) {
	sizes := []struct {
		name          string
		width, height int
	}{
		{"256x256", 256, 256},
		{"512x512", 512, 512},
		{"1024x1024", 1024, 1024},
	}

	for _, sz := range sizes {
		b.Run(sz.name, func(b *testing.B) {
			mm := NewMemoryManager(nil, MemoryManagerConfig{
				MaxMemoryMB: 512,
			})
			defer mm.Close()

			config := TextureConfig{
				Width:  sz.width,
				Height: sz.height,
				Format: TextureFormatRGBA8,
			}

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tex, err := mm.AllocTexture(config)
				if err != nil {
					b.Fatalf("Allocation failed: %v", err)
				}
				_ = mm.FreeTexture(tex)
			}
		})
	}
}

// BenchmarkMemoryTouch benchmarks LRU touch operations.
func BenchmarkMemoryTouch(b *testing.B) {
	mm := NewMemoryManager(nil, MemoryManagerConfig{
		MaxMemoryMB: 128,
	})
	defer mm.Close()

	// Allocate some textures
	var textures []*GPUTexture
	for i := 0; i < 50; i++ {
		tex, err := mm.AllocTexture(TextureConfig{
			Width:  128,
			Height: 128,
			Format: TextureFormatRGBA8,
		})
		if err != nil {
			break
		}
		textures = append(textures, tex)
	}

	if len(textures) == 0 {
		b.Skip("No textures allocated")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		mm.TouchTexture(textures[i%len(textures)])
	}
}

// =============================================================================
// Full Scene Benchmarks
// =============================================================================

// BenchmarkSceneCreation benchmarks scene creation and encoding.
func BenchmarkSceneCreation(b *testing.B) {
	benchmarks := []struct {
		name  string
		setup func(s *scene.Scene)
	}{
		{"single_rect", func(s *scene.Scene) {
			rect := scene.NewRectShape(10, 10, 80, 80)
			s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)
		}},
		{"10_rects", func(s *scene.Scene) {
			for i := 0; i < 10; i++ {
				rect := scene.NewRectShape(float32(i*10), float32(i*10), 50, 50)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)
			}
		}},
		{"100_rects", func(s *scene.Scene) {
			for i := 0; i < 100; i++ {
				x := float32((i % 10) * 50)
				y := float32((i / 10) * 50)
				rect := scene.NewRectShape(x, y, 40, 40)
				s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)
			}
		}},
		{"with_layers", func(s *scene.Scene) {
			rect := scene.NewRectShape(0, 0, 100, 100)
			s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.White), rect)

			s.PushLayer(scene.BlendMultiply, 0.8, nil)
			circle := scene.NewCircleShape(50, 50, 30)
			s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), circle)
			s.PopLayer()
		}},
	}

	for _, bm := range benchmarks {
		b.Run(bm.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				s := scene.NewScene()
				bm.setup(s)
				_ = s.Encoding()
			}
		})
	}
}

// BenchmarkBackendInit benchmarks backend initialization.
func BenchmarkBackendInit(b *testing.B) {
	for i := 0; i < b.N; i++ {
		be := NewBackend()
		if err := be.Init(); err != nil {
			b.Skipf("GPU not available: %v", err)
		}
		be.Close()
	}
}
