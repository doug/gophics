# Platform capabilities: clean Go interfaces, generated wiring, zero-CGo native code

gophics is a pure-Go, zero-CGo framework, but real apps need per-platform native
integration — file dialogs, share sheets, notifications, keychains, and so on.
This document describes the pattern that lets us keep **Go-only interfaces** as
the single source of truth while still shipping native integration, without CGo,
and without hand-maintaining boilerplate across three packages.

## The three layers

A capability is defined once, in pure Go, and flows through three layers:

```
shell/<cap>.go            (1) Contract   — the interface + the <X>Window opt-in
        │                                   (hand-written, pure Go, the truth)
        ▼
widget.Capabilities       (2) Dispatch   — Owner field + Ctx.<Cap>() accessor
app.wireCapabilities                       + app-runner wiring
        │                                   (100% GENERATED from layer 1)
        ▼
shell/<platform>/…        (3) Impl        — one build-tagged file per platform
                                            that returns the capability from its
                                            Window (hand-written, tiny, on shared
                                            FFI/bridge helpers)
```

Only layers 1 and 3 are hand-written. Layer 2 — the part that previously meant
editing `widget.Owner`, the `Ctx` accessors, and the runner wiring in lockstep
every single time — is generated.

### Layer 1 — the contract (`shell/<cap>.go`)

Two declarations, following a strict convention:

```go
// shell/filepicker.go
type FilePickerWindow interface{ FilePicker() FilePicker } // the opt-in marker
type FilePicker interface {                                // the capability
    Open(OpenOptions, func([]PickedFile, error))
    Save(SaveOptions, []byte, func(error))
}
```

The convention the generator keys on: an interface named **`<Cap>Window`** (other
than the base `shell.Window`) whose methods are all **zero-argument, single-
interface-result getters**. `MediaWindow` may return more than one capability
(`Camera() Camera`, `Audio() Audio`); most return one.

Files are carried as **bytes, not paths** — the web File API and mobile content
URIs expose no stable filesystem path, so bytes are the one portable currency.

### Layer 2 — generated dispatch

`internal/capgen` parses `shell/*.go` and emits:

- **`widget/capabilities_gen.go`** — a `Capabilities` struct (embedded in
  `Owner`, so its fields are promoted) plus a `Ctx.<Cap>()` accessor per
  capability.
- **`app/capabilities_gen.go`** — `wireCapabilities(owner, window)`: one
  `if x, ok := w.(shell.<X>Window); ok { owner.<Cap> = … }` per window.
- **`shell/posted_gen.go`** — `Posted<Cap>` adapters that deliver every callback
  through a `post func(func())`. The wiring wraps each callback-carrying
  capability with `Owner.Post`, so the documented **"all callbacks fire on the
  UI goroutine"** contract is enforced *centrally* — platform implementations
  may invoke callbacks from any goroutine (a JS promise resolution, a native
  completion handler) without each one hand-marshaling. The wrappers are
  recursive: an interface handed out through a callback or result (e.g. the
  `Recorder` from `Audio.Record`) is wrapped too, so its own callbacks also
  post. Interfaces with no callbacks (SecureStorage, Playback) pass through
  unwrapped, and synchronous methods (`Recorder.Level`) forward directly.

`widget.Owner` embeds the generated struct:

```go
type Owner struct {
    …core fields…
    Capabilities // generated; do not add capability fields by hand
    …
}
```

Regenerate with `go generate ./...` (directive in `widget/gen.go`). The generator
locates the module root itself, so the working directory doesn't matter, and its
output is deterministic — re-running produces no diff.

### Layer 3 — the native implementation

Each platform's shell `Window` implements the `<X>Window` for the capabilities it
supports; `wireCapabilities` publishes them. A capability the platform can't
provide is simply never returned, so `ctx.<Cap>()` is `nil` there and callers
hide the affordance.

```go
// shell/web/filepicker_web.go  (//go:build js && wasm)
func (w *window) FilePicker() shell.FilePicker { return &webFilePicker{doc: w.doc} }
```

## Keeping it zero-CGo

Native integration never needs CGo. There are three native-call paths, already
used elsewhere in the tree:

