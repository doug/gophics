// Package ogg decodes Ogg Vorbis audio into a sound.Sample. It is a separate
// subpackage so the base sound package keeps no decoder dependency.
package ogg

import (
	"io"

	"github.com/jfreymuth/oggvorbis"

	"github.com/doug/gophics/sound"
)

// Decode reads an entire Ogg Vorbis stream and returns it as a mono Sample
// resampled to sound.SampleRate.
func Decode(r io.Reader) (*sound.Sample, error) {
	data, format, err := oggvorbis.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return sound.FromInterleaved(data, format.SampleRate, format.Channels), nil
}
