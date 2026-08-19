//go:build !nogpu

package gpu

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gg/scene"
)

// Kept from renderer_test.go when the placeholder pipeline cluster was deleted.
// This exercises scene encoding and tag decoding, which are unrelated to the
// removed GPUSceneRenderer -- it never referenced it.
func TestSceneDecoderIntegration(t *testing.T) {
	// Create scene with various commands
	s := scene.NewScene()

	// Transform
	s.PushTransform(scene.TranslateAffine(100, 100))

	// Fill
	rect := scene.NewRectShape(0, 0, 50, 50)
	s.Fill(scene.FillNonZero, scene.IdentityAffine(), scene.SolidBrush(gg.Red), rect)

	// Stroke
	line := scene.NewLineShape(0, 0, 100, 100)
	s.Stroke(scene.DefaultStrokeStyle(), scene.IdentityAffine(), scene.SolidBrush(gg.Blue), line)

	// Pop transform
	s.PopTransform()

	// Layer
	s.PushLayer(scene.BlendScreen, 0.8, nil)
	circle := scene.NewCircleShape(75, 75, 30)
	s.Fill(scene.FillEvenOdd, scene.IdentityAffine(), scene.SolidBrush(gg.Green), circle)
	s.PopLayer()

	// Get encoding and verify
	enc := s.Encoding()

	// Count tags
	tagCounts := make(map[scene.Tag]int)
	for _, tag := range enc.Tags() {
		tagCounts[tag]++
	}

	// Should have various commands
	if tagCounts[scene.TagFill] == 0 {
		t.Error("Expected Fill commands")
	}
	if tagCounts[scene.TagStroke] == 0 {
		t.Error("Expected Stroke commands")
	}
	if tagCounts[scene.TagPushLayer] == 0 {
		t.Error("Expected PushLayer commands")
	}
	if tagCounts[scene.TagPopLayer] == 0 {
		t.Error("Expected PopLayer commands")
	}
}
