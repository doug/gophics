# Spec — media capture shell + Journal app

A capability-driving project: the **Journal** app (a private, local Day One) is the
forcing function; the real deliverable is a **reusable camera + audio-in + audio-out
layer in the `shell` platform interface**. Build web-first to nail the Go-side API,
then implement the same interface on gomobile (the real native work) and desktop.

See [example-app-ideas.md](example-app-ideas.md) §G for context.

## 1. Core shell capabilities (the reusable prize)

New optional capability interfaces, discovered through `Ctx` and **nil when the
platform/browser can't provide them** (graceful degradation, like the File System
Access "unsupported" path). All callbacks fire on the UI goroutine.

```go
// shell/media.go
type Permission uint8
const ( PermissionPrompt Permission = iota; PermissionGranted; PermissionDenied )

type Facing uint8
const ( FacingBack Facing = iota; FacingFront )

type Camera interface {
    Authorize(func(Permission))
    Capture(CaptureOptions, func(img image.Image, err error)) // one still frame
}
type CaptureOptions struct { Facing Facing; MaxDim int } // MaxDim = longest-edge cap px

type Audio interface {
    Authorize(func(Permission))            // microphone
    Record(RecordOptions) (Recorder, error)
    Play(Clip) (Playback, error)
}
type RecordOptions struct{}                // reserved (sample rate, etc.)
type Recorder interface {
    Level() float32           // 0..1, poll per frame for the live meter
    Elapsed() time.Duration
    Stop() (Clip, error)
    Cancel()
}
type Playback interface {
    Position() time.Duration; Duration() time.Duration
    Playing() bool; Seek(time.Duration); Stop()
}
type Clip struct {
    Data     []byte        // encoded audio bytes
    Mime     string        // e.g. "audio/webm", "audio/wav"
    Duration time.Duration
    Envelope []float32     // downsampled 0..1 peaks for the waveform view
}
```

Access mirrors `ctx.Clipboard()` / `ctx.OpenURL`:

```go
ctx.Camera() Camera   // nil if unavailable
ctx.Audio()  Audio    // nil if unavailable
```

Wiring: the `Owner` gains `Camera`/`Audio` fields (nil by default); each shell sets
them from its window when supported. Only the web shell implements them in M1, so
desktop/terminal/mobile simply leave them nil — no per-platform stubs needed.

### Per-platform implementation

| | Camera | Audio out | Audio in |
|---|---|---|---|
| **Web** (M1) | hidden `<input type=file accept=image/* capture>` — native camera UI on mobile, file dialog on desktop; decode with Go `image` | Web Audio `decodeAudioData` + `AudioBufferSourceNode` | `getUserMedia({audio})` + `MediaRecorder` for the clip + `AnalyserNode` for live level/envelope |
| **Mobile** (M3) | AVCaptureSession / CameraX ImageCapture | AVAudioPlayer / AudioTrack | AVAudioEngine / AudioRecord → PCM |
| **Desktop** (M2) | *deferred* → file-pick | `oto`/`malgo` | `malgo` |

Web-first rationale: the browser gives all three with **no native code**, so we prove
the Go interface + UX on `localhost`, then implement the same interface on gomobile
with the contract settled. The web camera uses `<input capture>` (the idiomatic web
camera: real camera UI on mobile, file dialog on desktop) — a live `getUserMedia`
preview is later polish; the interface (`Capture → image`) is unchanged.

**Decisions**
- Audio format: **portable PCM16 WAV everywhere** (resolved at M1.5). The web
  recorder captures raw PCM via a ScriptProcessorNode and encodes WAV with the
  pure-Go codec (`shell/wav.go`), so a web recording plays back unchanged on
  desktop/mobile once those shells land — no format schism. `Clip` still carries
  `Data`+`Mime` in case a platform later prefers another codec.
- Capability discovery: **nil-return** from `ctx.Camera()`/`ctx.Audio()` (not error).
- Desktop camera: **file-pick fallback** first; native capture deferred.

**Permissions/manifest (M3):** iOS `NSCameraUsageDescription` /
`NSMicrophoneUsageDescription`; Android `CAMERA` / `RECORD_AUDIO`. The CLI's mobile
scaffolding injects these. `Authorize` triggers the prompt; the app shows a rationale
on denial. (On web, `<input capture>` needs no permission; mic recording prompts on
`getUserMedia`.)