| Platform | Mechanism | Precedent in-tree |
|---|---|---|
| **macOS / Linux** | An internal `ffi` package: `dlopen`/`dlsym` + asm trampolines. Call the Objective-C runtime (`objc_msgSend`) → AppKit (`NSOpenPanel`, `NSSharingServicePicker`, `UNUserNotificationCenter`), or dlsym'd C entry points; on Linux, D-Bus portals via a pure-Go D-Bus client. | `internal/gfx/wgpu/hal/metal/objc.go` loads `libobjc.A.dylib` and dispatches `objc_msgSend` |
| **Windows** | `windows.NewLazyDLL` + `syscall.SyscallN` over COM vtables. | `internal/gfx/gogpu/internal/platform/dialog_windows.go` drives `IFileDialog` |
| **Android / iOS** | The gomobile `Bridge` + host-drain: Go enqueues a request, the Kotlin/Swift host drains it (`TakeX()` poll) and does the native work, results flow back through the bound interface. The native code lives in Kotlin/Swift — **Go stays pure**. | the `TakeHaptic` / `TakeOpenedURL` pattern in `shell/mobile` |

So: desktop dlsym's C symbols directly (no cgo); mobile delegates native work to
Kotlin/Swift over the bridge (no cgo in Go). The leaf native call is the only
hand-written platform code, and it sits on shared helpers, e.g.:

```go
// objc helper over the ffi trampolines (sketch)
panel := objc.Class("NSOpenPanel").Send("openPanel")
if objc.SendInt(panel, "runModal") == 1 { … }
```

These helper packages (`objc`, a COM `vtbl` helper, the mobile `bridge` base) are
shared infrastructure — the per-capability native body is typically ~10 lines.

## Adding a capability, end to end

1. **Write the interface** in `shell/<cap>.go` — the `<Cap>Window` marker + the
   capability (pure Go).
2. **`go generate ./...`** — `Capabilities`, the `Ctx.<Cap>()` accessor, and the
   `wireCapabilities` clause appear. Nothing hand-edited in `widget`/`app`.
3. **Implement per platform** — a build-tagged file per shell that returns the
   capability from its `Window`:
   - **web**: JS via `syscall/js` (see `shell/web/*_web.go`)
   - **desktop**: `objc`/COM helpers over the `ffi`/`syscall` layer
   - **mobile**: enqueue on the `Bridge`; add the Kotlin/Swift host method
4. **Use it**: `if x := ctx.<Cap>(); x != nil { … }`.

## What's generated vs hand-written

| | Source | Regenerated |
|---|---|---|
| Capability interface (`shell/<cap>.go`) | hand | no |
| `widget.Capabilities` + `Ctx` accessors | **generated** | yes |
| `app.wireCapabilities` | **generated** | yes |
| Per-platform impl (`shell/<platform>/…`) | hand | no |
| FFI/bridge helpers (`objc`, COM, mobile bridge) | hand (shared) | no |

The interface is the single source of truth; codegen owns all the wiring; the
only hand-written platform code is the leaf native call on a shared helper.

## Future extensions

The generator can also emit, when a capability lacks one, a **build-tagged stub**
per platform (returning nil with a `// TODO(<platform>)` marker) and the mobile
**bridge scaffold** (the Go queue + `TakeX()` drain, plus a Kotlin/Swift host
stub) — so a new capability starts life buildable on every target and you fill in
the native bodies incrementally. A `capabilities_test.go` can assert every
capability is implemented on at least one platform, so nothing ships as a silent
no-op everywhere.

---

## Capability status (2026-08-10)

