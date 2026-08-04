// Copyright 2026 The gogpu Authors

package audio

// DefaultDriver returns the platform's audio driver (CoreAudio on macOS,
// PulseAudio on Linux, WASAPI on Windows, WebAudio on wasm, else a no-op). Open
// it, SetSource a ReadFloat32er, and Start to play.
func DefaultDriver() Driver { return defaultDriver() }
