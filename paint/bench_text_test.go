package paint

import (
	"fmt"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
)

func benchPainter(b *testing.B) *Painter {
	p := NewPainter()
	if err := p.LoadFont(goregular.TTF); err != nil {
		b.Fatal(err)
	}
	return p
}

// A feed-like scene: 40 rows of a title + meta line, redrawn fully.
func BenchmarkRasterTextHeavy(b *testing.B) {
	p := benchPainter(b)
	lines := make([]string, 40)
	for i := range lines {
		lines[i] = fmt.Sprintf("Story %d: some reasonably long headline text here", i)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := p.BeginOffscreen(geom.Size{W: 480, H: 900}, 2)
		c.Clear(RGB(0.1, 0.1, 0.1))
		y := float32(20)
		for _, ln := range lines {
			c.TextIn("", ln, geom.Pt{X: 20, Y: y}, 15, RGB(0.9, 0.9, 0.9))
			c.TextIn("", "example.com · 100 points · 40 comments", geom.Pt{X: 20, Y: y + 18}, 12, RGB(0.5, 0.5, 0.5))
			y += 40
		}
		_ = p.Image()
	}
}

// Same scene shape but rects instead of text, to isolate text cost.
func BenchmarkRasterRectsOnly(b *testing.B) {
	p := benchPainter(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c := p.BeginOffscreen(geom.Size{W: 480, H: 900}, 2)
		c.Clear(RGB(0.1, 0.1, 0.1))
		y := float32(20)
		for range 40 {
			c.FillRRect(geom.RectXYWH(20, y, 440, 32), 6, RGB(0.15, 0.15, 0.18))
			y += 40
		}
		_ = p.Image()
	}
}
