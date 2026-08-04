package gg

import (
	"testing"
)

// pixelByte returns a channel value as byte [0,255] for threshold checks.
func pixelByte(v float64) int {
	b := int(v*255.0 + 0.5)
	if b < 0 {
		return 0
	}
	if b > 255 {
		return 255
	}
	return b
}

// TestBlendMode_Multiply_AffectsPixels verifies that BlendMultiply produces
// the correct visual result: red * blue = black (1*0=0 for each non-matching channel).
func TestBlendMode_Multiply_AffectsPixels(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	// Draw red background.
	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	// Draw blue rect with Multiply blend.
	dc.SetBlendMode(BlendMultiply)
	dc.SetRGB(0, 0, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	// Multiply(red, blue): R=1*0=0, G=0*0=0, B=0*1=0 -> black.
	p := dc.pixmap.GetPixel(50, 50)
	if pixelByte(p.R) > 5 {
		t.Errorf("BlendMultiply red*blue: R should be ~0, got %d", pixelByte(p.R))
	}
	if pixelByte(p.G) > 5 {
		t.Errorf("BlendMultiply red*blue: G should be ~0, got %d", pixelByte(p.G))
	}
	if pixelByte(p.B) > 5 {
		t.Errorf("BlendMultiply red*blue: B should be ~0, got %d", pixelByte(p.B))
	}
}

// TestBlendMode_Screen_AffectsPixels verifies Screen(red, blue) = magenta.
func TestBlendMode_Screen_AffectsPixels(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendScreen)
	dc.SetRGB(0, 0, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	if pixelByte(p.R) < 250 {
		t.Errorf("BlendScreen red+blue: R should be ~255, got %d", pixelByte(p.R))
	}
	if pixelByte(p.G) > 5 {
		t.Errorf("BlendScreen red+blue: G should be ~0, got %d", pixelByte(p.G))
	}
	if pixelByte(p.B) < 250 {
		t.Errorf("BlendScreen red+blue: B should be ~255, got %d", pixelByte(p.B))
	}
}

// TestBlendMode_Darken_AffectsPixels verifies Darken: min per channel.
func TestBlendMode_Darken_AffectsPixels(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendDarken)
	dc.SetRGB(0, 1, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	if pixelByte(p.R) > 5 || pixelByte(p.G) > 5 || pixelByte(p.B) > 5 {
		t.Errorf("BlendDarken red,green: expected ~black, got (%d,%d,%d)",
			pixelByte(p.R), pixelByte(p.G), pixelByte(p.B))
	}
}

// TestBlendMode_Lighten_AffectsPixels verifies Lighten: max per channel.
func TestBlendMode_Lighten_AffectsPixels(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendLighten)
	dc.SetRGB(0, 1, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	if pixelByte(p.R) < 250 {
		t.Errorf("BlendLighten: R should be ~255, got %d", pixelByte(p.R))
	}
	if pixelByte(p.G) < 250 {
		t.Errorf("BlendLighten: G should be ~255, got %d", pixelByte(p.G))
	}
}

// TestBlendMode_Difference_AffectsPixels verifies Difference.
func TestBlendMode_Difference_AffectsPixels(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendDifference)
	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	r := pixelByte(p.R)
	if r < 120 || r > 135 {
		t.Errorf("BlendDifference: R should be ~128, got %d", r)
	}
}

// TestBlendMode_Exclusion_AffectsPixels verifies Exclusion.
func TestBlendMode_Exclusion_AffectsPixels(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendExclusion)
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	r := pixelByte(p.R)
	if r < 120 || r > 135 {
		t.Errorf("BlendExclusion: R should be ~128, got %d", r)
	}
}

