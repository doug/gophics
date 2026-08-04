# Games on gophics: solitaire + a 2D side-scroller

## Context

Gophics is a Flutter-architecture UI framework in pure Go with four live platforms.
`design/example-app-ideas.md` §E already names games as the strongest *mobile* target
today — "casual/turn-based games need only touch + Canvas + animation + layout/grid —
all present" — while noting the framework is "not a game engine — no sprites/physics."

The question this plan answers: **what actually has to be built to ship a Klondike
solitaire and a real 2D action side-scroller on gophics, across desktop, web, iOS
and Android?**

The short answer, established by exploration:

- **Solitaire needed essentially nothing new — and is now shipped** (`examples/solitaire`).
  Drag, animation, canvas painting, damage tracking and headless golden tests were all
  in place, so it was built with zero framework changes: a pure, fully unit-tested
  `klondike` engine plus a bespoke-Canvas board with deal/flip animations, drag-drop by
  overlap area, tap-to-foundation, auto-complete, a **win cascade**, real card faces, and
  file-backed persistence. Desktop and web run today; mobile shares the same code and
  rides on the Stage-5 GPU-present bring-up.
- **The side-scroller is gated on five things**: sprite-atlas blitting, a real
  keyboard model with held-state polling, a low-latency sound mixer, display-list
  throughput, and — for mobile — a GPU present path (**built this session**; Go side
  done and API-validated, on-device verification pending — see §Stage 5).

Everything gated is a *primitive* gophics is missing, not a game engine. Adding
them makes the framework better at charts, whiteboards, data viz and the JJ-GUI
commit DAG too. That is the test applied throughout: **would this feature still earn
its place if no game were ever built on it?**

---

## The gophics decision (settled)

`~/src/gophics` is a creative-coding framework on its own concrete WebGPU renderer —
a struct that owns a `wgpu.Device`, queue, surface and pipelines, with wgpu types in
its public API. It has no pluggable-renderer seam. Importing it into a `Sketch` widget
would mean running **two renderers side by side**, not adding a widget.

Three options were weighed:

| | What you get | What it costs |
|---|---|---|
| **A. Extend gophics's `paint.Canvas`** ✅ | Sprites, paths, blend modes. Works on CPU fallback, works on mobile, one damage model, one binary, headless golden tests keep working | Nothing from gophics' shader/3D/feedback layers |
| **B. Embed gophics as a shared-device GPU texture layer** | WGSL shader passes, blur, ping-pong feedback, 3D with shared depth buffer | GPU-only ⇒ **breaks the mobile requirement**; two font stacks, two input models, two color types; device-sharing plumbing across two forked repos; loses `GOPHICS_RENDERER=cpu` and deterministic headless tests |
| **C. Offscreen `image.RGBA` bridge** | Ten lines, portable | Full GPU readback + re-upload per frame (~3.5 MB at 1280×720). Fine for a static panel, not 60fps |

**Decision: A.** The reason is not that gophics is worse — it is that gophics' value
for a *2D game* sits almost entirely in its renderer-independent half (`Vec2`,
`gmath`, `Rand`, easing/tween, camera, particles), which ports cleanly, while its
irreplaceable half (shaders, 3D, feedback) is exactly what a card game and a
side-scroller don't need.

Critically, **option A is cheap because the backend already does the work.** The
`gg` fork at `../third_party/gg` already implements every missing primitive —
`gg.DrawImageOptions` has `SrcRect`, `Opacity`, `BlendMode`, `Interpolation`; there
are path fills, ellipses, arcs, radial/sweep gradients, blend modes and masks.
gophics simply doesn't expose them through `paint.Canvas`.

