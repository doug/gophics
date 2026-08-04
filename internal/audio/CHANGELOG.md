# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-05-10

### Added

- **WASAPI driver** (Windows) — Pure Go audio output via COM vtable (`syscall.SyscallN`). Shared mode, event-driven, AUTOCONVERTPCM. Same COM pattern as gogpu/wgpu DX12 backend. Zero CGO.
- **Audio Context** — singleton manager, configurable sample rate and channels. Auto-selects platform driver (WASAPI on Windows, NullDriver elsewhere).
- **Player** — `io.Reader` PCM stream playback with play/pause/stop/volume
- **Driver interface** — pull-model platform abstraction (`ReadFloat32er`). Background goroutine feeds hardware.
- **NullDriver** — testing and headless mode (discards output)
- **WAV decoder** — Pure Go, 8/16/24/32-bit PCM + 32-bit float, stereo, duration
- **Mixer** — multi-channel mixing, per-source + master volume, clamping
- **`internal/wasapi/`** — COM vtable structs (IMMDeviceEnumerator, IAudioClient, IAudioRenderClient), GUIDs, DLL procs
- **`examples/play_wav`** — generates and plays 440Hz sine wave
- 42 tests, 94.8% coverage, lint 0 issues (Windows/Linux/macOS)
