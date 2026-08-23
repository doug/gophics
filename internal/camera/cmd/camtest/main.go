//go:build darwin && !ios

// Command camtest opens the camera and reports what arrives — the quickest way
// to tell a working capture path from a plausible-looking one.
package main

import (
	"fmt"
	"image/png"
	"os"
	"time"

	"github.com/doug/gophics/internal/camera"
)

func main() {
	fmt.Printf("authorization: %v\n", camera.Authorization())
	c, err := camera.Open(camera.Options{Facing: camera.FacingFront, Width: 640})
	if err != nil {
		fmt.Fprintln(os.Stderr, "open:", err)
		os.Exit(1)
	}
	defer c.Stop()

	deadline := time.Now().Add(5 * time.Second)
	var seen int
	var last any
	for time.Now().Before(deadline) {
		if f := c.Frame(); f != nil && any(f) != last {
			seen++
			last = any(f)
			if seen <= 3 || seen%20 == 0 {
				b := f.Bounds()
				// Mean luminance, so a black frame is distinguishable from a picture.
				var sum int
				for i := 0; i < len(f.Pix); i += 4 {
					sum += int(f.Pix[i]) + int(f.Pix[i+1]) + int(f.Pix[i+2])
				}
				mean := float64(sum) / float64(len(f.Pix)/4*3)
				fmt.Printf("frame %-3d %dx%d  mean=%.1f\n", seen, b.Dx(), b.Dy(), mean)
			}
		}
		time.Sleep(8 * time.Millisecond)
	}
	fmt.Printf("total distinct frames in 5s: %d\n", seen)
	if f := c.Frame(); f != nil {
		out, _ := os.Create("/tmp/camshot.png")
		png.Encode(out, f)
		out.Close()
		fmt.Println("wrote /tmp/camshot.png")
	}
}
