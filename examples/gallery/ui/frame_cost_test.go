package ui_test

import (
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/examples/gallery/ui"
	"github.com/doug/gophics/geom"
)

// Frame cost per theme and per section, reported as percentiles.
//
// A mean hides exactly the thing that is complained about: a stutter is a
// small number of frames far above the rest, and averaging them into 59 good
// ones makes them disappear. p99 and the worst frame are what a person feels.
//
// Damage culling is switched off (GOPHICS_NO_DAMAGE) so every frame is a full
// repaint. That is the worst case rather than the typical one, which is the
// point: it measures how much work a scene costs when it does have to be
// drawn, without a frame's cost depending on which pixels happened to change.
func gallerySection(t *testing.T, theme, section string) *apptest.App {
	t.Helper()
	a := apptest.New(t, ui.Gallery{}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 420, H: 760}, Font: goregular.TTF,
	}))
	settle := func() {
		for range 40 {
			a.Step(1.0 / 60)
		}
	}
	if theme != "" {
		a.TapText(theme)
		settle()
	}
	if section != "" {
		a.Move(geom.Pt{X: 210, Y: 400})
		for i := 0; i < 20 && a.NodeContaining(section).Rect.Min.Y > 640; i++ {
			a.Scroll(geom.Pt{Y: -400})
			settle()
		}
		a.TapText(section)
		settle()
	}
	return a
}

func percentiles(d []time.Duration) (p50, p95, p99, max time.Duration) {
	slices.Sort(d)
	at := func(f float64) time.Duration {
		i := int(f * float64(len(d)-1))
		return d[i]
	}
	return at(0.50), at(0.95), at(0.99), d[len(d)-1]
}

func TestFrameCostByThemeAndSection(t *testing.T) {
	if os.Getenv("FRAME_COST") == "" {
		t.Skip("set FRAME_COST=1 to measure frame cost")
	}
	t.Setenv("GOPHICS_NO_DAMAGE", "1")

	cases := []struct{ theme, section string }{
		{"Light", ""},
		{"Glass", ""},
		{"Light", "Charts"},
		{"Glass", "Charts"},
		{"Light", "Typography"},
		{"Glass", "Typography"},
	}
	fmt.Printf("\n%-8s %-12s %8s %8s %8s %8s\n", "theme", "section", "p50", "p95", "p99", "max")
	for _, c := range cases {
		a := gallerySection(t, c.theme, c.section)
		const frames = 200
		d := make([]time.Duration, 0, frames)
		for range frames {
			t0 := time.Now()
			a.Render()
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

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

// TestProfileGlassCharts isolates the combination that costs far more than
// either ingredient: glass is ~1.6x on every other page, but glass over the
// charts page is nearly 3x. Run under -cpuprofile to find where that goes.
func TestProfileGlassCharts(t *testing.T) {
	if os.Getenv("FRAME_COST") == "" {
		t.Skip("set FRAME_COST=1 to profile")
	}
	t.Setenv("GOPHICS_NO_DAMAGE", "1")
	a := gallerySection(t, "Glass", "Charts")
	for range 120 {
		a.Render()
	}
}
