<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/gogpu/.github/main/assets/logo.png">
    <source media="(prefers-color-scheme: light)" srcset="https://raw.githubusercontent.com/gogpu/.github/main/assets/logo.png">
    <img src="https://raw.githubusercontent.com/gogpu/.github/main/assets/logo.png" alt="GoGPU Logo" width="100" />
  </picture>
</p>

<h1 align="center">audio</h1>

<p align="center">
  <strong>Pure Go audio engine for Windows, macOS, and Linux</strong><br>
  Zero CGO. PCM output. WAV decoding. Mixing. Platform-native drivers.
</p>

<p align="center">
  <a href="https://github.com/gogpu/audio/actions"><img src="https://github.com/gogpu/audio/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://pkg.go.dev/github.com/gogpu/audio"><img src="https://pkg.go.dev/badge/github.com/gogpu/audio.svg" alt="Go Reference"></a>
  <a href="https://goreportcard.com/report/github.com/gogpu/audio"><img src="https://goreportcard.com/badge/github.com/gogpu/audio" alt="Go Report Card"></a>
  <a href="https://github.com/gogpu/audio/blob/main/LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License"></a>
  <a href="https://github.com/gogpu/audio"><img src="https://img.shields.io/badge/Pure_Go-Zero_CGO-brightgreen" alt="Zero CGO"></a>
</p>

---

## Overview

**audio** is a Pure Go audio engine for the [gogpu](https://github.com/gogpu) ecosystem. It provides low-latency PCM audio output, WAV decoding, and multi-channel mixing using platform-native APIs — without CGO.

Part of the [gogpu GPU computing ecosystem](https://github.com/gogpu) (~800K+ LOC Pure Go).

## Features

- **Pure Go** — zero CGO on all platforms. Uses goffi/syscall for platform calls
- **Platform-native drivers** — WASAPI (Windows), CoreAudio (macOS), PulseAudio (Linux)
- **WAV decoder** — Pure Go WAV file parsing and playback
- **Mixer** — multi-channel audio mixing with per-channel volume
- **io.Reader streams** — composable audio pipeline (decoder → mixer → driver)
- **Non-blocking** — audio runs in background goroutine, never blocks rendering

## Status

Windows audio output working (WASAPI). macOS and Linux drivers planned.

## Architecture

```
User Code
    │
    ▼
audio.Context (singleton, manages audio device)
    │
    ├── WAV Decoder (Pure Go)
    ├── Mixer (multi-channel, volume control)
    │       ↑ ReadFloat32er (pull model)
    ▼
Platform Driver (background goroutine → hardware)
    ├── WASAPI (Windows) ✅ — COM vtable via syscall.SyscallN
    ├── CoreAudio (macOS) — planned
    └── PulseAudio (Linux) — planned
```

## API

```go
package audio

// Create audio context (one per application)
ctx, err := audio.NewContext()
defer ctx.Close()

// Play a WAV file
player, err := ctx.PlayWAV(wavData)
player.SetVolume(0.8)

// Play from io.Reader stream
player, err := ctx.NewPlayer(pcmReader)
player.Play()
```

## Examples

```bash
# Play 440Hz sine wave
go run ./examples/play_wav

# Play Mozart's "Eine Kleine Nachtmusik" (Casio watch style)
go run ./examples/mozart
```

## Platform Support

| Platform | Driver | Status |
|----------|--------|--------|
| Windows | WASAPI (COM vtable via syscall) | ✅ Working |
| macOS | CoreAudio (AudioQueue via goffi) | Planned |
| Linux | PulseAudio (libpulse-simple via dlopen) | Planned |

## Ecosystem

| Package | Description |
|---------|-------------|
| [gogpu](https://github.com/gogpu/gogpu) | Application framework, windowing |
| [gogpu/sound](https://github.com/gogpu/gogpu/tree/main/sound) | UI system sounds (clicks, alerts) |
| **gogpu/audio** | Full audio engine (this package) |
| [wgpu](https://github.com/gogpu/wgpu) | Pure Go WebGPU |
| [gg](https://github.com/gogpu/gg) | 2D graphics |
| [ui](https://github.com/gogpu/ui) | GUI toolkit |

## Related: `gogpu/sound` (UI System Sounds)

For simple UI feedback sounds (button clicks, notifications), use [`gogpu/sound`](https://github.com/gogpu/gogpu/tree/main/sound) — a thin platform delegation layer (~500 LOC) that plays OS system sounds via winmm.dll (Windows), NSSound (macOS), and PulseAudio (Linux).

`gogpu/audio` is for games, media apps, and audio visualization that need full PCM playback, mixing, and streaming.

## License

MIT License — see [LICENSE](LICENSE).