Option B stays available later as an additive escape hatch for a shader-driven
visual toy — the seams exist on both sides (`gg.DeviceProviderAware` /
`gg.DrawGPUTexture`; gophics' `g3d.NewShared`). It is not foreclosed by choosing A.

---

## Where the framework stands

Verified against the code, not the docs.

**Present and good:** `widget.Canvas{W,H,Clip,Draw func(paint.Canvas, geom.Size)}`
custom-paint escape hatch (`widget/basic.go:1170`, doc comment literally says "game
boards"); `widget.Ticker{Tick(dt float64) bool}` self-perpetuating 60fps loop with
delta time; full drag gesture stack with slop, axis arbitration and priority
(`widget/widget.go:180`); `anim.Controller` + curves; affine `PushTransform`; clip
stack; group opacity; real text shaping; `app.NewHeadless` with
`Render()/Step(dt)/Tap/Drag/Key` for deterministic golden tests (~35 test files use it).

**Reference implementation to copy:** `examples/canvas/main.go` — a full-window
Canvas re-recording ~830 shapes plus live text every frame, driven by a ticker. That
file plus `widget/basic.go:1152-1232` is the entire surface a game sits on today.

**Live bugs found during exploration, both confirmed by reading the code:**

1. `shell/mobile/mobile.go:110` — `Bridge.Touch` never sets `Source: shell.SourceTouch`.
   Every touch on Android and iOS is reported as `SourceMouse`, so
   `Handler.DragPriority(touch bool)` receives `false` and selection/scroll
   disambiguation takes the mouse branch on phones.
2. `app/app.go:703-721` — `shellHandler.Event` has no `case shell.Focus`. Focus
   events are dropped entirely. This must be fixed before any held-key state exists,
   or alt-tabbing away mid-jump leaves a key stuck down forever. The web shell also
   registers no `blur`/`visibilitychange` listener, so web has no focus signal to drop.

Both are cheap, both are prerequisites, both land in Stage 0.

---

## Workstream 1 — Input

> **UPDATE: the core is built** (`7c233de`). The two bugs listed below were
> already fixed (mobile `SourceTouch`; the `shell.Focus` case). The **`input`
> package** now provides poll-style held state (`Down`/`JustPressed`/
> `JustReleased`/`Axis`/pointer, sticky edges), fed by the runner
> (`HandleKey`/`HandlePointer` before the keyboard focus early-return so polling
> is focus-free, `NewFrame` at frame end, `Clear` on blur) and read via
> `Ctx.Input()`. `shell.KeyCode` gained physical game keys (Space/WASD/digits,
> appended to keep the mobile ABI frozen), mapped in the desktop shell.
> `Headless.KeyDown/KeyUp` added; `examples/mover` is a real-time held-key demo.
> **Remaining:** the full W3C `KeyNamed` bridge for mobile, multi-touch tier-1
> (pointer IDs), and gamepad. What follows is the original analysis.

### The key model

**Adopt physical / W3C `KeyboardEvent.code` semantics** and say so explicitly in
`shell/shell.go` — this is the "pending M0 ADR" the current comment is waiting for.
Games bind WASD as a *shape*, not as letters; text input is already fully served by
the separate `shell.Text` + `shell.Composition` path, so `KeyCode` loses nothing by
becoming physical-only.

`shell.KeyCode` today has 16 values and no Space, no WASD, no digits, no function
keys. Widening it is **a pure mapping-function edit on desktop with zero upstream
work**: `gpucontext.Key` (`../third_party/gpucontext/events.go:124-240`) already
defines ~110 contiguous codes and all three desktop backends already populate them.
`shell/desktop/desktop.go:112` is just a 16-case switch that throws the rest away.

**Numbering is ABI-constrained.** `shell/mobile/mobile.go:282` `Bridge.Key(code int,
pressed bool)` passes the raw integer, and `examples/hn/android/.../MainActivity.kt:108`
hardcodes `Hnmobile.key(2, true) // Backspace`. So: **keep the existing 16 constants
first, in their current order, and append everything else at 16+.** One comment marks
them frozen. Then add a name-based bridge entry (`Bridge.KeyNamed(name string, pressed,
repeat bool)`, W3C code names) so the numeric ABI never has to grow again — hosts map
`KeyEvent.getKeyCode()` / `UIKey.keyCode` to names in ~40 lines of Kotlin/Swift.

Add `Repeat bool` to `shell.Key`, **synthesized in gophics** (a `KeyPress` for a code
already recorded as down is a repeat) rather than plumbed through `gpucontext`, whose
`OnKeyPress(key, mods)` has no repeat field and is shared across the gogpu ecosystem.

Per-shell work: `shell/shell.go` (table + `Repeat` + `String()`/`KeyCodeFromName`),
`shell/desktop/desktop.go:112` (arithmetic ranges over the contiguous enum),
`shell/web/web.go:139` (switch `e.key` → `e.code`), `shell/mobile/mobile.go:282`
(`KeyNamed`), `app/headless.go:120` (add `KeyDown`/`KeyUp` — today's `Key` is
press-only, which cannot test held state). **Terminal is a dead end and should be
documented as one**: a TTY sends no key-up, so keyboard games are out of scope under
`-tags gophics_term`.

The web `preventDefault` policy needs a rule rather than "suppress anything
recognized". Note this is **lower risk than it first appears**: `633eae9` already
suppresses every printable key without Ctrl/Meta (`shell/web/web.go:113-119`) and
already excludes command combos. The rule to make explicit: never suppress when
`ctrlKey||metaKey` unless the code is in the app-shortcut allowlist; never suppress
F1-F12 or Escape. Add `shell.Config.CaptureKeys []KeyCode` so a game opts in
explicitly instead of the framework guessing.

### Held-state polling — new `input` package

Games need `Down(KeyW)` / `JustPressed(KeySpace)` per frame, not callbacks.

**Build it in gophics from `shell` events; do not use `gogpu/input`.** That package
exists and is free on desktop, but it is desktop-only (web/mobile would each need a
parallel implementation that would drift), it is invisible to `app.NewHeadless` (so
game input could not be tested in `go test` — a stated house constraint), and it is
fed from gogpu's own dispatch, giving two sources of truth for "is W down" that
diverge precisely around focus loss. It is ~250 lines to own outright.

```go
package input // depends only on shell + geom; no build tags

type State struct{ /* bitsets + sticky edge sets + pointer/touch */ }

func (s *State) Down(k shell.KeyCode) bool
func (s *State) JustPressed(k shell.KeyCode) bool
func (s *State) JustReleased(k shell.KeyCode) bool
func (s *State) Axis(neg, pos shell.KeyCode) float32 // -1/0/+1 — the WASD helper
func (s *State) Mods() shell.Mods
func (s *State) Pointer() geom.Pt
func (s *State) PointerDown(button uint8) bool
func (s *State) Touches() []Touch
func (s *State) TextCapturing() bool   // a focused TextField owns the keyboard

// runner-side, not for widgets
func (s *State) HandleEvent(e shell.Event)
func (s *State) NewFrame()
func (s *State) Clear()
```

Wiring: `widget.Owner` gains `Input *input.State` beside `Camera`/`Audio`
(`widget/element.go`); `widget.Ctx` gains `Input()` beside `Audio()`
(`widget/widget.go:115`); `app.Core.Pointer`/`Core.Keyboard` (`app/app.go:488,553`)
call `HandleEvent` **before** dispatching to the focused target, so polling works with
zero focused widgets; `shellHandler.Frame` (`app/app.go:656`) and `Headless.Step`
(`app/headless.go:78`) call `NewFrame()` right after `drainPosted()`.

Two details that are easy to get wrong:

- **Sticky edge sets, not a prev/cur diff.** At 60fps a fast tap can go down and up
  between frames; a `cur && !prev` diff drops the jump. `HandleEvent` ORs into
  `pressedThisWindow`/`releasedThisWindow`, cleared by `NewFrame` after copying.
  (`gogpu/input`'s diff has exactly this bug.)
- **Focus loss must clear held keys** — hence the `case shell.Focus` fix above, plus
  a web `blur`/`visibilitychange` listener. Also clear defensively on a `KeyRelease`
  for a code that isn't down.

The focus-model tension resolves itself: held state is fed pre-dispatch and is global,
so it works whether or not anything has focus. The real problem is the inverse — WASD
moving the player while the user types — solved by the documented idiom
`if in.TextCapturing() { return }`, derived from the `Owner.KeyboardTarget != nil` that
`app/app.go:642` already computes. A proper focus-scope design is a **deferred ADR**,
deliberately not entangled here.

### Multi-touch — required for mobile, confirmed missing

`shell.Pointer` has **no ID field**, and `app.Core`'s gesture state is scalar (one
`pressed`, one `dragging`, one `downPos`). `shell/mobile/mobile.go:110`
`Touch(phase, x, y)` has no pointer index; `shell/web/web.go:78` reads `pointerId`
only to call `setPointerCapture` and discards it. So on a virtual d-pad, finger 2's
`PointerDown` overwrites finger 1's state and simultaneous move+jump is impossible.

**Two tiers.** Tier 1 (in this plan): add `ID int32` to `shell.Pointer` (0 = primary,
so the zero value keeps every existing caller and test correct), forward `pointerId`
on web, add `Bridge.TouchAt(pointerID, phase, x, y)` on mobile, accumulate the live set
in `input.State.Touches()`, and have `Core.Pointer` **ignore non-zero IDs for widget
gesture dispatch** — preserving today's exact semantics — while feeding them all to
`input.State`. A game canvas hit-tests its own d-pad from `Touches()`, which is what
game engines do anyway.

Tier 2 — a real multi-pointer gesture arena enabling pinch/rotate for *widgets* —
touches every gesture test in `app/` and needs its own ADR. **Explicitly out of scope.**

### Five input gotchas that shape both apps

These are behaviours, not bugs-in-waiting — each one will bite an app author who
doesn't know about it, and two of them change the *design* of solitaire.

1. **`OnPress` fires on every hit box, not just the topmost.** `Core.Pointer`'s
   `PointerDown` branch (`app/app.go:509`) loops all hits calling `OnPress`; only
   tap/drag pick a winner, and there is no event consumption. So a HUD button over a
   game canvas presses **both**. → ask: `Handler.Opaque bool`, ~10 lines in `Core.Pointer`.
2. **Setting `OnDoubleTap` defers *every* single tap by 300ms** (`doubleTapWindow`,
   `app/app.go:93`) to disambiguate. That would make solitaire's stock click feel
   broken. **Design around it rather than fighting it** — see the solitaire section.
3. **`OnRelease()` carries no position.** Track the last `OnDrag` pos. → ask:
   `OnRelease(pos geom.Pt)`.
4. **`TextInputActive()` returns `KeyboardTarget != nil`** (`app/app.go:644`), and
   `shell/mobile.Bridge` consumes it to raise the soft keyboard. **A game canvas with
   `OnKey` will pop the Android/iOS keyboard.** One-line fix: also require
   `t.OnText != nil`. This is a bug and must land before either game ships to a phone.
5. **No focus scopes.** The game canvas autofocuses at mount and nothing releases it on
   overlay push, so `theme.ShowDialog`'s scrim never receives Escape. Both games handle
   `KeyEscape` themselves for now; a proper fix (Overlay clears `KeyboardTarget` on push,
   restores on dismiss) is ADR-sized and deferred.

### Gamepad — Stage 4, deliberately last

Nothing exists (`../third_party/gogpu/input/input.go:11` says "Gamepads will be added
later"). Keyboard + touch is a complete shippable control scheme on all four targets,
so gamepad is pure upside — and macOS, the primary dev platform, is the hardest
backend (GameController.framework needs a run loop and block/KVO callbacks through
`goffi`'s Objective-C path; ~1-2 weeks, high risk). Ship the platform-agnostic model
(`PadID`/`PadButton`/`PadAxis` + `shell.PadWindow` + headless injector) first, then
backends in ascending difficulty: web `navigator.getGamepads()` (~1 day, low risk) →
Android/iOS host forwarding → Linux evdev via `golang.org/x/sys` → Windows XInput via
`goffi` → macOS last.

---

## Workstream 2 — Audio

> **UPDATE: a first cut is built.** The `sound` package is a pure-Go DSP mixer
> (Source/Osc/Tone/Gain + a thread-safe `Mixer`, DSP core adapted from the author's
> gophics/audio) with `Sample` playback voices (`Play`/`Loop`/`Stop`), procedural SFX
> (`Blip`/`Coin`/`Thud`/`Hit`), and a `ReadFloat32s` adapter. `sound/device` feeds it
> to the platform via the **`github.com/doug/audio`** fork's `Driver` (CoreAudio /
> PulseAudio / WASAPI / WebAudio), kept out of the pure package. `examples/roguelike`
> plays SFX on game events; the device opens and plays on macOS. This is the simple
> tier (mono clips, no pan/pitch/fade yet); the fuller `shell.Sound`/`Voice`/`Sink`
> design below is the next iteration. **Music decoding also landed**: `sound/ogg`
> and `sound/mp3` decode Ogg Vorbis / MP3 to a `Sample` (via jfreymuth/oggvorbis
> and hajimehoshi/go-mp3, pure-Go, in subpackages so the base stays decoder-free),
> alongside the synthesized `Drone`/`DungeonMusic` and voice `FadeIn`/`FadeOut`.

### What exists (as of this session's commits)

More than expected, and it moved during this session. `b6acfc6` landed the media
capture layer and `796b842` added `shell/wav.go` with `EncodeWAV(pcm []int16,
sampleRate int) []byte` and `DecodeWAV(b []byte) ([]int16, int, error)` — a portable
mono 16-bit WAV codec that the sound work should reuse rather than pulling in a second
decoder. `shell/web/media_web.go:39,48` implements `Camera()`/`Audio()`, and
`app/present.go` wires both. So `shell.Audio` is real on web.

But its **shape is wrong for games**: permission-gated, encoded bytes, one clip at a
time, callback-per-play. It is the capture/journal API and it should stay exactly as
it is. Game audio is a *separate capability*.

### New capability: `shell.Sound` + package `sound`

```go
// shell/sound.go
type SoundWindow interface{ Sound() Sound }

type PCM struct { Samples []float32; SampleRate, Channels int } // immutable after Load
type Sample uint32

type PlayOptions struct {
    Volume, Pan, Pitch float32 // zero means the natural value
    Loop   bool
    FadeIn time.Duration
}

type Voice interface {
    Stop(); FadeOut(time.Duration)
    SetVolume(float32); SetPan(float32); SetPitch(float32)
    Playing() bool
}

type Sound interface {
    Load(PCM) (Sample, error)
    Unload(Sample)
    Play(Sample, PlayOptions) Voice
    StopAll()
    SetMasterVolume(float32)
    Resume()  // unblock browser autoplay from a user gesture; no-op elsewhere
    Suspend() // release the device on background
    Close() error
}
```

**The key architectural move: gophics owns the mixer; the platform supplies only a
pull-sink.** That is what makes it uniform, testable, and independent of any one
audio library's design.

```go
// package sound
type Sink interface {
    Open(sampleRate, channels, bufferMs int) error
    Start(pull func(out []float32)) error   // pull runs on the platform audio thread
    Stop() error; Close() error
    SampleRate() int; Channels() int
}

type Mixer struct{ ... }              // implements shell.Sound over any Sink
func NewMixer(Sink, ...Option) (*Mixer, error)

type NullSink struct{}                // headless default — never starts a thread
type CaptureSink struct{ ... }        // tests pull frames and assert on them
type Bank struct{ ... }               // name-keyed convenience over embed.FS
```

**Do not build on `gogpu/audio`'s `Mixer`/`Player`** — use it for its `Driver`
implementations only. Its `Player` is one-shot and unreplayable (`Stop()` sets
`done=true` permanently), finished players are appended to `Context.players` and
**never removed** (`RemoveSource` exists but nothing calls it — an unbounded leak
within minutes of gameplay, iterated under a lock every callback), it round-trips
float32 → bytes → float32 per callback, and it has no pan, pitch, fade or loop.
`sound.Mixer` implements `audio.ReadFloat32er`, which plugs straight into
`audio.Driver.SetSource`, so the desktop/web sink is a ~60-line adapter.

**Threading contract, to be stated in the doc comment:** all `shell.Sound` methods run
on the UI goroutine; `pull` runs on a platform audio thread; commands cross via a
fixed-capacity ring drained at the top of each `pull` block; a full ring drops the
command and increments `Mixer.Dropped()` (dropping the 15th simultaneous footstep is
the right failure mode); no allocation, no logging, no widget access inside `pull`;
voice completion reaches app code via `Owner.Post`.

Headless wires `Owner.Sound` to `NewMixer(&NullSink{})` **unconditionally**, so
`ctx.Sound()` is never nil in tests and app code needs no nil-check test path.

### Per-platform

| Platform | Driver | Effort | Risk / watch-outs |
|---|---|---|---|
| macOS | `audio/driver_darwin.go`, AudioQueue via `goffi` dlopen | small | Callback enters Go from a C thread. `defaultBufferSizeMs=20` is a **const** — add `WithBufferSize` to the fork |
| Linux | `driver_linux.go`, `pa_simple` | small | **`pa_simple_new` is called with a NULL `pa_buffer_attr`**, so latency is at the server's discretion (100ms+). Passing an attr with `tlength` is a required fork change |
| Windows | `driver_default_windows.go`, WASAPI | small | Verify event-driven shared mode, not polled |
| Web | `driver_js.go`, WebAudio | small | **`ScriptProcessorNode` runs on the main thread** and contends with the rAF loop. Buffer is 4096 (≈93ms — unusable for a jump SFX); drop to 1024. `AudioWorklet` needs wasm-on-worker — deferred. Autoplay: `Resume()` from the first gesture |
| iOS | `driver_darwin.go` compiles for `GOOS=ios` | small Go | Needs host-side Swift `AVAudioSession` category or the ringer switch silences it. **Validate dlopen-of-system-framework on real hardware before committing** |
| Android | **Does not compile at all** | — | `GOOS=android go build` fails: `undefined: defaultDriver`. `driver_linux.go` is `linux && !android`, `driver_default_other.go` excludes linux — Android matches neither |

**Android recommendation: a host-pulled sink, not a native driver.** The mobile shell
is already "the host owns the loop, Go is a library" (the host drives `RenderFrame` on a
Go-owned Bridge). Audio should follow the same contract —
`Bridge.SoundOpen(rate, channels)` / `SoundMix(frames) []byte` / `SoundClose()`, fed to
Kotlin `AudioTrack` in `ENCODING_PCM_FLOAT` and Swift `AVAudioSourceNode`. This kills
three risks at once (no new FFI driver, no C-thread callbacks through gomobile, no
`AVAudioSession`-from-Go) and reuses the same `sound.Mixer` as every other platform.
Ship the 5-minute `driver_android.go` `NullDriver` stub immediately regardless — the
module is currently un-buildable for a required target, which is its own bug.

**Format reality:** WAV is fine for SFX (a 0.2s mono clip is ~9 KB) and hopeless for
music (3 min stereo 44.1 kHz ≈ 30 MB). Looping background music realistically waits
for Stage 4's `sound/vorbis` (`jfreymuth/oggvorbis`, MIT, pure Go) and `sound/mp3`
(`hajimehoshi/go-mp3`, Apache-2.0, pure Go), kept in **separate subpackages** so the
base module's dependency set doesn't grow. Opus is rejected — the mature binding is CGo.

---

## Workstream 3 — Rendering

### Correction to the "it's all just plumbing" premise

The early read — "`gg` already implements everything missing, so this is plumbing" —
is **only true on the CPU path**. Verified against the fork:

- `DrawImageOptions.Interpolation` and `.BlendMode` **are CPU-only.**
  `QueueImageDraw` (`internal/gpu/gpu_render_context.go:374`) carries position, size,
  opacity and UVs — **no blend, no interpolation**. The GPU sampler is hardcoded
  `FilterModeLinear` (`internal/gpu/image_pipeline.go:309-315`) and the pipeline blend
  is hardcoded `BlendStatePremultiplied()` (`image_pipeline.go:144`). **Nearest-neighbour
  pixel art and additive particles do not work on the GPU path** — which is the default
  on desktop and web.
- **Tint does not exist publicly at all** — only an offline `NewColorTintFilter` color
  matrix in `internal/filter/`.
- **Flip does not exist** in `DrawImageOptions`.
- **Rotation bails.** `isAxisAligned` (`context_image.go:401-411`) rejects any
  non-axis-aligned CTM and falls back to `SetFillPattern`→`Fill()` — the exact path
  `paint/paint.go:640-648` warns "forces a mid-frame accelerator flush that drops the
  queued shapes — fatal on the direct-surface path." **A single rotated sprite may
  corrupt or tank a GPU frame today.** Highest-priority thing to check.

So `Src` (source rect) genuinely is plumbing — the UVs are already in `QueueImageDraw`.
Tint, flip, nearest sampling, blend mode and rotation are **real work in the gg fork**,
sized at roughly 30–120 lines each.

### The predicted bottleneck is not what I assumed

I flagged gophics's per-op interface boxing as the throughput risk. The bigger one is
in gg: `buildImageResources` (`internal/gpu/render_session.go:1889-1990`) does **one
`CreateBindGroup` + one uniform `WriteBuffer` + one draw call per image, rebuilt every
frame**, with an unpreallocated append accumulating vertex bytes per quad. "Hundreds of
sprites" is exactly the workload that shape is worst at.

Related cliff: gg's GPU texture cache budget is **64 textures, LRU**
(`internal/gpu/image_cache.go:13`). A 52-card deck plus UI art is already at ~80% of
budget — this is a *solitaire-tier* cliff, not just an action-tier one. And gophics's
own `Painter.imgBufs` does a wholesale `clear()` at 256 (`paint/paint.go:783`), which
reassigns `GenerationID`s and invalidates gg's texture cache too, so a 257-sprite game
re-uploads every texture every frame.

### The Stage-1 unlock: atlases already work

`(*image.RGBA).SubImage(r)` returns an image whose `Pix` is offset to the sub-origin
and whose `Stride` is the atlas stride, and gg's `FromStdImage`
(`internal/image/io.go:196-221`) has a stride-aware path that handles it correctly.
So **"no source-rect ⇒ no sprite atlas" is true of the API but not of the outcome** —
solitaire can ship from one atlas PNG today with zero API change. What you can't do is
share *one GPU texture* across slices, which is what makes it insufficient for the
action tier and what runs straight into the 64-texture budget.

### Measure before building

The PLAN §6.4 figure of ~60ms/frame for full-scene animation predates GPU-by-default
(`90f8b48`) and is not evidence about today's system. **Stage 0 produces a numbers
table and nothing ships before it.**

Benchmarks: **B0** reproduce §6.4 · **B1 SpriteStorm** — N ∈ {50…2000} textured quads
at 1280×720@2x, crossed with 1/8/64/128 distinct textures (crosses the cache cliff) and
axis-aligned vs rotated (crosses the rotation bail) · **B2 RectStorm** (separates "gg's
image path is slow" from "gophics's display list is slow") · **B3 record-only**
allocs/op · **B4 diff-only** · **B5 CPU present** at phone resolution · **B6 real
mobile hardware**, not the SwiftShader emulator that produced the 89ms figure · **B7**
browser.

Decision rules, written down before running:

- B1 @ 500 sprites ≤ 6ms ⇒ **skip the gg instanced-sprite pipeline entirely** (the
  single largest gg item) and do plumbing only.
- A cliff between 64 and 128 textures ⇒ atlas + src-rect is **mandatory**.
- B1-rotated slower than 5× axis-aligned, or wrong ⇒ the rotation bail is a confirmed
  bug and jumps to the front of the queue.
- B3 record ≥ 2ms at 1000 ops ⇒ do the SoA rewrite. **Otherwise do not.**
- B2 @ 2000 rects already > 16ms ⇒ the ceiling is gg's rasterizer; escalate into the fork.

Budget for the action tier at 1280×720@2x, 500 sprites: build+layout ≤0.5ms, record
≤1.0ms, diff ≤0.2ms, replay ≤3.0ms, GPU submit ≤4.0ms ⇒ **renderer ≤8.7ms**, leaving
≥7.9ms for game logic. Solitaire's real requirement is different and stricter in one
way: **idle must cost zero raster** — battery, not throughput.

### The API: four methods, not sixteen

> **UPDATE:** most of this is **built**. The chart workstream landed `paint.Path` +
> `Canvas.FillPath` (`75a6c96`) and `Canvas.StrokePath` (`9f7a5d5`) — a *retained
> `*paint.Path`* (pointer + `Gen()` in the scene op, so `opEqual`'s `==` stays valid
> and `opBounds` uses `Bounds()`). Then **`Canvas.DrawSprite`** — a shared-atlas blit
> `Sprite{Src, Dst, Alpha, Tint, Rotation, FlipX, Nearest}`, one cached texture per
> atlas, with `Tint` (color-multiply via a tinted-texture cache) and `Rotation`
> (local `RotateAbout` — the "rotation bails on GPU" worry proved outdated). Driven by
> `examples/roguelike` (atlas tiles, flip facing, tint torchlight). **All CPU==GPU
> verified.** **Remaining:** batched `DrawSprites` (needs a gg instanced pipeline) and
> additive `Blend` (needs GPU blend-mode support) — both real gg-fork work, unlike the
> above.

```go
// appended to paint.Canvas
DrawSprite(img image.Image, s Sprite)
DrawSprites(img image.Image, sampling Sampling, blend Blend, in []SpriteInstance)
FillPath(p *Path, rule FillRule, c Color)
StrokePath(p *Path, style Stroke, c Color)
```

`Sprite{Src image.Rectangle, Dst geom.Rect, Tint Color, Alpha, Rotation float32, Flip,
Sampling, Blend}` — an options struct, because there are six independent axes
(method-per-combination is 2⁶; parameter-per-axis is a 9-argument call), because each
new *method* costs seven touchpoints, and because **`Sprite` is comparable**, which the
display list requires (see below). Zero-value-means-default follows the convention
`paint.Transform` already sets (`SX==0 → 1`).

**Paths as a retained `*paint.Path` value, not streamed `MoveTo`/`LineTo` ops** — six
reasons, all grounded in this codebase: ops are compared with `==`, so a streamed path
is N ops re-diffed every frame while a retained one is one pointer + generation compare;
`opBounds` is *impossible* for a lone `lineToOp`, so streaming would permanently poison
damage tracking; static geometry costs zero per-frame allocation; `gg.Path` is already
exactly this shape with `Bounds()`/`Transform()`/`Clone()` and `Context.FillPath`;
gg pre-tessellates fill paths at queue time, and a retained Path is the hook for caching
that; and it matches Flutter's `ui.Path`. Guard in-place mutation with a `gen` counter
carried in the op.

**Explicitly not adding:** `PushBlend`/`PopBlend` — `gg.PushLayer` allocates a
full-surface `Pixmap` per push and full-surface composites on pop
(`context_layer.go:66-79`), and swapping `c.pixmap` forces a GPU queue auto-flush. Games
want per-draw blend, which `Sprite.Blend` gives them. Also not adding: streaming path
commands, per-vertex meshes, nine-slice (compose from 9 `SpriteInstance`s),
`Ellipse`/`Arc` methods (use `Path`), radial/multi-stop gradients, `PushAffine`.

### The enabling refactor: make the op contract compiler-enforced

Two silent-bug classes exist today and both block the sprite work:

- `opEqual` is `a == b` on interface values (`scene/diff.go:85`), so **any op struct
  containing a slice, map or func panics at runtime**. A batched-sprite op literally
  cannot be added until this changes.
- `opBounds` (`scene/diff.go:87`) returns the zero rect for unhandled types and
  `ReplayDamage` culls zero-bounds ops — so a new op added without a bounds case
  **renders on GPU and vanishes on CPU, with no compiler error.**

Fix both by giving `op` real methods — `replay`, `bounds`, `structural`, `equal`.
Behavior-neutral, ~120 lines, covered by the existing `app/damage_test.go` and
`app/gpu_equiv_test.go`. **Non-negotiable prerequisite for everything else.**

### Damage and games

`widget.Canvas` forces full-surface damage because its `Paint` always pushes a
transform. That matters less than it first appears: **on the GPU present path damage is
ignored entirely** (`app/present.go:36-41` replays the whole scene every frame), so on
the primary (GPU) path of **all four** targets — mobile now included — it costs only a
wasted `Diff` and never-skipped frames. It costs real pixels only on the CPU render
paths: `GOPHICS_RENDERER=cpu`, mobile `Snapshot`, and every headless test.

Three layers, cheapest first:

1. **Pure-translate transforms stop poisoning damage.** The recorder keeps a
   translation stack and bakes the offset into recorded coordinates when the whole stack
   is pure translation, emitting no transform op at all. **This benefits every app in
   the framework** — today every `Transform` widget and every `Canvas` nukes damage.
2. **`widget.Canvas.Damage func(size geom.Size) geom.Rect`** — the game declares its own
   dirty rect, because it knows it infinitely better than a positional op-diff. Solitaire
   returns the union of moving card rects during a deal and an **empty rect when idle**.
   A wrong `Damage` degrades to visual staleness, never a crash, and `GOPHICS_NO_DAMAGE=1`
   already exists to bisect it.
3. **(Named now, built later)** A `RepaintBoundary`-style retained surface: the game
   renders into its own offscreen texture and the UI composites it. gg already has
   `CreateOffscreenTexture` and `DrawGPUTexture`. This gives pause-menu-over-frozen-frame
   for free *and* per-surface resolution — separately the strongest mobile lever.

### Mobile GPU present — the largest item, and the tail

> **UPDATE (built this session, commits `6d7ce0d` + `691422b`).** The Go side of this
> is done and API-validated against wgpu/ggcanvas: `shell/mobile.Bridge.SetSurface`
> creates a wgpu device+surface from a native handle and `mobileGPUTarget` presents via
> `ggcanvas.RenderDirect` (a near-verbatim copy of `webProvider`, as predicted). Mobile
> is now **GPU-only**: the CPU host-blit path was dropped, `RenderFrame` presents to the
> surface, and `Snapshot` is the offscreen/CPU path kept for headless tests. The `hn`
> hosts were converted (iOS CAMetalLayer; Android ANativeWindow via an NDK/JNI shim).
> What remains of this section is **on-device bring-up** (rotation, surface loss) and
> multi-touch — not the from-scratch integration described below. The rest of this
> section is kept as the original analysis.

> **UPDATE 2 (2026-08-03) — bring-up ran on a real Pixel 10 Pro. The Vulkan-Android
> preview holds; the blocker moved to gg's rendering tiers.** Full log in
> `design/mobile-gpu-bringup.md`. Three bugs stood between "built" and "presents on
> device", none of them the ones this section predicted:
>
> 1. **The mobile binaries contained zero GPU backends.** wgpu only links backends
>    you import; the desktop shell gets them via gogpu's renderer, which the mobile
>    build never pulls in. So `hal/vulkan` was never in the binary and *every* device
>    was guaranteed to fall back to the CPU blit. The Stage-0 "clear a surface red on
>    a real Pixel" spike would have caught this in an hour — it was the right call and
>    skipping it cost the most. Fixed by `shell/mobile/backends.go`.
> 2. **Surface format was hardcoded `BGRA8Unorm`.** PowerVR's swapchain offers only
>    `RGBA8Unorm`, so `Configure` failed, the GPU reported ready, and every frame died
>    on "surface is not configured". Now negotiated from `GetSurfaceCapabilities`.
> 3. **16 KB page alignment.** Pixel 10 reports `pagesize.max=16384`; 4 KB-aligned
>    native libs trip Android's "app isn't compatible" dialog. Both libs now link with
>    `-Wl,-z,max-page-size=16384`.
>
> **Result: ~4-5 ms/frame on Tensor G5 / PowerVR D-Series at 1080x2238 @2.625x**,
> against ~117 ms for the CPU blit it had silently been using. That is a 23x margin
> and it retires the "will an action game hit 60fps on a phone" question below —
> comfortably, with room for 120 Hz.
>
> **The surface plumbing is correct.** Colors, gradients, path fills, device scale
> and touch all match the desktop reference pixel-for-pixel in structure. What fails
> is two of gg's draw tiers, detailed in the risk register (#1, #6). Note the
> schedule risk this section named — rotation and surface loss — is **still
> unmeasured on the GPU path**: the lifecycle exercise ran while the app was on the
> CPU fallback, so it validated the fallback, not the handoff.

**Honest assessment: a real action game will not hit 60fps on the CPU rasterizer at
phone resolution.** The §6.4 figure was at 1.54 Mpx; a modern phone is 2.6 Mpx. And
`asRGBA` + the `[]byte` handoff + the host's blit alone are ~10 MB/frame at 1080p —
**630 MB/s at 60fps before drawing anything.**

Two cheap mitigations that should ship regardless and be measured *before* committing to
the GPU path: **reduced backing scale** for the game surface (the hook already exists —
`Bridge` reports `FrameWidth`/`FrameHeight` separately from surface size precisely so
the host can scale the blit), and **`Bridge.RenderFrameRect`** so the host uploads only
the dirty sub-rect, composing directly with the `Damage` hint above.

**Good news: the surface plumbing already exists in the fork.** This is integration
work, not new-backend work — `wgpu.SurfaceTargetFromAndroidNativeWindow` and
`SurfaceTargetFromMetalLayer` are in the pure-Go path
(`../third_party/wgpu/surface_native.go:66,185`), `hal/vulkan` has an Android backend,
and `hal/metal` builds under `darwin` which `GOOS=ios` satisfies. As shipped,
`shell/mobile` gained `SetSurface(displayHandle, windowHandle int64, widthPx, heightPx
int, scale float32)` (`int64`, not `uintptr` — gomobile rejects `uintptr` in exported
signatures) creating the surface via `Instance.CreateSurface(display, window)`;
`RenderFrame` presents on the GPU (no CPU return) and `frame.Target()` returns the GPU
target when a surface is bound. **Nothing above `shell` changed** — `app/present.go`
already selects per-frame. That is the existing abstraction paying off.

The genuinely new native code is small but real: `ANativeWindow_fromSurface` is an NDK
**C** API with no Java accessor, so ~40 lines of JNI/C plus CMake in the host build.
And the schedule risk is concentrated in real-device bring-up — Android destroys the
surface on **every rotation** and on backgrounding, needing bind/unbind plus
`Outdated`/`Lost` reconfigure-and-retry.

**Two Stage-0 spikes with enormous schedule leverage:** `hal/vulkan/doc.go:31` calls
Android/arm64 an explicit **preview** — that is the highest-uncertainty dependency in
the whole plan, so spike a 50-line program that clears a surface red on a real Pixel
(~1 day). And build `hal/metal` for `GOOS=ios` (~1 hour). If the first fails, the mobile
action tier is blocked on upstream Vulkan work and must be re-scoped.

---

## Workstream 4 — The `game` package and the two apps

### The game-loop idiom (verified, and it already works)

A game must **not** rebuild the widget tree 60×/second. It doesn't have to:

- `RecordScene` (`app/app.go:310`) unconditionally `Reset()`s and re-paints the whole
  box tree every frame, so `canvasBox.Paint` → `b.draw(c, size)` runs the captured
  closure each frame regardless of dirtiness.
- A `Ticker` returning `true` already triggers `w.Invalidate()` (`app/app.go:682`),
  which calls `Owner.requestFrame()` — **it does not mark anything dirty**, so no
  rebuild happens.

So the idiom is: mutate game state directly in `Tick`, return `true`, and have the
`Draw` closure read **through the state pointer**:

```go
func (s *gameState) Tick(dt float64) bool {
    s.world.Step(dt)   // no SetState — no rebuild
    return true        // keeps frames coming; Draw re-reads s.world
}
func (s *gameState) Build(widget.Ctx) widget.Widget {
    return widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
        s.world.Render(c, size)   // reads s live, built once
    }}
}
```

Note `examples/canvas` does it the **wasteful** way — it calls `SetState` every frame
and copies `t := s.t` out of the state before building the closure. The cost isn't the
one-widget rebuild; it's that `element.update` → `updateBox` → **`markBoxChainDirty()`**
walks to the root calling `layout.MarkDirty` on every ancestor, invalidating the whole
layout skip-cache spine 60×/second. In a one-widget demo that's noise. In a game screen
(`Stack{Loop, HUD Column{Text, Text}}`) it re-lays-out and re-shapes the HUD text every
frame. **This warrants an ADR and a fix to `examples/canvas`, because it is the
reference every game author will copy.**

**A second, independent full-repaint cause:** `canvasBox.Paint` *always* calls
`PushTransform{TX: at.X, TY: at.Y}`, and `scene.recorder.PushTransform`
(`scene/scene.go:170`) sets `hasLayers = true` → `RecordScene` escalates to full-surface
damage. For a full-bleed canvas `at` is `(0,0)`, so this is an **identity transform
defeating damage tracking**. Two cheap fixes, either of which pays off immediately for
solitaire and enormously on mobile: skip the push when `at == geom.Pt{}`, and don't set
`hasLayers` for a pure-translate transform (a translate *is* bounds-mappable, unlike
scale/rotate).

### Package scope — hold the line on "not a game engine"

PLAN.md lists a game engine as a permanent non-goal. The line this plan holds:
**gophics core gets primitives** (sprite blit, blend, paths, keys, held-state, sound
mixer — each justified by non-game callers too); **a thin `game` package gets
conveniences**; **anything above that lives in the example app, not the framework.**
The test for every item: *would this still earn its place if no game were ever built
on it?*

```
game/
  loop.go      Loop widget: Canvas + Interactive + Ticker, fixed-timestep
               accumulator (logic at a fixed 1/60, render interpolated)
  vec.go       Vec2 — port from gophics/vec2.go
  math.go      Lerp/Clamp/Map/Norm/Wrap/Radians/Sign — port from gophics/gmath.go
  rand.go      deterministic Rand — port from gophics/rand.go (seeded; replays)
  camera.go    world↔screen, follow with deadzone, parallax — adapt gophics/camera.go
  sprite.go    SpriteSheet: atlas + named animation clips + frame timing
  tilemap.go   tile grid, AABB queries, culled draw
  collide.go   AABB sweep + resolution (new — Verlet is wrong for a platformer)
  particles.go emitter — adapt gophics/particles.go onto paint.Canvas
```

**Reconcile, do not duplicate** — the port decisions that matter:

- **Do not port `Vec2`.** `geom.Pt{X, Y float32}` is bit-identical. Add the missing
  methods (`Len/Dot/Cross/Normalize/Limit/Rotate/Angle/Perp/Dist`) **to `geom.Pt`**. A
  parallel `Vec2` would force a conversion at every `FillRect` call site.
- **Port `gmath` non-generically.** ADR 0001 makes everything float32, so the `Float`
  constraint buys nothing; Go's `min`/`max` builtins cover the rest. Add
  `Clamp/Wrap/Sign/Map/Norm/Approach` beside the existing `LerpFloat`.
- **Port `Color` constructors only** — `Hex/HSV/Gray/FromColor` into `paint`.
  `HSV` in particular is what makes procedural palettes and damage tints pleasant.
- **Port easing *formulas*, not the `easing` package** (float64 + an `Easer` interface
  is the wrong shape) — as `anim.Curve` funcs. Add `Controller.Repeat/Reverses`.
- **Port `rand.go` minus the package-level global source** — a mutable global RNG is
  anti-testable and gophics has none. **Add `Shuffle[T]`**, which gophics lacks and
  solitaire needs.
- **Do not port `physics.go`. This is a trap.** Position-Verlet with distance
  constraints is a cloth/rope solver: velocity is *implicit* in `Pos - Prev`, which
  actively fights every platformer feel primitive — you cannot cleanly cut vertical
  velocity for variable jump height, and coyote time and fixed ground speed both fight
  the integrator. It will "work" for a demo and then make the controller unfixable.
  Write swept-AABB-vs-tilemap with separate-axis resolution instead.
- **Do not port `scene.go`.** Gophics already has two retained trees (`element` and
  `scene.List`); a third is the engine the non-goals forbid.
- **Do not introduce a `Sketch{Setup,Update,Draw}` analog.** `Initer.Init` is Setup,
  `Ticker.Tick` is Update, `Canvas.Draw` is Draw. What's worth adding is `game.Loop` —
  a widget that owns the accumulator, not a lifecycle interface.

### `examples/solitaire` — Klondike (✅ shipped this session)

**Architecture: one full-screen `widget.Canvas` doing bespoke card layout and its own
hit-testing.** Not 52 widgets — `layout.Stack` paints every child at the same origin
(`layout/layout.go:426-430`) and there is no `Positioned`, so widget-per-card means
`Padding{Insets}` churn through layout on every drag frame. A card game's layout is
bespoke anyway. **Do not add `Positioned` for this** — add it when a second caller
appears.

- **Game state is pure and rendering-free**: deck, seven tableaus, four foundations,
  stock/waste, legal-move rules, undo stack, win detection. 100% `go test`-able with no
  framework at all — this is the majority of the code and it has zero dependencies.
- **Card art: draw everything, embed no PNGs — shipped this way.** Body = `FillRRect` +
  `StrokeRRect` + `paint.DropShadow`; back = `FillRRectGradient` + a rotated-square
  motif; empty slot = a `StrokeRRect` ghost; a subtle felt gradient underneath.
  **Suit pips — the plan predicted `goregular` would lack ♠♥♦♣ and that they'd be
  constructed from primitives; that turned out false. `goregular` *has* all four**, so
  the deck renders them as shaped text: the traditional pip arrangement (N symbols per
  rank in the standard grid, lower-half pips rotated 180°), an Ace with one large central
  pip, court cards with a large rank letter, and two opposing corner indices (top-left +
  a rotated bottom-right), with glyph centering via calibrated offsets (the Canvas has no
  measure API). Ranks via `TextIn`/`Text` with real shaping. Still zero binary assets and
  zero new Canvas primitives. (The constructed-shape and atlas-PNG routes both stay
  viable but proved unnecessary — the atlas route would also need 53 textures against a
  64-texture GPU budget and can't recolour for dark mode.)
- **Drag** via `widget.Interactive{Handler{OnPress, OnDrag, OnRelease}}` wrapping the
  Canvas, hit-testing top-down through the stacks. Compute the drop target by
  **maximum overlap area against the dragged card's rect, not the pointer position** —
  this is what makes drop feel right. `DragPriority`/`DragAxis` aren't needed; nothing
  nests.
- **❗ Do not set `OnDoubleTap`** — it would defer every single tap by 300ms and make
  the stock click feel broken. **Design around it: single tap attempts an auto-move to
  a foundation, drag is the manual move, tap on the stock draws.** This is better on
  touch anyway and is what modern solitaires do.
- **Undo is a move journal, not snapshots** — O(1), and it forces the
  `FlippedSource bool` field to be explicit on each `Move`. That bit (did this move
  expose a face-down card, so undo must re-hide it?) is *the* classic Klondike undo bug;
  naming it in the type is the point.
- **Cmd/Ctrl+Z is impossible today** — `shell.KeyZ` doesn't exist. Ship the button; add
  the shortcut when the key table lands in Stage 2.
- **Acknowledged cost: a Canvas-drawn board is invisible to the a11y bridge**
  (`Core.A11yTree` walks render boxes), and gophics just landed TalkBack/VoiceOver.
  Mitigation is a later semantics overlay — transparent `widget.Semantics{Role, Label}`
  nodes positioned by `Padding` from the same pure `board.Layout` function, ~80 LOC.
  This is deliberate debt, not an oversight.
- **Animations** via `anim.Controller`: deal, flip, invalid-move snap-back, auto-complete.
- **Damage**: return the union of moving card rects, and an **empty rect when idle**.
  This is the feature that makes it cost zero battery at rest, which is the actual
  product requirement for a card game.
- **Persistence**: reuse the `examples/notes` file pattern (File System Access on web).
- **Tests**: `app.NewHeadless` golden images for deal/drag/win, plus pure unit tests for
  the rules engine.

**Framework changes required: none.** Only the two bug fixes and the two cache-budget
adjustments, all of which are wanted independently.

### `examples/scroller` — the action-tier driver

**The real project risk here is art, not code**, and the fix is structural: ship a
`Skin` interface with two implementations.

```go
type Skin interface {
    Background(c paint.Canvas, cam *game.Camera, view geom.Size)
    Tile(c paint.Canvas, m *game.Tilemap, x, y int, dst geom.Rect)
    Actor(c paint.Canvas, kind ActorKind, frame int, dst geom.Rect, flip bool, tint paint.Color)
}
```

- **`skinFlat`** — rounded rects and a good palette, the Thomas-Was-Alone / Downwell
  aesthetic. Needs **zero new Canvas features** and is genuinely attractive.
- **`skinPixel`** — sprites generated **procedurally in Go at init**. Zero binary
  assets, deterministic, and — the part that matters — **palette-swappable, which gives
  enemy variants and damage-flash tint for free, closing the "gg has no tint" gap**, and
  regenerable at the device pixel ratio so blits are 1:1 and nearest-vs-bilinear
  becomes moot.
- A CC0 pack (Kenney) stays available as a documented opt-in third skin behind a build
  tag, but it adds 200–500 KB of third-party binaries and a provenance file to a repo
  whose ethos is "pure Go, minimal deps."

**This is the schedule de-risk: the demo is shippable and pretty before sprites exist**,
so sprites become a visible *upgrade* that demonstrates the new blit path rather than a
blocker gating the whole example. It also makes art a swappable, independently
golden-testable axis.

One framework gap this surfaces: generating sprites at the right resolution needs the
device pixel ratio, and `Canvas.Draw(c, size geom.Size)` has neither it nor a route to
it (`Painter.scale` is unexported, `shell.Frame.Scale()` is unreachable from a widget).
→ ask: **`Ctx.DevicePixelRatio() float32`**.

**Level format: plain-text ASCII maps in `levels/*.txt`, `go:embed`-ed.** Diffable in
git, editable in any editor, **reviewable in a PR**, ~2 KB per level, zero tooling, zero
importer, zero codegen — exactly the gophics grain. The parser is ~60 LOC. The trick
that makes it look authored rather than programmer-art: `Tilemap.Neighbours(x,y)` returns
a 4-bit same-class mask, so the skin picks the right edge/corner/interior tile from a
16-tile autotile set and picks decorative variants from `hash(x,y)`. ~30 LOC, and it is
the difference between "a grid" and "a level."

Scope discipline: **3 short levels, 2 enemy types (a patrolling walker and a stationary
hopper that fires on a cooldown), 1 collectible, 1 goal, checkpoints.** Cut deliberately:
shooting, wall-jump, dash, ladders, bosses — and **moving platforms**, which are
tempting but account for a disproportionate share of all platformer collision bugs
(kinematic carry velocity, crush resolution, riding through one-ways). The README calls
it a technology demo, not a game.

**Tunnelling is the #1 platformer bug.** No swept CCD in v1; instead bound `|delta|` per
fixed step below `TileSize` — which `maxFall` and `Step: 1/60` make satisfiable — and
assert it with a `TestNoTunneling` that drives the maximum possible delta into a 1-tile
wall 10k times from randomised sub-tile offsets. Naming the invariant and testing it
*is* the mitigation.

- **Controls**: `input.State.Axis(KeyA, KeyD)` + `JustPressed(KeySpace)`, guarded by
  `if in.TextCapturing() { return }`. Arrow keys already work today, so a playable
  keyboard scroller needs no framework change at all.
- **Touch, and the design that unblocks it.** Gophics is single-pointer, so you cannot
  hold "right" and tap "jump" simultaneously until multi-touch lands. Rather than let
  that gate the demo: **auto-run.** The player always runs forward; tap = jump, hold =
  higher jump. That is one pointer, it's a legitimate genre, and it plays well on a
  phone. Ship it as `Settings.AutoRun`, defaulted on for touch; multi-touch later
  unlocks an optional two-thumb mode. **A schedule blocker becomes a design choice.**
- **Device-independent intent layer.** Keyboard, touch zones and (later) gamepad all
  write into one `Buttons{MoveX float32; Jump, JumpHeld, Pause bool}`; the game only
  ever reads that. This is what makes adding gamepad in Stage 6 a non-event.
- **Camera** follow with deadzone; parallax layers as scaled scroll offsets.
- **Collision**: AABB sweep against the tilemap, resolved axis-separately.
- **Sound**: jump/land/coin one-shots through `sound.Bank`; music waits for Stage 4's
  Vorbis decoder (a 3-minute stereo WAV is ~30 MB).
- **Pause menu** via `theme.ShowDialog` over a frozen frame — which is exactly the case
  the retained-surface layer (Rendering L3) would make free.

**What it looks like at each stage** — the point being that it is runnable throughout:

| Stage | The scroller is… |
|---|---|
| 2 | **grey-box**: solid rounded rects on a tile grid — but with a real fixed-timestep sim, coyote-time jumping that already feels good, a smoothed deadzone camera, a gradient parallax sky and a coin counter. Ugly, and genuinely fun to move around in. **This is the milestone that proves the thesis.** |
| 3 | a **complete flat-vector platformer** (`skinFlat`) — 2 enemies, hearts, checkpoints, 3 levels, particles, camera shake, SFX. Fully shippable on desktop + web with no further framework features |
| 4 | the same game with `skinPixel` sprites, autotiled terrain, additive particles and damage flash; hundreds of entities at 60fps, measured |
| 5 | the same thing at 60fps on a real phone, auto-run touch controls |

**The determinism prize.** With `Step` fixed and `game.Rand` seeded, calling
`Headless.Step(1.0/60)` N times is a **bit-exact replay**. An input trace
(`[]struct{Frame int; Ev any}`) replayed headlessly must produce an identical final
state *and* identical rendered pixels. That is a stronger testing story than any
mainstream engine offers, it costs almost nothing given the fixed-timestep design, and
it should be an explicit test in both examples.

---

## Staging

Each stage ends runnable and demonstrable.

**Stage 0 — Spikes and measurement (~3 days, no product code).**
B0–B5 land as committed benchmarks (`app/bench_test.go` and `app/gpu_equiv_test.go`
already exist as homes). **Android Vulkan hello-surface on real hardware** — the single
highest-leverage day in the plan, since `hal/vulkan` calls Android/arm64 a *preview*.
`GOOS=ios` build of `hal/metal` (~1 hour). Golden test proving `SubImage` atlas slicing.
**Exit: a numbers table that gates Stages 3–5.** Anything built before it is a guess.

**Stage 1 — Foundations + a provably correct Klondike (~2 weeks, LOW risk).**
Framework: the two bug fixes (`SourceTouch`, `case shell.Focus`) plus the
`TextInputActive` soft-keyboard fix. `geom.Pt` vector methods + scalar helpers; `anim`
easing curves + `Controller.Repeat`; `paint` color constructors. `Painter.imgBufs`
256-then-clear → LRU; gg image cache budget 64 → 512; `RecordScene` skips the `Diff` it
discards; the `op`-interface refactor. **Plus the ADR** on the game-loop idiom and the
fix to `examples/canvas` so the reference stops teaching per-frame `SetState`.
Then `game/{game,rand,clock,loop,input,timer}.go`, and
`examples/solitaire/klondike/` — **complete, fully unit-tested, zero rendering imports.**
**Demo: `go test ./examples/solitaire/klondike` proves a correct Klondike — legality
matrix, stock recycle, undo/redo including `FlippedSource`, auto-complete termination,
and a fuzz test asserting 52 distinct cards survive 10k random legal moves. The pure
model banked before a single pixel.**
**✅ Built this session: the `klondike/` engine (card / game / snapshot / auto-complete)
with unit tests, rendering-free.** (The framework bug-fixes and `game/` package listed
above remain to do — solitaire simply didn't need them.)

**Stage 1b — Solitaire playable and shipped (~2.5 weeks, LOW risk). ✅ DONE this session
on desktop + web.**
Board layout as a pure `Layout(size, game) Board` function, top-down hit-testing, the
deck rendered with shaped-text suit glyphs + standard pip layouts (see the card-art note
above), drag/drop with overlap-area targeting, tap-to-foundation,
deal/flip/auto-complete/win-cascade animations, persistence via the `examples/notes`
`Store` pattern. Mobile targets share the same code and ride on Stage 5's GPU present.
**Gate remaining: on-device mobile bring-up + benchmark frame cost before the phone
ship.** Built with essentially zero renderer change — the first shippable artifact.

**Stage 2 — Damage + key model + held-state (~2.5 weeks, MEDIUM risk).**
Pure-translate transforms stop poisoning damage; `widget.Canvas.Damage`. Full
`shell.KeyCode` table (ABI-safe append) + `Repeat`; desktop/web/mobile mapping;
`Headless.KeyDown/KeyUp`. New `input` package with sticky edges; `Ctx.Input()`; focus
clearing everywhere.
**Demo: solitaire idles at zero raster; a rectangle runs and jumps with WASD at 60fps
on desktop and web, with a headless test asserting jump-on-`JustPressed`.**

**Stage 3 — Sprites, paths, sound (~4 weeks, MEDIUM risk).**
`paint.Sprite`/`DrawSprite`, `paint.Path`/`FillPath`/`StrokePath`. gg fork work in
dependency order, each independently shippable: flip → nearest sampler → tint →
**non-axis-aligned quads (jumps to first if the rotation bail is confirmed)** → per-draw
blend. In parallel: `shell.Sound` + `sound` package + desktop/web sinks + the
`driver_android.go` stub that unbreaks the Android build.
**Demo: side-scroller vertical slice with real sprites, tilemap, parallax and SFX on
desktop + web.**

**Stage 4 — Batch + throughput (~1.5–3 weeks, gated on B1/B3).**
`DrawSprites` + batched op. gg instanced sprite pipeline — **skipped entirely if B1
@500 sprites already fits budget.** SoA display list **only if B3 demands it**.
`sound/vorbis` for music.
**Demo: 500+ sprites plus particles at 60fps, measured.**

**Stage 5 — Mobile GPU present (mostly DONE this session; on-device tail remains).**
The core landed already (`6d7ce0d`, `691422b`): `SetSurface` + `mobileGPUTarget` (the
`webProvider` copy), GPU-only `RenderFrame` with `Snapshot` for offscreen, the Android
SurfaceView host + NDK/JNI `ANativeWindow` shim, and the iOS CAMetalLayer host. What's
left is **real-device bring-up** (rotation, background, surface `Outdated`/`Lost`
reconfigure) and multi-touch tier 1 — plus the still-optional interim wins (reduced
backing scale, `RenderFrameRect`). **Demo: the side-scroller at 60fps on a real Pixel
and iPhone.**

**Stage 6 (optional).** Gamepad (web → mobile hosts → Linux → Windows → macOS last).
Retained game surface + per-surface resolution.

**Roll-up: ~4–5 months focused. A correct Klondike proven by tests — ✅ done; solitaire
shipped on desktop + web — ✅ done (mobile riding the GPU-present tail); a complete
flat-vector platformer on desktop + web by ~week 11 of the remaining work; sprites by
~week 14; mobile is the tail.** The tail is caused by the
Vulkan-preview and NDK-shim risks, not by gophics's architecture — which already has
the right seam (`app/present.go`'s per-frame `Target` selection) for mobile GPU to drop
in without touching a line above `shell`.

### Scope-creep guard

Put the non-goals list in `game`'s package doc and make it a review gate. Proposed
wording: *"`game` provides the renderer-independent pieces a 2D game needs that
gophics's UI layers have no reason to contain. It provides no entity system, no scene
graph, no rigid-body solver, no asset pipeline, no audio, and no scripting. Games own
their own state structs. If a proposed addition would be at home in Unity's
`Assets/Scripts`, it does not belong here."* Consider holding `tilemap.go` and
`sprite.go` inside `examples/scroller/` until a second consumer exists.

### Explicitly out of scope (named so they don't creep in)

Multi-pointer *widget* gesture arena (pinch/rotate) · focus scopes and keyboard
traversal · web `AudioWorklet` (wasm-on-worker) · Kitty keyboard protocol for the
terminal shell · native Android AAudio driver · `PushAffine`/shear · radial and
multi-stop gradients · nine-slice · per-vertex meshes · embedding gophics' renderer
(revisitable later, additively).

---

## Risk register

Ordered by threat to the plan.

1. **GPU text renders as solid blocks on Vulkan — the mobile blocker (confirmed on
   device, 2026-08-03).** Every string on the Pixel 10 Pro draws as opaque blobs, one
   per glyph, fully illegible. Glyph *positions and advances are correct* — the block
   run matches the string's letter grouping — so shaping and layout are fine and the
   fault is isolated to coverage. On the GPU path `paint.go:704` routes text to
   `fillGlyphs`, which prefers gg's **glyph-mask tier** (`DrawShapedGlyphs`,
   `paint/paint.go:746`): each glyph is rasterized into a device-resolution atlas and
   batched as quads. Every quad sampling as fully-covered is exactly the symptom of a
   wrong mask **texture format or sampler** on the Vulkan backend — the same class of
   bug as the surface-format mismatch above, and worth checking first. It does not
   reproduce through the Metal reference render, so it is Vulkan-specific. **Until
   this is fixed no text-bearing UI is shippable on Android**, which is most of them.
   Note the fallback below it (outline fills) is *not* obviously affected, so forcing
   that tier is a plausible stopgap if the atlas fix is slow.

2. **Mobile GPU bring-up itself is done and the Vulkan-Android preview held.** The
   original form of this risk — "unverified on-device, and `hal/vulkan` calls
   Android/arm64 a preview" — is **retired**: Vulkan came up on Tensor G5/PowerVR at
   ~4-5 ms/frame (see the section above). The mobile action tier is **not** blocked on
   upstream Vulkan work. Two residuals remain: `allbackends` registers Vulkan on
   **android/arm64 only**, so the x86_64 emulator has no backend and stays CPU-blit by
   design (emulator perf numbers remain meaningless); and **rotation / backgrounding
   are still unverified on the GPU path** — Android destroys the surface on every
   rotation, and the lifecycle test to date ran on the CPU fallback. That is now the
   cheapest unknown left and should be run before any further mobile work.
3. **Full-scene perf is unmeasured.** The ~60ms figure predates GPU-by-default. Every
   throughput decision in Stages 3–4 is gated on Stage 0's numbers.
4. **gg's GPU image path rebuilds a bind group + uniform buffer + draw call per sprite
   per frame** — the predicted bottleneck, worse than gophics's op boxing.
5. **A full-window Canvas defeats damage tracking twice over** (identity `PushTransform`
   + `hasLayers` escalation). Cheap fix, large payoff, especially solitaire on mobile.
6. **Rotated sprites vanish on the direct-surface path — this prediction is now
   CONFIRMED on device (2026-08-03).** In the gpucheck scene the plain and tinted
   sprites draw and the **rotated one is simply absent**, while a rotated *path* in
   the same frame draws correctly — so it is specific to the image tier, not to
   transforms. Mechanism is exactly as predicted: `third_party/gg/context_image.go:374`
   bails out of the tier-3 GPU image path with `return false` for any non-axis-aligned
   quad, and the fallback is the `DrawImage` bitmap path that `paint/paint.go:702`
   calls "fatal on the direct-surface path" because it forces a mid-frame accelerator
   flush that drops the queued shapes. So a single rotated sprite can take *other*
   draws down with it, not just itself. The fix is to support rotated quads in the
   tier-3 path (the shader already takes a destination quad; it is the `isAxisAligned`
   guard and the axis-aligned `x,y,w,h` call signature at `context_image.go:395` that
   need to become a full quad) rather than to improve the fallback.
7. **Texture cache cliffs at 64 (gg) and 256 (Painter)** — a 52-card deck is already at
   80% of the first.
8. **Single-pointer input** blocks two-thumb touch. Auto-run is the designed-around
   answer; multi-touch tier 1 is the real fix.
9. **Canvas content is invisible to the a11y bridge** — the acknowledged cost of a
   Canvas-drawn solitaire, freshly relevant since TalkBack/VoiceOver just landed.
10. **Art for the scroller.** Mitigated structurally by the `Skin` interface, which makes
   the demo shippable before any sprite exists.
11. **Scope creep in `game`** — the package doc gate above.

## Verification

- **Per stage, `go test ./...` stays green** — ~35 files use `app.NewHeadless`, and the
  damage/`op` refactors are exactly where regressions would hide.
- **`app/gpu_equiv_test.go` gains a `Canvas` case** — every new `Canvas` method must
  render identically on CPU and GPU. This is the guard against the silent
  "renders on GPU, vanishes on CPU" class.
- **`GOPHICS_PACING=1 gophics run -p desktop ./examples/scroller`** and the same on
  `-p web`, against the Stage-0 budget table.
- **Real devices, not emulators** — the 89ms figure came from SwiftShader and is not
  evidence about phone hardware.
- **`GOPHICS_RENDERER=cpu` must keep working** for both examples at every stage; it is
  the mobile and fallback path, and it is what keeps the tests deterministic.
- **Solitaire: zero raster while idle**, verified via `Core.FrameStats` and by
  confirming frames are skipped, not merely cheap.
- **Deterministic replay** — same seed + same input trace → identical final state *and*
  identical image bytes, in both examples. Enabled by the fixed timestep and the seeded
  `game.Rand`; this is the strongest test lever the design produces.
- **Test harness to copy:** `examples/todo/render_test.go` and
  `examples/gallery/render_test.go` already establish the `stateHook` + settle +
  pixel-probe pattern; both games' tests should follow it rather than invent one.
- **`TestNoTunneling`** in `game/collide.go`'s tests — the named invariant that keeps
  the platformer's worst bug class from shipping.