## 2. Storage — extend `store` to binary blobs (M1.5)

The notes `store` is text-only; add blobs so photos/audio live beside entries
(desktop → `attachments/`; web → an FSA subfolder handle):

```go
WriteBlob(name string, data []byte) (id string, err error)
ReadBlob(id string) ([]byte, error)
RemoveBlob(id string) error
```

M1 keeps entries **in memory** to stay focused on the media pipeline; blob
persistence lands right after.

## 3. Journal app

**Data model** (one markdown file per entry + frontmatter; attachments by id):

```go
type Entry struct {
    ID string; Created time.Time
    Text string        // markdown body
    Photos []string    // attachment ids
    Audio  []AudioRef
}
type AudioRef struct { ID string; Duration time.Duration; Envelope []float32 }
```

**Screens (Navigator):**
1. **Timeline** — reverse `LazyList`, newest first; card = date + text preview + photo
   thumbnail + audio chip. "+" to compose.
2. **Composer** — multiline `TextField` (caret-into-view) + `[📷 Photo] [🎙 Record]`;
   photo → `Camera.Capture` → thumbnail; save writes entry (+ blobs at M1.5).
3. **Recorder** — big **Canvas** level meter/waveform from `Recorder.Level()` on a
   ticker + elapsed + Stop/Cancel.
4. **Entry detail** — markdown render + photos (tap → fullscreen) + audio player:
   waveform bar + progress cursor, tap-to-`Seek`, ticker updates `Position()`.

All widgets already exist (LazyList, Navigator, TextField, Canvas, Image, animations).
On platforms where `ctx.Camera()`/`ctx.Audio()` is nil (desktop M1), the app hides
those controls and stays text-only.

## 4. Milestones

- **M1 — web-first: shell media API + web impl + focused demo.** Capture a photo,
  record a voice memo (live meter), play it back with a scrubbing waveform, in a
  list — all in the browser. *Proves the Go interface with no native code.* Entries
  in memory.
- **M1.5 — blob store**: persist photos/audio + entries via the (extended) store, on
  web (FSA) and desktop.
- **M2 — desktop audio** (`malgo`); photos via file-pick.
- **M3 — gomobile bridges** (iOS + Android) implementing the same interfaces +
  permission strings + CLI scaffolding. *The real native work, de-risked by M1.*
- **M4 — stretch:** video capture (→ 1 Second Everyday mode) + a low-latency audio
  tier (unblocks tuner/metronome/games).

## 5. Status
- [x] M1 shell interfaces (`shell/media.go`) + `Ctx.Camera()`/`Ctx.Audio()` wiring
      (owner fields set from the window via `shell.MediaWindow` in `app.go`)
- [x] M1 web implementation (`shell/web/media_web.go`) — camera via `<input capture>`,
      audio record via getUserMedia + MediaRecorder + AnalyserNode, playback via Web Audio
- [x] M1 demo app (`examples/journal`) — compose photo + voice memo, live meter,
      waveform playback, in-memory timeline; text-only where capabilities are nil
- [x] M1.5 portable audio format — pure-Go WAV codec (`shell/wav.go`, tested); web
      recorder captures PCM → WAV so clips are cross-platform
- [x] M3 Go side — `shell/mobile.Bridge` implements `shell.MediaWindow` via a native
      `MediaHost` (request/deliver by reqID); camera + record→WAV + playback fully
      tested headless with a faked host (`shell/mobile/media_test.go`)
- [x] M3 native reference shims — iOS Swift + Android Kotlin `MediaHost` +
      permission notes (`shell/mobile/native/`), to verify on device
- [ ] browser smoke test (localhost, Chrome) — needs manual run
- [ ] M3 on-device verification (gomobile bind + host project wiring)
- [ ] M2 desktop audio (needs a backend dep decision: purego vs CGo)
- [ ] blob persistence (photos/audio + entries via the store)
- Known limits: photos decode JPEG/PNG only (iOS HEIC not yet); web audio uses the
  deprecated-but-universal ScriptProcessorNode (AudioWorklet is a later upgrade); no
  live camera preview (input-capture UI instead).