Every capability follows the same three-layer pattern; "✅" = implemented and
built (web = browser-verified or -verifiable — see the live-verification note
below; desktop/mobile = compile-verified on the targets noted, not device-run),
"~" = partial/best-effort, "stub" = interface + compile-checked `TODO(platform)`
(no native code written that can't be verified here), "—" = not applicable.

| Capability | Web | Desktop | Mobile | Notes |
|---|---|---|---|---|
| Clipboard, OpenURL, SafeInsets, Input, DarkMode | ✅ | ✅ | ✅ | pre-existing core |
| Camera, Audio, Haptic | ✅ | stub | ✅ | mobile via bridge |
| **FilePicker** | ✅ | ✅ macOS / stub | stub | macOS = real NSOpenPanel/NSSavePanel via zero-CGo objc; linux/windows TODO |
| Share, Notifier, SecureStorage, Permissions | ✅ | stub | stub | native share sheet/notifications/keychain TODO |
| **Socket** (WebSocket) | ✅ | ✅ | ✅ | pure-Go RFC 6455 client (`!js`), tested |
| **Lifecycle** (fg/bg) | ✅ | ✅ | stub | desktop via focus routing |
| **Links** (deep links) | ✅ | ~ | stub | desktop = os.Args; OnLink no-op |
| **WindowControl** (title/fullscreen) | ✅ | ✅ | — | rides gogpu; tray/native-menus deliberately excluded |
| **Connectivity** | ✅ | ~ | — | desktop best-effort (online=true) |
| **Battery** | ✅ | stub | stub | |
| **Gamepad** | ✅ | stub | stub | poll-style; empty without hardware |
| **Geolocation** | ✅ | stub | stub | |
| **TextInput** (IME) | ✅ | — | stub | hidden-input keyboard; mobile composition routing TODO |
| **Accessibility** | ~ | stub | stub | Announce (aria-live) ✅; SetTree (AT-tree) TODO |
| **WebView** | ✅ | stub | stub | iframe overlay; native subview TODO |

**Web live-verification (2026-08-10).** The `examples/capabilities` inspector now
wires every capability, so the web implementations were exercised in a real
browser (Chrome, Apple GPU, WebGPU active) rather than only compiled. Verified
working end-to-end:

- **Connectivity** — reads `online`; **Battery** — `100% (charging)`;
  **Lifecycle** — reports `background` and updates on tab switch; **Links** —
  surfaces the launch URL; **Gamepad** — polls (`no controller connected`).
- **SecureStorage** — write round-trips to localStorage (`saved …`).
- **Clipboard** — `writeText` invoked with the phrase (spied) + on-screen confirm.
- **TextInput (IME)** — `Show()` creates and focuses the hidden `<input>`; an
  `input` event delivers committed text to `OnText`, updating the app (`typed: …`).
- **Accessibility** — `Announce` creates an `aria-live="polite"` region with the text.
- **WebView** — an `<iframe src="https://example.com">` overlay renders over the canvas.
- **WindowControl** — `SetTitle` changes `document.title`. (`requestFullscreen` is
  issued but Chrome refuses it under a synthetic click; it works on a real user
  gesture — an automation limitation, not a code defect.)

Confirmed **wired and reachable but not scriptable here** (invoking them opens a
native OS dialog or permission prompt that blocks headless browser automation —
each needs a human click to complete): **FilePicker, Share, Notifier, Geolocation**.
The backing web APIs (`<input type=file>`, `navigator.share`/`canShare`,
`Notification`, `navigator.geolocation`) are all present in the browser.

**macOS FilePicker (2026-08-12) — device-verified.** The macOS file panels are
implemented for real, on `internal/objc` (a small dlopen + `objc_msgSend` bridge
over the pure-Go goffi FFI — still zero CGo), and verified by running a live app:
a real `NSOpenPanel` becomes modal and its cancel path reports `files=0, err=nil`
per the contract. The objc bridge itself has unit tests (NSString round-trip
including unicode, `-length`, `-isEqualToString:`, NSArray bridging, nil-messaging
safety, framework loading).

Getting there uncovered a framework-wide constraint worth knowing: gogpu runs
window/input events and `OnUpdate` on the **main thread** but `OnDraw` on a
**render thread**, and gophics drives Build/layout/tickers from `OnDraw` — so
framework code is *not* on the main thread by default, while AppKit aborts the
process (not a catchable error) if an NSWindow is constructed anywhere else. The
desktop shell now has a main-thread dispatch queue (`shell/desktop/mainthread.go`,
drained in `OnUpdate`) that every main-thread-bound capability can use. This is the
prerequisite for the remaining AppKit capabilities (share sheet, notifications).

**Desktop / mobile / native.** The other desktop implementations compile on
macOS/linux/windows but can't be run-verified from this environment. The native
leaf calls (macOS objc, Linux portals/GTK, Windows COM, Android Kotlin, iOS Swift)
are marked `TODO(platform)` — not written speculatively — because they can't be
compiled or run on the current host; each is a small fill-in on the shared
FFI/bridge helpers when a device is available.

**Not yet declared** (roadmap, need the WebView-class subview decision or deep
native work): Video playback, Biometrics (Face/Touch ID), Sensors, Push
notifications, System tray, native menu bar, global hotkeys, and the full
accessibility AT-tree (Accessibility.SetTree) + mobile IME composition.
