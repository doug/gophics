// Package device connects a sound.Mixer to the platform's audio output using
// the doug/audio drivers (CoreAudio / PulseAudio / WASAPI / WebAudio). It is
// kept separate from the pure sound package because it pulls in platform FFI —
// call Open from an app's main, never from headless tests.
package device

import (
	"io"

	audio "github.com/doug/audio"

	"github.com/doug/gossamer/sound"
)

// Open starts the default platform driver pulling from m at 44.1 kHz stereo.
// Close the returned handle to stop playback and release the device.
func Open(m *sound.Mixer) (io.Closer, error) {
	d := audio.DefaultDriver()
	if err := d.Open(sound.SampleRate, 2, 20); err != nil {
		return nil, err
	}
	d.SetSource(m)
	if err := d.Start(); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}
