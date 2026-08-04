# Contributing to audio

Thank you for your interest in contributing to gogpu/audio! This document covers how to build, test, and submit changes.

## Prerequisites

- **Go 1.25+** ([download](https://go.dev/dl/))
- **golangci-lint** (`go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@latest`)

## Building

```bash
go build ./...
```

## Running Tests

```bash
go test ./...
```

## Code Style

- Run `go fmt ./...` before every commit. CI enforces this.
- Run `golangci-lint run --timeout=5m` and fix all issues.
- Follow standard Go naming conventions (`ID`, `URL`, `HTTP` are uppercase).
- Handle every error or explicitly ignore with `_ =` and a comment explaining why.
- Exported types and functions must have doc comments.

## Pull Request Workflow

1. Fork the repository and create a feature branch:
   ```bash
   git checkout -b feat/my-feature
   ```
2. Make your changes. Keep commits focused and well-described.
3. Verify locally:
   ```bash
   go fmt ./...
   go build ./...
   go test ./...
   golangci-lint run --timeout=5m
   ```
4. Push and open a pull request against `main`.
5. Wait for CI to pass. All checks must be green before merge.

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/):
```
feat: add WASAPI driver for Windows
fix(wav): handle 24-bit PCM sign extension
docs: update platform support table
```

## Architecture

```
context.go      Audio context (singleton, device management)
player.go       PCM stream player (io.Reader based)
driver.go       Driver interface (platform abstraction)
driver_null.go  Null driver (testing/headless)
wav.go          WAV decoder (Pure Go)
mixer.go        Multi-channel mixer
```

Platform drivers (planned):
- `driver_wasapi.go` — Windows WASAPI (COM via syscall)
- `driver_coreaudio.go` — macOS CoreAudio (AudioUnit via goffi)
- `driver_pulse.go` — Linux PulseAudio (native protocol, Pure Go)

## Priority Areas

1. **Platform drivers** — WASAPI, CoreAudio, PulseAudio implementations
2. **Format decoders** — OGG Vorbis, MP3 (Pure Go)
3. **Testing** — cross-platform audio output verification
4. **Documentation** — examples, API guides

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
