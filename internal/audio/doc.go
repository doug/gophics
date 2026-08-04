// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

// Package audio provides a Pure Go audio engine for cross-platform PCM
// audio output, WAV decoding, and multi-channel mixing.
//
// Platform drivers:
//   - Windows: WASAPI (Windows Audio Session API)
//   - macOS: CoreAudio (AudioUnit via goffi)
//   - Linux: PulseAudio (native protocol, Pure Go)
//
// This package is part of the gogpu ecosystem (https://github.com/gogpu).
// For simple UI feedback sounds (button clicks, notifications), use
// gogpu/sound instead — a thin platform delegation layer.
package audio
