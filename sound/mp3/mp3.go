// Package mp3 decodes MP3 audio into a sound.Sample. It is a separate subpackage
// so the base sound package keeps no decoder dependency.
package mp3

import (
	"io"

	gomp3 "github.com/hajimehoshi/go-mp3"

	"github.com/doug/gophics/sound"
)

// Decode reads an entire MP3 stream and returns it as a mono Sample resampled to
// sound.SampleRate. go-mp3 outputs 16-bit little-endian stereo.
func Decode(r io.Reader) (*sound.Sample, error) {
	dec, err := gomp3.NewDecoder(r)
	if err != nil {
		return nil, err
	}
	raw, err := io.ReadAll(dec)
	if err != nil {
		return nil, err
	}
	// 16-bit LE, 2 channels → interleaved float32.
	n := len(raw) / 2
	data := make([]float32, n)
	for i := 0; i < n; i++ {
		s := int16(raw[2*i]) | int16(raw[2*i+1])<<8
		data[i] = float32(s) / 32768
	}
	return sound.FromInterleaved(data, dec.SampleRate(), 2), nil
}
