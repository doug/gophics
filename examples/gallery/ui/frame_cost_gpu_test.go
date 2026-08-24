//go:build gophics_gpu

package ui_test

import (
	"fmt"
	"os"
	"testing"
	"time"
)

// The GPU counterpart of TestFrameCostByThemeAndSection.
//
// The CPU numbers put half a glass frame in the software box blur, which the
// device never runs: the GPU path blurs the backdrop in a shader
// (appendBackdropBlur). So the CPU profile locates a cost on desktop's
// fallback renderer and says nothing about a phone. These numbers include a
// readback that a real frame does not pay, which inflates every row equally —
// so read the *ratio* between themes, not the absolute figures.
func TestGPUFrameCostByThemeAndSection(t *testing.T) {
	if os.Getenv("FRAME_COST") == "" {
		t.Skip("set FRAME_COST=1 to measure frame cost")
	}
	t.Setenv("GOPHICS_NO_DAMAGE", "1")

	cases := []struct{ theme, section string }{
		{"Light", ""},
		{"Glass", ""},
		{"Light", "Charts"},
		{"Glass", "Charts"},
	}
	fmt.Printf("\n%-8s %-12s %8s %8s %8s %8s\n", "theme", "section", "p50", "p95", "p99", "max")
	for _, c := range cases {
		a := gallerySection(t, c.theme, c.section)
		if a.RenderGPU() == nil {
			t.Skip("no GPU adapter available")
		}
		const frames = 120
		d := make([]time.Duration, 0, frames)
		for i := 0; i < frames; i++ {
			t0 := time.Now()
			a.RenderGPU()
			d = append(d, time.Since(t0))
		}
		p50, p95, p99, max := percentiles(d)
		name := c.section
		if name == "" {
			name = "catalog"
		}
		fmt.Printf("%-8s %-12s %8.2f %8.2f %8.2f %8.2f\n", c.theme, name,
			ms(p50), ms(p95), ms(p99), ms(max))
	}
}
