// Command mictest is a manual check that native capture works on this machine.
// It opens the default input, prints the level and detected pitch for a few
// seconds, and exits. Nothing is recorded or written anywhere.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/doug/gophics/internal/audio"
	"github.com/doug/gophics/internal/dsp"
	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/sound/device"
	"github.com/doug/gophics/sound/pitch"
)

func main() {
	c := audio.DefaultCapture()
	rate, err := c.Open(44100)
	if err != nil {
		fmt.Println("open:", err)
		return
	}
	fmt.Printf("capture open at %d Hz\n", rate)

	an := dsp.New(rate, dsp.DefaultWindow)
	var blocks int
	if err := c.Start(func(pcm []float32) { blocks++; an.Write(pcm) }); err != nil {
		fmt.Println("start:", err)
		return
	}
	defer c.Close()

	// With -tone, play A4 through the speakers so the microphone has something
	// definite to hear: if the detector reads back A4, every stage of the loop
	// — device, callback, ring buffer, FFT, YIN — is proven at once.
	if len(os.Args) > 1 && os.Args[1] == "-tone" {
		mx := sound.NewMixer()
		if h, derr := device.Open(mx); derr == nil {
			defer h.Close()
			go func() {
				for i := 0; i < 6; i++ {
					mx.PlaySource(sound.Tone(440, 0.6, 0.5), sound.PlayOptions{})
					time.Sleep(500 * time.Millisecond)
				}
			}()
		} else {
			fmt.Println("output:", derr)
		}
	}

	det := &pitch.Detector{SampleRate: rate, MinFreq: 70, MaxFreq: 1100}
	buf := make([]float32, an.WindowSize())
	bands := make([]float32, 8)

	for i := 0; i < 12; i++ {
		time.Sleep(250 * time.Millisecond)
		n := an.Samples(buf)
		r := det.Detect(buf[:n])
		note := "—"
		if r.Voiced {
			nn, cents := pitch.FromFreq(r.Freq)
			note = fmt.Sprintf("%v %+.0f cents (clarity %.2f)", nn, cents, r.Clarity)
		}
		an.Bands(bands)
		fmt.Printf("blocks=%-5d level=%.4f rms=%.4f  %s  bands=%.2f\n",
			blocks, an.Level(), r.RMS, note, bands)
	}
	fmt.Printf("total callback blocks: %d\n", blocks)
}
