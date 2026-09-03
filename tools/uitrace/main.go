// Command uitrace replays a scroll gesture through gophics and writes what the
// scroll view did with it: a trace, a CSV, metrics, and optionally the frames
// and a video.
//
//	uitrace fling   [-v0 -2400] [-dur 0.1] [-hz 120] [-physics ios|android] [-out out] [-frames] [-video]
//	uitrace replay  [-hz 120] [-out out] [-frames] [-video] trace.json
//	uitrace compare a.json b.json
//
// fling drives a synthetic flick; replay drives the finger phase recorded in a
// trace (typically one a native twin wrote — see tools/uitrace/README.md for
// the contract). compare prints two traces' metrics side by side, which is
// the whole harness in one line of output: same gesture, two curves, numbers.
package main

import (
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/gif"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/tools/uitrace/trace"
)

func main() {
	if len(os.Args) < 2 {
		usage()
	}
	switch os.Args[1] {
	case "fling":
		runFling(os.Args[2:])
	case "replay":
		runReplay(os.Args[2:])
	case "compare":
		runCompare(os.Args[2:])
	default:
		usage()
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: uitrace fling|replay|compare [flags]")
	os.Exit(2)
}

type outputs struct {
	dir     string
	frames  bool
	video   bool
	hz      float64
	physics string
}

func outputFlags(fs *flag.FlagSet) *outputs {
	o := &outputs{}
	fs.StringVar(&o.dir, "out", "out", "output directory")
	fs.BoolVar(&o.frames, "frames", false, "write every frame as PNG")
	fs.BoolVar(&o.video, "video", false, "assemble the frames into a video (implies -frames)")
	fs.Float64Var(&o.hz, "hz", 120, "frame rate to step at")
	fs.StringVar(&o.physics, "physics", "ios", "fling curve to replay under: ios or android")
	return o
}

func runFling(args []string) {
	fs := flag.NewFlagSet("fling", flag.ExitOnError)
	o := outputFlags(fs)
	v0 := fs.Float64("v0", -2400, "finger velocity, px/s (negative flicks upward)")
	dur := fs.Float64("dur", 0.1, "finger phase duration, seconds")
	fs.Parse(args)
	input := trace.SyntheticFlick(*v0, *dur, o.hz)
	run(input, o, fmt.Sprintf("synthetic flick v0=%.0f dur=%.2fs", *v0, *dur))
}

func runReplay(args []string) {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	o := outputFlags(fs)
	fs.Parse(args)
	if fs.NArg() != 1 {
		usage()
	}
	src, err := trace.ReadFile(fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	run(src.Input, o, "replay of "+fs.Arg(0)+" ("+src.Source+")")
}

func run(input []trace.Sample, o *outputs, note string) {
	if err := os.MkdirAll(o.dir, 0o755); err != nil {
		fatal(err)
	}
	var frames []image.Image
	opts := trace.ReplayOptions{Hz: o.hz}
	if o.frames || o.video {
		framesDir := filepath.Join(o.dir, "frames")
		if err := os.MkdirAll(framesDir, 0o755); err != nil {
			fatal(err)
		}
		opts.Frames = func(i int, _ float64, img image.Image) {
			// Copy: Render's image is retained and rewritten next frame.
			c := image.NewRGBA(img.Bounds())
			draw.Draw(c, c.Bounds(), img, img.Bounds().Min, draw.Src)
			frames = append(frames, c)
			writePNG(filepath.Join(framesDir, fmt.Sprintf("%05d.png", i)), c)
		}
	}
	switch o.physics {
	case "ios", "":
		opts.Physics = shell.IOSScrollPhysics()
	case "android":
		opts.Physics = shell.AndroidScrollPhysics()
	default:
		fatal(fmt.Errorf("unknown -physics %q (ios or android)", o.physics))
	}
	tr, err := trace.Replay(input, opts)
	if err != nil {
		fatal(err)
	}
	tr.Source = "gophics/" + o.physics
	tr.Notes = note

	writeFile(filepath.Join(o.dir, "trace.json"), tr.Write)
	writeFile(filepath.Join(o.dir, "offsets.csv"), tr.WriteCSV)
	m := tr.Compute()
	if err := os.WriteFile(filepath.Join(o.dir, "metrics.txt"), []byte(m.String()+"\n"), 0o644); err != nil {
		fatal(err)
	}
	fmt.Println(m)
	fmt.Printf("\n%d frames at %.0fHz, release at %.3fs → %s/\n", len(tr.Offset), tr.Hz, tr.ReleaseT, o.dir)

	if o.video && len(frames) > 0 {
		writeVideo(o.dir, o.hz, frames)
	}
}

func runCompare(args []string) {
	if len(args) != 2 {
		usage()
	}
	a, err := trace.ReadFile(args[0])
	if err != nil {
		fatal(err)
	}
	b, err := trace.ReadFile(args[1])
	if err != nil {
		fatal(err)
	}
	ma, mb := a.Compute(), b.Compute()
	row := func(name string, x, y float64, unit string) {
		d := y - x
		fmt.Printf("%-18s %10.3f %10.3f  %+9.3f %s\n", name, x, y, d, unit)
	}
	fmt.Printf("%-18s %10s %10s  %9s\n", "", a.Source, b.Source, "delta")
	row("release velocity", ma.ReleaseV, mb.ReleaseV, "px/s")
	row("fling start (fit)", ma.FitV0, mb.FitV0, "px/s")
	row("peak velocity", ma.PeakV, mb.PeakV, "px/s")
	row("decay tau", ma.Tau, mb.Tau, "s")
	row("fit R²", ma.TauR2, mb.TauR2, "")
	row("settle time", ma.SettleT, mb.SettleT, "s")
	row("momentum distance", ma.MomentumDist, mb.MomentumDist, "px")
	row("total distance", ma.TotalDist, mb.TotalDist, "px")
}

// writeVideo assembles frames with ffmpeg when it is installed, and falls back
// to a GIF from the standard library when it is not — slower and paletted,
// but it needs nothing and still shows the motion.
func writeVideo(dir string, hz float64, frames []image.Image) {
	if ffmpeg, err := exec.LookPath("ffmpeg"); err == nil {
		out := filepath.Join(dir, "fling.mp4")
		cmd := exec.Command(ffmpeg, "-y", "-loglevel", "error",
			"-framerate", fmt.Sprintf("%g", hz),
			"-i", filepath.Join(dir, "frames", "%05d.png"),
			"-pix_fmt", "yuv420p", out)
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err == nil {
			fmt.Println("video:", out)
			return
		}
		fmt.Fprintln(os.Stderr, "ffmpeg failed; writing a GIF instead")
	}
	out := filepath.Join(dir, "fling.gif")
	g := &gif.GIF{}
	delay := int(100/hz + 0.5)
	if delay < 2 {
		delay = 2 // browsers floor GIF delays at ~2 (20ms)
	}
	pal := color.Palette{}
	for i := 0; i < 256; i++ {
		v := uint8(i)
		pal = append(pal, color.RGBA{v, v, v, 255})
	}
	for _, f := range frames {
		p := image.NewPaletted(f.Bounds(), pal)
		draw.FloydSteinberg.Draw(p, p.Bounds(), f, f.Bounds().Min)
		g.Image = append(g.Image, p)
		g.Delay = append(g.Delay, delay)
	}
	writeFile(out, func(w io.Writer) error { return gif.EncodeAll(w, g) })
	fmt.Println("video:", out)
}

func writePNG(path string, img image.Image) {
	writeFile(path, func(w io.Writer) error { return png.Encode(w, img) })
}

func writeFile(path string, fn func(io.Writer) error) {
	f, err := os.Create(path)
	if err != nil {
		fatal(err)
	}
	defer f.Close()
	if err := fn(f); err != nil {
		fatal(err)
	}
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "uitrace:", err)
	os.Exit(1)
}
