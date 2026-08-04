// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

// Driver is the platform audio output interface.
// Implementations exist per platform: WASAPI (Windows), CoreAudio (macOS),
// PulseAudio (Linux). The NullDriver is provided for testing and headless use.
//
// Drivers use a pull model: a background goroutine reads PCM data from the
// source (typically the Mixer) and feeds it to the hardware. This matches
// the native model of all three platforms (WASAPI event-driven, CoreAudio
// callback, PulseAudio write-when-ready).
type Driver interface {
	// Open opens the audio device with the given format and buffer size.
	Open(sampleRate, channels, bufferSizeMs int) error

	// SetSource sets the audio data source. The driver's background goroutine
	// pulls PCM samples from this source via ReadFloat32s.
	SetSource(src ReadFloat32er)

	// Start begins audio playback. The driver goroutine starts pulling from
	// the source and feeding the hardware.
	Start() error

	// Stop pauses audio playback without closing the device.
	Stop() error

	// Close stops playback and releases the audio device.
	Close() error
}

// ReadFloat32er provides interleaved float32 PCM samples.
// The Mixer implements this interface.
type ReadFloat32er interface {
	ReadFloat32s(buf []float32) (int, error)
}