// TestBlendMode_Normal_Unchanged verifies BlendNormal still works correctly.
func TestBlendMode_Normal_Unchanged(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendNormal)
	dc.SetRGB(0, 0, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	if pixelByte(p.R) > 5 {
		t.Errorf("BlendNormal: R should be ~0, got %d", pixelByte(p.R))
	}
	if pixelByte(p.B) < 250 {
		t.Errorf("BlendNormal: B should be ~255, got %d", pixelByte(p.B))
	}
}

// TestBlendMode_PersistsAcrossDraws verifies SetBlendMode persists.
func TestBlendMode_PersistsAcrossDraws(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendMultiply)
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	r := pixelByte(p.R)
	if r < 55 || r > 72 {
		t.Errorf("BlendMultiply persisted: R should be ~64, got %d", r)
	}
}

// TestBlendMode_ResetToNormal verifies reset to BlendNormal works.
func TestBlendMode_ResetToNormal(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendMultiply)
	dc.SetRGB(0, 1, 0)
	dc.DrawRectangle(0, 0, 50, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendNormal)
	dc.SetRGB(0, 0, 1)
	dc.DrawRectangle(50, 0, 50, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(75, 50)
	if pixelByte(p.R) > 5 {
		t.Errorf("After reset: R should be ~0, got %d", pixelByte(p.R))
	}
	if pixelByte(p.B) < 250 {
		t.Errorf("After reset: B should be ~255, got %d", pixelByte(p.B))
	}
}

// TestBlendMode_SetColorDoesNotResetBlend verifies color changes don't affect blend mode.
func TestBlendMode_SetColorDoesNotResetBlend(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetBlendMode(BlendMultiply)
	dc.SetRGB(0.5, 0.5, 0.5)

	got := dc.GetBlendMode()
	want := BlendMode(blendModeMultiply)
	if got != want {
		t.Errorf("SetRGB reset blend mode: got %d, want %d", got, want)
	}
}

// TestBlendMode_Destination_NoOp verifies BlendDestination strength reduction.
// Uses direct paint.blendMode assignment since blendModeDestination=2 collides
// with BlendScreen=2 in the legacy mapping — there is no public constant for it.
func TestBlendMode_Destination_NoOp(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 0, 0)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	// Set directly on paint (bypassing SetBlendMode's legacy mapping).
	dc.paint.blendMode = blendModeDestination
	dc.SetRGB(0, 0, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()
	dc.paint.blendMode = blendModeSourceOver // restore

	p := dc.pixmap.GetPixel(50, 50)
	if pixelByte(p.R) < 250 {
		t.Errorf("BlendDestination no-op: R should be ~255, got %d", pixelByte(p.R))
	}
	if pixelByte(p.B) > 5 {
		t.Errorf("BlendDestination no-op: B should be ~0, got %d", pixelByte(p.B))
	}
}

// TestBlendMode_Overlay_DarkVsLight verifies Overlay behaves differently on dark vs light bg.
func TestBlendMode_Overlay_DarkVsLight(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(0.2, 0.2, 0.2)
	dc.DrawRectangle(0, 0, 50, 100)
	_ = dc.Fill()

	dc.SetRGB(0.8, 0.8, 0.8)
	dc.DrawRectangle(50, 0, 50, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendOverlay)
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	darkP := dc.pixmap.GetPixel(25, 50)
	lightP := dc.pixmap.GetPixel(75, 50)

	if pixelByte(darkP.R) >= pixelByte(lightP.R) {
		t.Errorf("Overlay: dark bg (%d) should produce darker result than light bg (%d)",
			pixelByte(darkP.R), pixelByte(lightP.R))
	}
}

// TestBlendMode_GetBlendMode_Default verifies default blend mode.
func TestBlendMode_GetBlendMode_Default(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	got := dc.GetBlendMode()
	if got != BlendMode(blendModeSourceOver) {
		t.Errorf("Default blend mode: got %d, want %d (SourceOver)", got, blendModeSourceOver)
	}
}

