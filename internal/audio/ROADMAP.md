# Roadmap

## Vision

**audio** is a Pure Go audio engine for the [gogpu](https://github.com/gogpu) ecosystem — PCM output, decoding, mixing, with platform-native drivers. Zero CGO.

## Current: v0.1.0

- WASAPI driver (Windows) — COM vtable via syscall.SyscallN, shared mode, event-driven
- Audio Context (singleton, sample rate/channels config, auto-selects platform driver)
- Player (io.Reader, play/pause/stop/volume)
- Pull-model Driver interface (ReadFloat32er) + NullDriver
- WAV decoder (8/16/24/32-bit PCM, 32-bit float)
- Mixer (multi-channel, volume, clamping)
- internal/wasapi/ (COM vtable structs, hidden from public API)
- CI workflow (3 platforms)

## Released

### v0.1.0 (2026-05-10)
- [x] WASAPI driver (Windows)
- [x] Audio Context + Player + Mixer
- [x] WAV decoder (Pure Go)
- [x] CI workflow (3 platforms)
- [x] 42 tests, 94.8% coverage

## Planned

### v0.2.0 — All Platforms

- [ ] CoreAudio driver (macOS, AudioQueue via goffi)
- [ ] PulseAudio driver (Linux, libpulse-simple via dlopen)
- [ ] Integration tests per platform

### v0.3.0 — Formats + Features

- [ ] OGG Vorbis decoder (Pure Go)
- [ ] MP3 decoder (Pure Go)
- [ ] Streaming playback (large files)
- [ ] 3D spatial audio (basic panning)

### Future

- [ ] Audio recording/capture
- [ ] MIDI support
- [ ] ALSA fallback driver (embedded Linux)
- [ ] Web Audio API (WASM target)
- [ ] PipeWire native driver (no PulseAudio compatibility layer)
