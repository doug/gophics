package mobile

import "testing"

// A GPU surface that configures but cannot present must give up.
//
// This is the worst failure this path has, and an Android emulator without
// working Vulkan produces it every time: SetSurface succeeds, GPUActive says
// yes, and every frame then fails to acquire a swapchain image. The host, told
// the GPU is live, renders into nothing — a black screen whose only explanation
// is sixty identical log lines a second.
//
// The contract is that GPUActive stops saying yes, so a host polling it each
// frame falls back to the CPU without needing to understand Vulkan.
func TestGPUSurfaceRetiresAfterRepeatedPresentFailures(t *testing.T) {
	b := NewBridge(nil)
	g := &mobileGPU{}
	b.gpu = g

	if !b.GPUActive() {
		t.Fatal("a fresh surface must report active")
	}

	// Transient failures do not retire it: a swapchain can break during a
	// resize and recover on the next frame.
	for i := 1; i < gpuFailureLimit; i++ {
		g.presentFailed("get current texture", errFake{})
		if !b.GPUActive() {
			t.Fatalf("retired after %d failure(s); the limit is %d, so a "+
				"transient break would cost the GPU for the whole session",
				i, gpuFailureLimit)
		}
	}

	g.presentFailed("get current texture", errFake{})
	if b.GPUActive() {
		t.Errorf("still active after %d consecutive failures — the host will keep "+
			"rendering into a surface that never presents", gpuFailureLimit)
	}

	// A successful present clears the count, so a surface that recovers is not
	// held against its earlier failures.
	g.failed = 0
	if !b.GPUActive() {
		t.Error("a recovered surface must report active again")
	}
}

type errFake struct{}

func (errFake) Error() string { return "fake" }