// TestBlendMode_GetBlendMode_AfterSet verifies GetBlendMode returns correct value.
func TestBlendMode_GetBlendMode_AfterSet(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	tests := []struct {
		name string
		set  BlendMode
		want paintBlendMode
	}{
		{"Normal", BlendNormal, blendModeSourceOver},
		{"Multiply", BlendMultiply, blendModeMultiply},
		{"Screen", BlendScreen, blendModeScreen},
		{"Overlay", BlendOverlay, blendModeOverlay},
		{"Darken", BlendDarken, blendModeDarken},
		{"Lighten", BlendLighten, blendModeLighten},
		{"Difference", BlendDifference, blendModeDifference},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc.SetBlendMode(tt.set)
			got := dc.GetBlendMode()
			if got != BlendMode(tt.want) {
				t.Errorf("GetBlendMode after Set(%s): got %d, want %d", tt.name, got, tt.want)
			}
		})
	}
}

// TestMapPublicBlendMode verifies the legacy-to-internal mapping.
func TestMapPublicBlendMode(t *testing.T) {
	tests := []struct {
		name string
		pub  BlendMode
		want paintBlendMode
	}{
		{"Normal(0)->SourceOver(3)", 0, 3},
		{"Multiply(1)->Multiply(14)", 1, 14},
		{"Screen(2)->Screen(15)", 2, 15},
		{"Overlay(3)->Overlay(16)", 3, 16},
		{"DestinationOver(4)->passthrough", 4, 4},
		{"Darken(17)->passthrough", 17, 17},
		{"Luminosity(28)->passthrough", 28, 28},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mapPublicBlendMode(tt.pub)
			if got != tt.want {
				t.Errorf("mapPublicBlendMode(%d): got %d, want %d", tt.pub, got, tt.want)
			}
		})
	}
}

// TestBlendMode_StrokeRespectsBlendMode verifies Stroke uses blend mode.
func TestBlendMode_StrokeRespectsBlendMode(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendMultiply)
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.SetLineWidth(20)
	dc.MoveTo(0, 50)
	dc.LineTo(100, 50)
	_ = dc.Stroke()

	p := dc.pixmap.GetPixel(50, 50)
	r := pixelByte(p.R)
	if r < 120 || r > 135 {
		t.Errorf("Stroke with BlendMultiply: R should be ~128, got %d", r)
	}
}

// TestBlendMode_AllModesNoPanic verifies all 29 modes work without panic.
func TestBlendMode_AllModesNoPanic(t *testing.T) {
	dc := NewContext(50, 50)
	defer func() { _ = dc.Close() }()

	modes := []BlendMode{
		BlendNormal, BlendMultiply, BlendScreen, BlendOverlay,
		BlendDestinationOver, BlendSourceIn, BlendDestinationIn,
		BlendSourceOut, BlendDestinationOut, BlendSourceAtop,
		BlendDestinationAtop, BlendXor, BlendPlus, BlendModulate,
		BlendDarken, BlendLighten, BlendColorDodge, BlendColorBurn,
		BlendHardLight, BlendSoftLight, BlendDifference, BlendExclusion,
		BlendHue, BlendSaturation, BlendColor, BlendLuminosity,
	}

	for _, mode := range modes {
		dc.SetBlendMode(mode)
		dc.SetRGB(0.5, 0.5, 0.5)
		dc.DrawRectangle(0, 0, 50, 50)
		_ = dc.Fill()
	}
}

// TestBlendFuncs_SourceOver_ByteLevel verifies byte-level SourceOver.
func TestBlendFuncs_SourceOver_ByteLevel(t *testing.T) {
	r, g, b, a := blendSourceOverFn(255, 0, 0, 255, 0, 0, 255, 255)
	if r != 255 || g != 0 || b != 0 || a != 255 {
		t.Errorf("SourceOver(red, blue): got (%d,%d,%d,%d), want (255,0,0,255)", r, g, b, a)
	}
}

