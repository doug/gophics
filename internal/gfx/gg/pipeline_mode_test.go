package gg

import "testing"

func TestPipelineModeString(t *testing.T) {
	tests := []struct {
		name string
		mode PipelineMode
		want string
	}{
		{"Auto", PipelineModeAuto, "Auto"},
		{"RenderPass", PipelineModeRenderPass, "RenderPass"},
		{"Compute", PipelineModeCompute, "Compute"},
		{"Unknown", PipelineMode(99), "Unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("PipelineMode(%d).String() = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestPipelineModeDefault(t *testing.T) {
	dc := NewContext(100, 100)
	if got := dc.PipelineMode(); got != PipelineModeAuto {
		t.Errorf("new context PipelineMode() = %v, want PipelineModeAuto", got)
	}
}

func TestPipelineModeSetGet(t *testing.T) {
	dc := NewContext(100, 100)

	dc.SetPipelineMode(PipelineModeCompute)
	if got := dc.PipelineMode(); got != PipelineModeCompute {
		t.Errorf("after SetPipelineMode(Compute): got %v, want PipelineModeCompute", got)
	}

	dc.SetPipelineMode(PipelineModeRenderPass)
	if got := dc.PipelineMode(); got != PipelineModeRenderPass {
		t.Errorf("after SetPipelineMode(RenderPass): got %v, want PipelineModeRenderPass", got)
	}

	dc.SetPipelineMode(PipelineModeAuto)
	if got := dc.PipelineMode(); got != PipelineModeAuto {
		t.Errorf("after SetPipelineMode(Auto): got %v, want PipelineModeAuto", got)
	}
}

func TestWithPipelineMode(t *testing.T) {
	tests := []struct {
		name string
		mode PipelineMode
	}{
		{"Auto", PipelineModeAuto},
		{"RenderPass", PipelineModeRenderPass},
		{"Compute", PipelineModeCompute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dc := NewContext(100, 100, WithPipelineMode(tt.mode))
			if got := dc.PipelineMode(); got != tt.mode {
				t.Errorf("NewContext with WithPipelineMode(%v): got %v", tt.mode, got)
			}
		})
	}
}

// SelectPipeline returns RenderPass for every scene. These cases are the ones
// that used to return Compute — >50 shapes, deep clips, high overlap, medium
// complexity — kept as cases precisely because they are where the old
// shape-count heuristic diverged. The cross-pipeline benchmark in
// pipeline_mode.go measured render-pass ahead in all of them.
func TestSelectPipelineAlwaysRenderPass(t *testing.T) {
	tests := []struct {
		name  string
		stats SceneStats
	}{
		{"simple", SceneStats{ShapeCount: 5, ClipDepth: 1}},
		{"complex 60 shapes", SceneStats{ShapeCount: 60}},
		{"deep clips", SceneStats{ShapeCount: 20, ClipDepth: 5}},
		{"high overlap", SceneStats{ShapeCount: 30, OverlapFactor: 0.7}},
		{"text heavy", SceneStats{ShapeCount: 10, TextCount: 20}},
		{"medium complexity", SceneStats{ShapeCount: 30, PathCount: 10, ClipDepth: 2}},
		{"zero", SceneStats{}},
	}

	for _, tt := range tests {
		for _, hasCompute := range []bool{false, true} {
			t.Run(tt.name, func(t *testing.T) {
				if got := SelectPipeline(tt.stats, hasCompute); got != PipelineModeRenderPass {
					t.Errorf("SelectPipeline(%+v, hasCompute=%v) = %v, want RenderPass",
						tt.stats, hasCompute, got)
				}
			})
		}
	}
}

// Auto must never route to compute. The table above names specific scenes; this
// sweeps the whole input space the heuristic used to branch on, so a threshold
// reintroduced anywhere in that space fails here rather than silently costing
// every user of PipelineModeAuto a 1.4x-7x slowdown.
func TestSelectPipelineNeverSelectsCompute(t *testing.T) {
	for _, shapes := range []int{0, 1, 9, 10, 49, 50, 51, 500, 100000} {
		for _, clips := range []int{0, 1, 3, 4, 64} {
			for _, text := range []int{0, 1, 1000} {
				for _, overlap := range []float64{0, 0.5, 0.51, 1} {
					for _, hasCompute := range []bool{false, true} {
						stats := SceneStats{
							ShapeCount:    shapes,
							PathCount:     shapes / 2,
							TextCount:     text,
							ClipDepth:     clips,
							OverlapFactor: overlap,
						}
						if got := SelectPipeline(stats, hasCompute); got != PipelineModeRenderPass {
							t.Fatalf("SelectPipeline(%+v, hasCompute=%v) = %v, want RenderPass",
								stats, hasCompute, got)
						}
					}
				}
			}
		}
	}
}

// --- Pipeline mode propagation tests ---

// mockPipelineAwareAccel is a test accelerator that records pipeline mode changes.
type mockPipelineAwareAccel struct {
	mode         PipelineMode
	initCalled   bool
	canCompute   bool
	fillCount    int
	fillPathMode string // "shape" or "path" — records which path was taken
}

func (m *mockPipelineAwareAccel) Name() string                        { return "mock-pipeline" }
func (m *mockPipelineAwareAccel) Init() error                         { m.initCalled = true; return nil }
func (m *mockPipelineAwareAccel) Close()                              {}
func (m *mockPipelineAwareAccel) CanAccelerate(op AcceleratedOp) bool { return true }
func (m *mockPipelineAwareAccel) FillPath(target GPURenderTarget, path *Path, paint *Paint) error {
	m.fillCount++
	m.fillPathMode = "path"
	return nil
}
func (m *mockPipelineAwareAccel) StrokePath(target GPURenderTarget, path *Path, paint *Paint) error {
	m.fillCount++
	m.fillPathMode = "path"
	return nil
}
func (m *mockPipelineAwareAccel) FillShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	m.fillCount++
	m.fillPathMode = "shape"
	return nil
}
func (m *mockPipelineAwareAccel) StrokeShape(target GPURenderTarget, shape DetectedShape, paint *Paint) error {
	m.fillCount++
	m.fillPathMode = "shape"
	return nil
}
func (m *mockPipelineAwareAccel) Flush(target GPURenderTarget) error { return nil }

// PipelineModeAware implementation.
func (m *mockPipelineAwareAccel) SetPipelineMode(mode PipelineMode) {
	m.mode = mode
}

// ComputePipelineAware implementation.
func (m *mockPipelineAwareAccel) CanCompute() bool {
	return m.canCompute
}

func TestSetPipelineModePropagation(t *testing.T) {
	mock := &mockPipelineAwareAccel{}
	oldAccel := Accelerator()
	defer func() {
		// Restore previous accelerator.
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)

	// Setting pipeline mode on context should propagate to accelerator.
	dc.SetPipelineMode(PipelineModeCompute)
	if mock.mode != PipelineModeCompute {
		t.Errorf("after SetPipelineMode(Compute): mock.mode = %v, want Compute", mock.mode)
	}

	dc.SetPipelineMode(PipelineModeRenderPass)
	if mock.mode != PipelineModeRenderPass {
		t.Errorf("after SetPipelineMode(RenderPass): mock.mode = %v, want RenderPass", mock.mode)
	}

	dc.SetPipelineMode(PipelineModeAuto)
	if mock.mode != PipelineModeAuto {
		t.Errorf("after SetPipelineMode(Auto): mock.mode = %v, want Auto", mock.mode)
	}
}

func TestComputeModeSkipsShapeDetection(t *testing.T) {
	mock := &mockPipelineAwareAccel{canCompute: true}
	oldAccel := Accelerator()
	defer func() {
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)
	dc.SetPipelineMode(PipelineModeCompute)

	// Draw a circle — would normally be detected as a shape and use SDF.
	// In Compute mode, it should go directly to FillPath (skip shape detection).
	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	if mock.fillPathMode != "path" {
		t.Errorf("in Compute mode: expected fillPathMode='path' (skip shape detection), got %q", mock.fillPathMode)
	}
}

func TestRenderPassModeUsesShapeDetection(t *testing.T) {
	mock := &mockPipelineAwareAccel{canCompute: true}
	oldAccel := Accelerator()
	defer func() {
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)
	dc.SetPipelineMode(PipelineModeRenderPass)

	// Draw a circle — should be detected as shape and use FillShape.
	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	if mock.fillPathMode != "shape" {
		t.Errorf("in RenderPass mode: expected fillPathMode='shape' (via shape detection), got %q", mock.fillPathMode)
	}
}

func TestAutoModeDefaultsBehavior(t *testing.T) {
	mock := &mockPipelineAwareAccel{canCompute: false}
	oldAccel := Accelerator()
	defer func() {
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)
	// Auto mode with no compute support — should use render pass path.

	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// Auto mode without compute = render pass = shape detection used.
	if mock.fillPathMode != "shape" {
		t.Errorf("in Auto mode (no compute): expected fillPathMode='shape', got %q", mock.fillPathMode)
	}
}

func TestComputeModeWithoutComputeFallsThrough(t *testing.T) {
	// If Compute is requested but CanCompute() returns false,
	// it should fall through to the render pass path.
	mock := &mockPipelineAwareAccel{canCompute: false}
	oldAccel := Accelerator()
	defer func() {
		accelMu.Lock()
		accel = oldAccel
		accelMu.Unlock()
	}()

	if err := RegisterAccelerator(mock); err != nil {
		t.Fatalf("RegisterAccelerator: %v", err)
	}

	dc := NewContext(100, 100)
	dc.SetPipelineMode(PipelineModeCompute)

	dc.DrawCircle(50, 50, 30)
	if err := dc.Fill(); err != nil {
		t.Fatalf("Fill: %v", err)
	}

	// Compute not available — falls through to render pass (shape detection).
	if mock.fillPathMode != "shape" {
		t.Errorf("in Compute mode (no compute support): expected fallthrough to shape, got %q", mock.fillPathMode)
	}
}
