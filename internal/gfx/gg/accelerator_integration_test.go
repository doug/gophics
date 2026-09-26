package gg

import (
	"sync"
	"testing"
)

// trackingAccelerator is a mock that tracks which methods were called.
type trackingAccelerator struct {
	mu            sync.Mutex
	fillShapeCt   int
	strokeShapeCt int
	fillPathCt    int
	strokePathCt  int
	lastShape     DetectedShape
}

func (a *trackingAccelerator) Name() string { return "tracking" }
func (a *trackingAccelerator) Init() error  { return nil }
func (a *trackingAccelerator) Close()       {}

func (a *trackingAccelerator) CanAccelerate(op AcceleratedOp) bool {
	return op&(AccelCircleSDF|AccelRRectSDF) != 0
}

func (a *trackingAccelerator) FillPath(_ GPURenderTarget, _ *Path, _ *Paint) error {
	a.mu.Lock()
	a.fillPathCt++
	a.mu.Unlock()
	return ErrFallbackToCPU
}

func (a *trackingAccelerator) StrokePath(_ GPURenderTarget, _ *Path, _ *Paint) error {
	a.mu.Lock()
	a.strokePathCt++
	a.mu.Unlock()
	return ErrFallbackToCPU
}

func (a *trackingAccelerator) FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	a.mu.Lock()
	a.fillShapeCt++
	a.lastShape = shape
	a.mu.Unlock()

	// Actually render using the SDF accelerator for verification.
	sdf := &SDFAccelerator{}
	return sdf.FillShape(target, shape, paint)
}

func (a *trackingAccelerator) StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	a.mu.Lock()
	a.strokeShapeCt++
	a.lastShape = shape
	a.mu.Unlock()

	sdf := &SDFAccelerator{}
	return sdf.StrokeShape(target, shape, paint)
}

func (a *trackingAccelerator) Flush(_ GPURenderTarget) error { return nil }

// flushTrackingAccelerator tracks Flush calls for testing Context.Close() behavior.
type flushTrackingAccelerator struct {
	trackingAccelerator
	flushCt int
}

func (a *flushTrackingAccelerator) Flush(_ GPURenderTarget) error {
	a.mu.Lock()
	a.flushCt++
	a.mu.Unlock()
	return nil
}

func TestContextCloseFlushesGPU(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &flushTrackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)

	// Draw a circle so there might be pending GPU state.
	dc.SetColor(Red.Color())
	dc.DrawCircle(100, 100, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() = %v", err)
	}

	tracker.mu.Lock()
	flushBefore := tracker.flushCt
	tracker.mu.Unlock()

	// Close should flush the accelerator.
	if err := dc.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	tracker.mu.Lock()
	flushAfter := tracker.flushCt
	tracker.mu.Unlock()

	if flushAfter <= flushBefore {
		t.Error("expected Close to flush GPU accelerator, but Flush was not called")
	}
}

// TestGPUDisabledContextNeverTouchesGlobalAccelerator: a context that opted
// out with SetGPUDisabled must not reach the process-global accelerator at
// all, not even through the "no per-context GPURenderContext" fallbacks.
//
// The fallbacks used to call Accelerator() directly, and ensureGPUCtx
// deliberately gives a disabled context no render context — so every CPU-only
// context took the fallback and drove the one accelerator everybody shares.
// Two of them rendering on different goroutines (several offscreen painters in
// one process) wrote its pending command queue concurrently, which -race
// caught in the framework's headless tests, and a single one flushed shapes
// another context had queued into its own pixmap.
func TestGPUDisabledContextNeverTouchesGlobalAccelerator(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &flushTrackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(64, 64)
	dc.SetGPUDisabled(true)

	// A rect fill and a circle fill: the first reaches the CPU renderer
	// through doFill's flush, the second is the shape the SDF fast path
	// would otherwise claim.
	dc.SetColor(Red.Color())
	dc.DrawRectangle(8, 8, 32, 32)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill(rect) = %v", err)
	}
	dc.DrawCircle(32, 32, 12)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill(circle) = %v", err)
	}
	dc.SetLineWidth(2)
	dc.DrawCircle(32, 32, 20)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke() = %v", err)
	}
	if err := dc.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}

	tracker.mu.Lock()
	defer tracker.mu.Unlock()
	if tracker.flushCt != 0 {
		t.Errorf("Flush called %d times on the global accelerator by a GPU-disabled context, want 0", tracker.flushCt)
	}
	if tracker.fillShapeCt != 0 || tracker.fillPathCt != 0 {
		t.Errorf("fill reached the global accelerator: FillShape=%d FillPath=%d, want 0/0",
			tracker.fillShapeCt, tracker.fillPathCt)
	}
	if tracker.strokeShapeCt != 0 || tracker.strokePathCt != 0 {
		t.Errorf("stroke reached the global accelerator: StrokeShape=%d StrokePath=%d, want 0/0",
			tracker.strokeShapeCt, tracker.strokePathCt)
	}

	// The drawing still landed on the CPU pixmap.
	if _, _, _, a := dc.pixmap.At(24, 24).RGBA(); a == 0 {
		t.Error("GPU-disabled fill produced a transparent pixel; CPU raster did not run")
	}
}