// TestBlendFuncs_Multiply_ByteLevel verifies byte-level Multiply.
func TestBlendFuncs_Multiply_ByteLevel(t *testing.T) {
	r, g, b, _ := blendMultiplyFn(255, 0, 0, 255, 0, 0, 255, 255)
	if r > 2 || g > 2 || b > 2 {
		t.Errorf("Multiply(red,blue): got (%d,%d,%d), want ~(0,0,0)", r, g, b)
	}
}

// TestBlendFuncs_Screen_ByteLevel verifies byte-level Screen.
func TestBlendFuncs_Screen_ByteLevel(t *testing.T) {
	r, g, b, _ := blendScreenFn(255, 0, 0, 255, 0, 0, 255, 255)
	if r < 253 || b < 253 || g > 2 {
		t.Errorf("Screen(red,blue): got (%d,%d,%d), want ~(255,0,255)", r, g, b)
	}
}

// TestLookupBlendFuncForPaint_SourceOver_ReturnsNil verifies the fast path.
func TestLookupBlendFuncForPaint_SourceOver_ReturnsNil(t *testing.T) {
	p := NewPaint()
	if fn := lookupBlendFuncForPaint(p); fn != nil {
		t.Error("lookupBlendFuncForPaint should return nil for default SourceOver paint")
	}
}

// TestLookupBlendFuncForPaint_NonSourceOver_ReturnsFunc verifies dispatch.
func TestLookupBlendFuncForPaint_NonSourceOver_ReturnsFunc(t *testing.T) {
	p := NewPaint()
	p.blendMode = blendModeMultiply
	if fn := lookupBlendFuncForPaint(p); fn == nil {
		t.Error("lookupBlendFuncForPaint should return non-nil for Multiply paint")
	}
}

// TestBlendMode_SemiTransparent_Coverage verifies blend with anti-aliased edges.
func TestBlendMode_SemiTransparent_Coverage(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(1, 1, 1)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendMultiply)
	dc.SetRGB(0.5, 0.5, 0.5)
	dc.DrawCircle(50, 50, 40)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	r := pixelByte(p.R)
	if r < 120 || r > 135 {
		t.Errorf("Multiply center: R should be ~128, got %d", r)
	}

	corner := dc.pixmap.GetPixel(5, 5)
	if pixelByte(corner.R) < 250 {
		t.Errorf("Multiply outside circle: R should be ~255, got %d", pixelByte(corner.R))
	}
}

// TestBlendMode_Plus_Clamp verifies Plus mode clamps to 255.
func TestBlendMode_Plus_Clamp(t *testing.T) {
	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetRGB(0.8, 0.8, 0.8)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	dc.SetBlendMode(BlendPlus)
	dc.SetRGB(0.8, 0.8, 0.8)
	dc.DrawRectangle(0, 0, 100, 100)
	_ = dc.Fill()

	p := dc.pixmap.GetPixel(50, 50)
	if pixelByte(p.R) < 250 {
		t.Errorf("BlendPlus clamp: R should be ~255, got %d", pixelByte(p.R))
	}
}

// TestBlendMode_PaintClone_CopiesBlendMode verifies Clone copies blend mode.
func TestBlendMode_PaintClone_CopiesBlendMode(t *testing.T) {
	p := NewPaint()
	p.blendMode = blendModeMultiply
	clone := p.Clone()
	if clone.blendMode != blendModeMultiply {
		t.Errorf("Clone didn't copy blendMode: got %d, want %d", clone.blendMode, blendModeMultiply)
	}
}

// TestBlendMode_NewPaint_DefaultSourceOver verifies NewPaint defaults to SourceOver.
func TestBlendMode_NewPaint_DefaultSourceOver(t *testing.T) {
	p := NewPaint()
	if p.blendMode != blendModeSourceOver {
		t.Errorf("NewPaint blendMode: got %d, want %d (SourceOver)", p.blendMode, blendModeSourceOver)
	}
}