func TestContextCloseWithoutAccelerator(t *testing.T) {
	resetAccelerator()

	// Verify Close works without panic when no accelerator is registered.
	dc := NewContext(100, 100)
	dc.DrawCircle(50, 50, 20)
	_ = dc.Fill()

	if err := dc.Close(); err != nil {
		t.Fatalf("Close() = %v", err)
	}
}

func TestContextWithSDFAcceleratorFillCircle(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Red.Color())
	dc.DrawCircle(100, 100, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() = %v", err)
	}

	tracker.mu.Lock()
	fc := tracker.fillShapeCt
	lastKind := tracker.lastShape.Kind
	tracker.mu.Unlock()

	if fc == 0 {
		t.Error("expected FillShape to be called for circle, but it was not")
	}
	if lastKind != ShapeCircle {
		t.Errorf("expected shape kind ShapeCircle, got %d", lastKind)
	}

	// Verify pixels were actually drawn.
	px := dc.pixmap.GetPixel(100, 100)
	if px.A < 0.5 {
		t.Errorf("center pixel alpha = %f, want >= 0.5", px.A)
	}
}

func TestContextWithSDFAcceleratorStrokeCircle(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Blue.Color())
	dc.SetLineWidth(2.0)
	dc.DrawCircle(100, 100, 30)
	if err := dc.Stroke(); err != nil {
		t.Fatalf("Stroke() = %v", err)
	}

	tracker.mu.Lock()
	sc := tracker.strokeShapeCt
	lastKind := tracker.lastShape.Kind
	tracker.mu.Unlock()

	if sc == 0 {
		t.Error("expected StrokeShape to be called for circle, but it was not")
	}
	if lastKind != ShapeCircle {
		t.Errorf("expected shape kind ShapeCircle, got %d", lastKind)
	}
}

func TestContextFallbackToSoftware(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	// Draw an arbitrary path that is NOT a recognized shape.
	dc.SetColor(Green.Color())
	dc.MoveTo(10, 10)
	dc.LineTo(100, 10)
	dc.QuadraticTo(100, 100, 10, 100)
	dc.ClosePath()
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() = %v", err)
	}

	// The accelerator does not support AccelFill, so it should fall back.
	tracker.mu.Lock()
	fc := tracker.fillShapeCt
	fpc := tracker.fillPathCt
	tracker.mu.Unlock()

	if fc != 0 {
		t.Errorf("expected FillShape not to be called for arbitrary path, got %d calls", fc)
	}
	// fillPathCt should be 0 because CanAccelerate(AccelFill) returns false.
	if fpc != 0 {
		t.Errorf("expected FillPath not to be called (unsupported op), got %d calls", fpc)
	}

	// Verify pixels were drawn by software renderer.
	px := dc.pixmap.GetPixel(50, 50)
	if px.A < 0.5 {
		t.Errorf("center pixel alpha = %f, want >= 0.5 (software fallback)", px.A)
	}
}

func TestContextNoAcceleratorSoftwarePath(t *testing.T) {
	resetAccelerator()

	dc := NewContext(100, 100)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Red.Color())
	dc.DrawCircle(50, 50, 20)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill() = %v", err)
	}

	// Verify software rendering works when no accelerator is registered.
	px := dc.pixmap.GetPixel(50, 50)
	if px.A < 0.9 {
		t.Errorf("center pixel alpha = %f, want >= 0.9", px.A)
	}
	if px.R < 0.9 {
		t.Errorf("center pixel red = %f, want >= 0.9", px.R)
	}
}

func TestContextFillPreserveWithAccelerator(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Red.Color())
	dc.DrawCircle(100, 100, 30)

	// FillPreserve should use accelerator but NOT clear the path.
	if err := dc.FillPreserve(); err != nil {
		t.Fatalf("FillPreserve() = %v", err)
	}

	tracker.mu.Lock()
	fc := tracker.fillShapeCt
	tracker.mu.Unlock()

	if fc == 0 {
		t.Error("expected FillShape to be called for FillPreserve")
	}

	// Path should still have elements.
	if dc.path.isEmpty() {
		t.Error("path should not be cleared after FillPreserve")
	}
}

func TestContextStrokePreserveWithAccelerator(t *testing.T) {
	resetAccelerator()
	defer resetAccelerator()

	tracker := &trackingAccelerator{}
	if err := RegisterAccelerator(tracker); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(200, 200)
	defer func() { _ = dc.Close() }()

	dc.SetColor(Blue.Color())
	dc.SetLineWidth(2.0)
	dc.DrawCircle(100, 100, 30)

	// StrokePreserve should use accelerator but NOT clear the path.
	if err := dc.StrokePreserve(); err != nil {
		t.Fatalf("StrokePreserve() = %v", err)
	}

	tracker.mu.Lock()
	sc := tracker.strokeShapeCt
	tracker.mu.Unlock()

	if sc == 0 {
		t.Error("expected StrokeShape to be called for StrokePreserve")
	}

	// Path should still have elements.
	if dc.path.isEmpty() {
		t.Error("path should not be cleared after StrokePreserve")
	}
}
