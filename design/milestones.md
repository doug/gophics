# Milestones

The next tranche of work, as milestones with exit criteria. PLAN §7 lists what
remains overall and in what order of value; this is the near-term slice of it,
broken down far enough to start.

Each milestone states an exit criterion that can be checked rather than
judged. "Done" means the criterion holds, not that the work felt finished.

Ordering note: **M1 and M2 come first because they are verification, not
construction.** Both are cheap, and either could remove work from this list
rather than add to it — M1 tells us whether the guards we already rely on are
running at all, and M2 may find that a bug listed as a blocker is already
fixed. Building on top of unverified guards is how the 18 MB binary reached
main past a check written to stop exactly that.

---

## M1 — Prove the build gates actually run ✅

**Goal.** Know that CI runs, what it reports, and that its guards work.

**Why now.** `.github/workflows/ci.yml` has a "no oversized tracked files"
gate that rejects any tracked file over 1 MB outside testdata and asset
directories. An 18 MB compiled binary reached `main` anyway. Either the
workflow is not running, or it failed and nobody looked. Until that is
settled, every other guard in CI — the lint pass, the race suite, the
generate-freshness check, the new embed-drift check — is of unknown value.

**Established so far (2026-08-16).**

- The gate's logic is sound. Staging a 3 MB file locally and running the
  workflow's own shell makes it fire; the 18 MB binary matched none of its
  exceptions (`*/testdata/*`, `*/assets/*`, `*.png`, `*.jpg`, `*.wasm.br`),
  so it would have been caught. The guard is not the problem.
- `ci.yml` triggers on `push: branches: [main]` with no path filter, so it
  should run on every push, including the one that carried the binary.
- Therefore the workflow did not run, or failed before reaching the gate.
  Which one cannot be settled from here: the repo is private, so the API
  needs a token, and there is none.
- `gh` has no OAuth token — `gh auth token` reports "no oauth token found".
  SSH auth covers git push and pull (verified: `git ls-remote` works), but
  the REST and GraphQL APIs are a separate credential. `gh auth login` set up
  the SSH key without storing one.

**Answered (2026-08-16), from the Actions tab.** CI runs on every push and has
been **failing**, which is the other branch of the original either/or — not
"the workflow never ran" as the evidence above suggested. Run #68 on 30ee4dd:
`lint` and `test (framework)` both red, `build` green on all four targets.

Nothing blocks a push to `main`, so a red run stops nothing. **That is the
actual reason the 18 MB binary landed**: the gate did fire, and the push
succeeded anyway because no branch protection requires the check to pass. The
guard was never the problem, and neither was Actions being off.

- [x] Confirmed the workflows run — on every push, no path filter.
- [x] `lint` failure found, reproduced locally and fixed (9e4e30b): the
      capability generator's output was stale, because `Accessibility` gained
      `SetTree` without a regenerate. Not cosmetic — see that commit; the gate
      caught a real thread-safety bug in the a11y bridges.
- [x] `test (framework)` failure found and fixed (e4b5d3d). It was not
      reproducible locally for a good reason: `TestSystemFontFallback` asserted
      that a CJK glyph resolves to a real system font, and the runner has no
      CJK font installed, so fontscan returned an arbitrary face whose glyph
      for 你 is 0. Routing and coverage are now asserted separately — routing
      holds anywhere, coverage skips with the package name to install — and CI
      installs `fonts-noto-cjk` and sets `GOPHICS_REQUIRE_SYSTEM_CJK=1` so a
      missing glyph is a failure there rather than a silent skip.
- [x] A red run now stops something: `.githooks/pre-push` runs
      `scripts/gates.sh` — the same script CI's lint job runs, so the two
      cannot drift — and refuses the push if any gate fails (~2s). Install per
      clone with `git config core.hooksPath .githooks`; `--no-verify` bypasses.
      Verified by breaking each gate in turn, including a real push that the
      hook rejected.
- [x] **Branch protection: declined (2026-08-16).** It is the stronger guard —
      a hook is per-clone, opt-in, bypassable, and cannot run the test suite —
      but this is a single-author private repo where requiring a four-minute
      check on every push costs more than it saves. The pre-push hook is the
      accepted answer. Revisit only if the repo gains a second contributor;
      until then a red run is a signal to read, not a gate.

**Exit — met (2026-08-16).** Run #69 is green on `main`, the first green run
this repo has had. The oversized-file gate was shown to fail twice over: once
against a deliberately staged 3 MB file, and once for real, when the pre-push
hook rejected a push carrying one.

**What this milestone was actually about.** The premise — "either the workflow
is not running, or it failed and nobody looked" — turned out to be the second,
and the local evidence pointed hard at the first: the gate worked when tested,
and CI triggers on every push, so it looked as though nothing could have run.
That reasoning was wrong in a way worth remembering. A guard that runs, fails,
and blocks nothing is indistinguishable from a guard that never ran, unless you
go and look. Both failures had been red for weeks and both were real: a stale
generator hiding a thread-safety bug in the a11y bridges, and a test that
could only ever fail on the machine it was written to protect.

---

## M2 — Retest GPU text on Android ✅

**Goal.** Establish whether the Vulkan text bug still exists.

**Why now.** PLAN §6.4 records "GPU text draws as solid blocks (glyph
positions correct, coverage wrong)" as Vulkan-only and blocking any
text-bearing Android UI. But after the surface-format fix, every web demo —
including text-heavy ones — rendered correctly on a Pixel 10 Pro over WebGPU,
and the native HN app rendered crisp text too. Same subsystem, same class of
bug: an attachment format that did not match what the pipeline was compiled
for. It is plausible this is already fixed and the plan is out of date.

- [x] Built and ran tally natively on the Pixel 10 Pro — dense text: tab bar,
      a 111,717.47 USD figure, chart axis labels down to 10 px, a category
      table.
- [x] **Glyphs render correctly on Vulkan.** Crisp at every size, no solid
      blocks. The bug PLAN §6.4 calls blocking is fixed, almost certainly by
      the surface-format work: same class of fault, an attachment format that
      did not match what the pipeline was compiled for.
- [x] Accessibility checked on the same run: 36 nodes with real labels, roles
      and bounds, buttons flagged clickable, charts described
      ("Area chart. 31 points…"), and the tree rebuilds live — switching to
      Balances republished 145 nodes.
- [x] Rotated sprites draw too. The `gophics_verify` bring-up scene shows its
      plain / tinted / rotated trio in full, plus gradients, path fill, nested
      opacity and backdrop blur. Fixed by 2a5f24b, which routes the
      non-axis-aligned case to the textured-quad path rather than to a CPU
      fallback the direct-surface path discards.
- [x] PLAN §6.4 rewritten: both bugs struck, with the retest command recorded
      so the next person does not have to rediscover it. The same stale claim
      in `design/games-plan.md` (§417 and finding 6) is corrected — the
      original prediction-then-confirmation is kept, since it is a good worked
      example, and marked fixed.

**Exit — met (2026-08-16).** PLAN §6.4 matches reality; each claim has device
evidence from a Pixel 10 Pro on Vulkan, direct surface.

---

## M3 — Ship the 16 KB alignment fix everywhere it is needed ✅

**Goal.** Any documented way of building an Android APK produces one that
runs without the compatibility dialog.

**Why now.** The `-Wl,-z,max-page-size=16384` flag lives only in
`internal/cli/mobile.go`, so it applies when building through the `gophics`
CLI. `examples/tally/package/android.sh` calls `gomobile bind` directly and
misses it, as does anyone following the README. On a Pixel 10 the resulting
APK shows Android's "not 16 KB compatible" dialog on launch. Verified this
session: adding the flag and reinstalling clears it.

Small, and it removes a first-run failure on current hardware.

- [x] Added the flag to `examples/tally/package/android.sh`. A test now reads
      both it and `gomobileBind`, so the two cannot drift apart again; it was
      checked by deleting the flag and watching the test fail.
- [x] iOS is not affected — `gomobileBind` only passes the flag for Android, so
      the script and the CLI already agree there.
- [x] Two bugs found on the way, both fixed and both invisible without a device:
      the script aborted under `set -u` because `$TASK…` swallowed the
      following non-ASCII byte as part of the variable name, and tally's JNI
      exports still named hn's package, so the app died with
      `UnsatisfiedLinkError` on its first `SurfaceView` callback.

**Exit — met (2026-08-16).** An APK built by `package/android.sh` installs and
launches on the Pixel 10 Pro with no compatibility dialog and no crash, and
both libs report `0x4000`:

    libgojni.so:           LOAD align = 0x4000
    libgophics_surface.so: LOAD align = 0x4000

---

## M4 — Fill in the stubbed platform capabilities

**Goal.** Battery, gamepad and geolocation return real data, or are honestly
absent, on every platform.

**Why now.** The capability layer is a headline claim in
`design/positioning.md` — "every platform service is a plain `ctx.<Cap>()` that
degrades cleanly" — and three of them degrading to silence undercuts it. Each
is small and independent, so this milestone lands in pieces.

Two corrections to this entry as first written: there are six `TODO(platform)`
markers, not nine, and **web already implements all three** — `battery_web.go`,
`gamepad_web.go` and `geolocation_web.go` have been there the whole time. The
gap was only ever desktop and mobile.

- [x] **Battery on every desktop platform.** macOS through IOKit's power
      sources (three FFI symbols; the rest read via toll-free bridging, since
      the CFDictionary it returns *is* an NSDictionary), Linux from
      `/sys/class/power_supply` (no upower, no session bus), Windows from
      `GetSystemPowerStatus`. A machine with no battery returns nil.
- [x] **Gamepad on macOS and Linux.** macOS through GameController, which
      already maps each vendor's pad to a known layout; Linux through evdev,
      chosen over joydev because only evdev reports the axis range that makes
      full deflection read as 1.0. Y is negated on macOS so `Axes[1]` points
      the same way it does on the web.
- [x] **Gamepad on Windows (XInput).** Desktop gamepad is now complete on all
      three platforms. XInput rather than raw HID or DirectInput, for the same
      reason macOS uses GameController: Windows already normalises any
      Xbox-compatible pad to one layout, and the per-vendor mapping table is the
      part that ages badly. Verified on Windows 11 ARM64 — button order,
      analog triggers, axis clamping (XInput's range is asymmetric, so full-left
      would read past -1), Y negation to match the other shells, and that
      XInputGetState actually resolves rather than every test passing against a
      nil proc.
- [ ] Geolocation on desktop: CoreLocation needs an authorization dance and a
      run loop, geoclue is D-Bus, Windows is COM. Deferred deliberately —
      three heavy bindings for something desktop apps rarely ask for.
- [ ] Mobile: battery, gamepad and geolocation all still nil. Each needs host
      work (Kotlin `BatteryManager`, Swift `CoreLocation`) on the far side of
      the bridge, not just Go.

**Testing note.** Neither a battery nor a controller was available on the
machine this was written on — a Mac Studio with nothing plugged in. So the
paths that only run when hardware *is* present were split out and tested
against fabricated input: `readDescription` against an NSDictionary shaped like
a power source, and the evdev decoder against synthetic `input_event` bytes.
The ioctl request numbers are checked against known constants, and on a laptop
the macOS read also cross-checks against `pmset`.

**Exit.** Battery and gamepad both meet it: `examples/capabilities` shows live
values on web and on every desktop, and returns nil rather than a silent zero
elsewhere. Geolocation remains web-only, and mobile remains unimplemented.

---

## M5 — Accessibility on Linux and Windows (Linux ✅, Windows all but bounds)

**Goal.** A screen reader can explore and operate a gophics app on every
supported desktop.

**Why now.** The last gap. Web, macOS, iOS and Android all publish the tree;
Linux (AT-SPI) and Windows (UI Automation) do not, so `ctx.Accessibility()`
returns nil there. The design work is done — one flat node type, a push
interface for platforms that want handing a tree and a pull interface for
those that query on their own schedule — so this is per-platform binding
rather than architecture.

The largest item here. Both spikes are now done — see below — and neither
platform is blocked on feasibility.

**Both spikes are done, and both are favourable (2026-08-16).**

- [x] **AT-SPI over D-Bus from pure Go: proven, not assumed.** A test now
      finds `org.a11y.Bus` on the session bus, reads the accessibility bus
      address from it, dials that second bus and completes SASL EXTERNAL auth
      plus Hello — verified against a real `at-spi2-core` in a container.
      Nothing new was needed to get there: `dbus_linux.go` already had the
      transport, auth and the full type marshaller, written for the file
      portal. See `atspi_linux.go`.

      What remains is protocol surface, not feasibility. Everything today is
      client-shaped — send a call, await the reply — and AT-SPI inverts that:
      the app is a server, exporting an object per node and answering
      `GetChildAtIndex`, `GetRole`, `GetExtents` on the screen reader's
      schedule, the same pull model AppKit uses. Needed on top: an inbound
      dispatch loop, METHOD_RETURN/ERROR replies (the encoder already takes a
      message type, so this is wiring), signal emission for focus and state,
      `org.freedesktop.DBus.Introspectable`, and the Accessible, Component and
      Action interfaces over `A11yNode`.

- [x] **UIA in pure Go: tractable, and the technique already ships here.**
      `internal/gfx/naga/internal/dxcvalidator/dxcvalidator_windows.go`
      implements a COM object in Go — a struct whose first qword is a vtable
      pointer, the vtable filled with `syscall.NewCallback` thunks — and
      `dxil.dll` calls into it through ordinary COM dispatch. A UIA provider is
      the same shape, and `platform_windows.go` already owns a `wndProc`, which
      is where `WM_GETOBJECT` and `UiaReturnRawElementProvider` hook in.

      Harder than that blob in three specific ways, all bounded: it fakes
      refcounting and answers `QueryInterface` with E_NOINTERFACE, whereas UIA
      really does query for `IRawElementProviderFragment`,
      `FragmentRoot` and pattern interfaces, and really does hold references
      across calls; `GetPropertyValue` and `GetRuntimeId` need VARIANT and
      SAFEARRAY marshalling; and the callback budget is per *vtable*, not per
      node, so a large tree costs nothing extra.

- [x] **AT-SPI implemented and verified end to end.** A gophics tree is
      published on the accessibility bus and read back through the real client
      stack — `pyatspi`, the same libraries a screen reader uses. Roles,
      labels, extents, states and actions all survive the trip, and `DoAction`
      over the bus reaches the Go callback:

          NODE|0|application|gophics-atspi-test|-||active,enabled,…
          NODE|1|frame|Root|0,0,400,300||enabled,sensitive,showing,visible
          NODE|2|button|Send|10,20,80,30|click|enabled,focusable,…
          DIDACTION
          NODE|2|check box|Agree|10,90,120,24|click|checkable,checked,…

      Four files: `dbus_server_linux.go` turns the client into a peer
      (METHOD_RETURN, ERROR, and the object-path/struct/dict writers AT-SPI
      needs); `atspi_tree_linux.go` maps ARIA roles and node flags onto AT-SPI's
      enums; `atspi_server_linux.go` is the object server —
      Accessible, Component, Action, Application, Properties, Introspectable —
      and `atspi_window_linux.go` hangs it off both the X11 and Wayland windows,
      which share it because AT-SPI knows nothing about the display server.

      The role and state numbers were read out of pyatspi rather than written
      from memory. Several are unobvious — CHECKABLE is 41, so it lands in the
      second word of the state bitset — and a wrong one does not fail loudly, it
      makes a button announce as something else.

- [x] **Change events, verified at a real client.** Serving on demand was only
      half of it: a screen reader reads a node once and then waits to be told.
      All five arrive —

          EVENT|object:property-change:accessible-name|0||
          EVENT|object:state-changed:checked|1|Agree|
          EVENT|object:children-changed:add|2|Root|
          EVENT|object:announcement|2|gophics-atspi-events|5 results
          EVENT|object:children-changed:remove|2|Root|

      Events come from diffing successive trees, not from republishing. gophics
      rebuilds whenever the widget tree changes, which for an animating UI is
      every frame; broadcasting an unchanged tree at 60Hz would drown the bus
      and make a reader unusable. Tests pin the quiet cases too: an identical
      republish emits nothing, and a node that merely *moves* emits nothing,
      since bounds are not state.

      `AnnounceA11y` works now as well — announcements are delivered as events,
      so they could not exist before this.
- [x] **Confirmed with Orca**, which found a bug pyatspi could not. Orca listed
      the application and the frame, then refused to enter it:

          [frame: 'Catalogue'] ... lacks active state
          Unable to find active window from [application: 'Gophics …']

      A top-level frame must carry ACTIVE, and must announce itself with a
      `window:activate` event. pyatspi never showed this because a client handed
      a tree walks it regardless; the active-window rule is the screen reader's.
      With both added, Orca speaks:

          SPEECH OUTPUT: 'Save button.'
          SPEECH OUTPUT: 'Remember me check box checked.'

      driven by our `object:state-changed:focused` events, with role and checked
      state announced correctly. Orca cannot run in CI, so `TestFrameIsActive`
      stands in for it.

      Worth remembering as a method note: two clients disagreed, and the
      stricter one was right. A protocol-level client proves the wire format; it
      does not prove the conventions layered on top.
- [ ] Run a real gophics window under Xvfb, rather than a tree published
      directly by a test.

### Windows (UI Automation)

**A UIA provider works, verified by a real client against a running app.**
`uia_windows.go` and friends build COM objects in pure Go —
`syscall.NewCallback` vtables, four interfaces per element laid out like C++
multiple inheritance, real refcounting and QueryInterface. A UIA client walking
a live gophics window on Windows 11 ARM64 sees:

    WINDOW name='counter' type=window
    NODE|0|group||enabled=True|focusable=False
    NODE|1|text|TAPS|enabled=True|focusable=False
    NODE|1|text|3|enabled=True|focusable=False
    NODE|1|button|Increment|InvokePatternIdentifiers.Pattern|enabled=True|focusable=True

Structure, names, control types, enabled and focusable states, and
InvokePattern on the button alone. The app side shows the handshake:
`WM_GETOBJECT lParam=-25` → `returning provider`.

- [ ] **BoundingRectangle reaches the client as zero.** Still open after a
      systematic hunt; what follows is so the next attempt does not repeat it.

      Established on device:

      - `get_BoundingRectangle` **is** called, for every node, with correct
        screen coordinates — `bounds id=3 node=(111,129 97x32) screen=(223,264)`
        for the Increment button.
      - Returning a hardcoded `100,100 200x50` from that method changes nothing
        at the client, so this is delivery, not arithmetic.
      - Property 30001 is never requested; UIA uses the fragment method.
      - The HWND is healthy: `GetWindowRect` gives `156,156 336x259` and the
        window is visible. The host provider is handed over successfully
        (`UiaHostProviderFromHwnd` → `hr=0x0`, non-null).
      - Writing through an out-parameter works in general — `GetPropertyValue`
        fills a VARIANT the same way and Name, ControlType and the rest all
        arrive intact.

      Ruled out: implementing 30001 as a property; giving the fragment root the
      window's rect; `NativeWindowHandle` (worse — answering it with our own
      HWND makes UIA re-enter the host provider and the tree becomes an endless
      chain of window elements); the not-supported sentinel.

      Not yet ruled out, and where to start next: **the client**. The native
      windows it reads correctly are HWND-based, where UIA derives geometry
      itself, so nothing has yet proved this client can surface *provider*
      -supplied rects at all. A native UIA client (IUIAutomation via COM) or
      Accessibility Insights would settle it. Also worth checking whether the
      fragment root failing `ElementProviderFromPoint` with E_NOTIMPL makes UIA
      distrust the fragment's geometry.

      Navigation, properties and activation do not depend on this; a screen
      reader's highlight does.
- [x] Returning UIA's reserved not-supported value for properties we do not
      answer. `VT_EMPTY` is not "no opinion" to UIA — it is "the value is
      empty", and it overrides whatever the host provider would have said. This
      did not fix the rectangle, but it is correct provider behaviour and was
      wrong before.
- [ ] Confirm with Narrator itself, the counterpart to the Orca pass.
- [ ] `AnnounceA11y` — UIA delivers announcements as a NotificationEvent, which
      needs `UiaRaiseNotificationEvent`. Currently a documented no-op.
- [ ] Point hit-testing. `ElementProviderFromPoint` takes two doubles and
      `syscall.NewCallback` refuses float parameters outright, so it answers
      E_NOTIMPL. Keyboard and focus navigation are unaffected; mouse and touch
      exploration are not available. The provider's own `hitTest` is written
      and tested, waiting on a route that can carry the coordinates.

**The validation loop is containerisable** — checked 2026-08-16, and worth
knowing before starting, because "needs a Linux desktop with a screen reader"
would otherwise make this look untestable from a Mac. In a plain
`debian:trixie` container, `at-spi2-core`, `python3-pyatspi`, Xvfb and Orca
itself (48.1) all install and run headless; the registry answers and
`pyatspi.Registry.getDesktop(0)` enumerates.

Prefer pyatspi to Orca for the work: it scripts the client side, so tree
shape, roles, labels, bounds and actions become assertions rather than
something read off a screen. `gdbus`/`busctl` sit below that for when one
reply is malformed. Keep Orca for the end-to-end pass — `--debug-file` records
what it perceived, so no speech output is needed.

Untested: running a gophics window in that container. Xvfb covers the display
and the `nogpu` pure-CPU tag exists (CI builds it), so an X11 window without
Vulkan looks plausible; lavapipe is the fallback. Confirm before relying on it.
- [x] macOS announcements now work. AppKit routes live-region speech through
      NSAccessibilityPostNotificationWithUserInfo — a plain C function, so it
      needs an FFI call interface rather than the objc_msgSend path everything
      else in that file uses. The subtlety is that its notification name and
      both userInfo keys are *data* symbols: dlsym yields the address of the
      pointer, so each needs one dereference. Getting that wrong gives a
      plausible non-nil value that is not a string, and AppKit silently ignores
      the post — so a test asserts the resolved globals have non-zero NSString
      length, not merely that they are non-nil.
- [ ] Validate iOS on-device with VoiceOver — the one platform verified only
      in the simulator.

**Exit — Linux met (2026-08-17).** Orca reads a gophics tree and announces
each widget with its role and state, and activation over the bus reaches the
Go callback. What remains for Linux is cosmetic rather than structural: the
tree in these tests is published directly rather than by a running window under
Xvfb. Windows and Narrator are untouched, though the UIA spike found the COM
technique already proven in-repo.

---

## M6 — Lifecycle on mobile (Go + Android done; device check pending)

**Goal.** An app knows when it has been backgrounded, in time to save.

**Why now.** `ctx.Lifecycle()` is nil on Android and iOS — `shell/mobile/
lifecycle.go` returns nil with a TODO naming the exact host callbacks. That is
the single most useful thing missing for mobile apps, because "persist state
before the OS kills me" is what most people mean when they ask for background
support, and it needs no scheduler at all.

Small: an atomic and a callback list on the Go side, four one-line overrides per
platform on the host side. Design in `design/mobile-background.md`.

- [x] `Bridge.SetAppState` and a real `Bridge.Lifecycle()`, replacing the nil.
      Repeated states are ignored, because Android sends onPause both on the way
      out and on the way back, and an app persisting on every callback would
      write several times for one visit to the background. Out-of-ladder values
      are ignored rather than clamped: clamping to background would pause an app
      that is running fine.
- [x] Android drives it from `onResume`/`onPause`/`onStop`, and `Focused` keeps
      its old meaning — it is window focus and fires for a dialog over the app,
      which is why the TODO said not to reuse it.
- [x] Race-tested: SetAppState arrives on the host UI thread while the widget
      tree reads State() on its own goroutine.
- [ ] Observe it on a device. The APK builds, which proves the gomobile binding
      exposes `setAppState` and Kotlin compiles against it, but the phone was
      unplugged before the transitions could be watched in logcat.
- [ ] iOS: `sceneDidBecomeActive` / `sceneWillResignActive` /
      `sceneDidEnterBackground`. Same three calls, not yet written.
- [ ] Show it in `examples/capabilities`.

**Exit.** State changes are observed on a device as the app is backgrounded and
restored, and a demo persists on `StateBackground` and restores on relaunch.

---

## M7 — Durable background work

**Goal.** Work an app declares gets done — eventually, once — whether or not the
app is open, on every platform.

**Why now.** Nothing in the tree touches WorkManager, JobScheduler,
BGTaskScheduler or `beginBackgroundTask`, so a gophics goroutine lives only as
long as the OS lets the process run.

**Not a scheduler.** The design (`design/mobile-background.md`) deliberately does
not wrap the platform schedulers. A `Schedule(name, every: 15m)` API looks like
it delegates the problem and does not: it leaves every app to solve idempotency,
retry, backoff, deduplication and persistence alone, which WorkManager does for
you and BGTaskScheduler does not. Instead gophics owns a durable queue, and the
platform mechanisms are demoted to what they are — sources of "you may run now".

That inversion is what makes iOS's unreliability degrade instead of fail (work
that cannot run stays queued and runs at next launch), makes the same API work on
web and desktop, and turns push into an optional wakeup source rather than an
architecture.

The capability is `Handle` / `Enqueue` / `KeepFresh` / `Pending`, with payloads
as `[]byte` rather than closures — work must survive process death, so it cannot
capture app state, and the signature should force that at compile time rather
than at 3am on a user's phone. Handlers take a `context.Context` and cannot
reach the widget tree, because when they run there may be no frames at all.

- [ ] `shell/background.go`, so capgen generates the plumbing.
- [ ] The durable queue: append-only log, at-least-once, persisted backoff,
      deadline cancellation. Pure Go, fully testable headlessly — including that
      at-least-once holds when killed between the work and its acknowledgement.
- [ ] Wakeup sources, cheapest first: launch and foreground (which alone make
      the feature useful everywhere), then the iOS backgrounding grace period,
      then WorkManager, then BGTaskScheduler.
- [ ] CLI writes task identifiers into `Info.plist` and `AndroidManifest.xml` —
      the piece most likely to be underestimated, being build-system work rather
      than API work.
- [ ] Record the device triggers (`adb shell cmd jobscheduler run`, the iOS
      debugger incantation) in the packaging README.
- [ ] Decide whether the requirement is periodic work or *timely* data. If the
      latter, push is the only reliable answer on iOS and is roughly the same
      size; gophics has none today (`shell/notify.go` is local-only).

**Exit.** An app enqueues work, is killed, and the work completes on next
wakeup; and on a physical Android device and a physical iPhone, work enqueued
while backgrounded completes without the app being reopened.

**Ordering note.** After the queue lands with launch/foreground wakeups, the
feature already works durably on every platform with no platform-specific code
at all. Each scheduler after that is an independent improvement, not a
prerequisite — which is the main practical argument for this shape.

---

## M8 — Native menus, exposed to apps ✅

**Goal.** A gophics desktop app can have a real menu bar.

**Why now.** The highest ratio of value to work outstanding, because the hard
part is already built. `gogpu.App.SetMenu` takes a full `Menu` / `MenuItem` /
`MenuRole` model and is implemented for macOS, Linux and Windows, with tests in
`platform/menu_linux.go` and `menu_windows.go`. None of it is reachable: `shell`
publishes no menu capability, so no app can call any of it. A desktop app with
no menu bar reads as unfinished, on macOS especially, where the menu bar is the
application.

This is a capability declaration and a desktop binding — not platform work.
capgen generates the widget, app and posted plumbing from the interface, as it
did for every other capability.

- [ ] `shell/menu.go` declaring `MenuWindow`, and a menu model that maps onto
      gogpu's without leaking it into the public API.
- [ ] Desktop binding, converting to `gogpu.MenuItem` the way
      `a11y_desktop.go` converts nodes.
- [ ] Decide what the other shells do. Web has no menu bar; terminal has none.
      Both should return nil rather than a capability that silently discards —
      the same rule the file pickers follow.
- [ ] Roles matter for macOS: About, Preferences, Quit, Services and the window
      list are placed by the OS, and an app that hand-rolls them looks wrong.
      `MenuRole` already exists; make sure the shell surface carries it.
- [ ] A demo in `examples/` — menus are the kind of thing that looks fine in a
      test and wrong on screen.

**Exit — met (2026-08-17).** `examples/menus` publishes a bar and Windows reports
it back through `GetMenu`, structure intact:

    BAR items=2
    TOP|File|children=6
      ITEM|New / Open… / — / Save As (submenu) / — / Quit
    TOP|Format|children=4

macOS builds it without complaint after the main-thread fix below; the visual
check there still wants a human, since `osascript` needs assistive access this
environment does not grant.

**What the demo caught that the tests could not.** Building the bar from the UI
goroutine raised an uncatchable `NSInternalInconsistencyException` — "Main menu
contents may only be modified from the main thread". The conversion tests all
passed; only running it found this.

Worth keeping in mind next to `a11y_desktop.go`, which had `runOnMain` *removed*
in the same session. The two look alike and want opposite things: an
accessibility activation must reach widget state, so it belongs on the UI
goroutine, while a menu belongs to AppKit and must be built where AppKit lives.
Marshalling is not one hop that fits everywhere.

---

---

## M9 — Close the widget and platform gaps

**Goal.** The things an app author reaches for and finds missing.

Six items, grouped because they share a property: each is small enough to finish
and visible enough that its absence is felt on first use.

- [ ] **Reorderable lists** — drag a row to a new position. The drag machinery
      exists (`Dismissible` swipes, `dragdrop.go`); this is the ordering case.
- [ ] **Tree views** — expand/collapse hierarchy, which every file browser,
      outline and settings pane needs and nobody wants to rebuild.
- [ ] **Autocomplete** — a text field with a filtered suggestion list.
- [ ] **System tray** — a desktop app that keeps running when its window closes
      has nowhere to live without it.
- [ ] **Desktop geolocation** — still open, and deliberately not written blind.
      `ctx.Geolocation()` is nil on desktop today, which is the honest answer;
      the risk is replacing it with code that returns nil for a different,
      invisible reason.

      Each backend needs something this environment cannot provide, established
      by trying rather than assumed:

      - **macOS / CoreLocation** — `CLLocationManager` needs a bundled app with
        `NSLocationUsageDescription` in its Info.plist, a delegate object, and a
        run loop. A `go run` binary is denied before it starts, so the happy
        path cannot be exercised without packaging first. The delegate itself is
        tractable: the a11y bridge already builds an Objective-C class with Go
        methods.
      - **Linux / geoclue** — reachable with the D-Bus client that already
        exists (`dbus_linux.go`, extended for AT-SPI), but `org.freedesktop.
        GeoClue2` lives on the **system** bus, needs the daemon and an agent,
        and needs a real location source. Installing geoclue in a container
        leaves it not activatable, which was checked.
      - **Windows** — no classic Win32 API; `Windows.Devices.Geolocation` is
        WinRT, so it needs `RoActivateInstance` and WinRT's own type system on
        top of the COM work the UIA provider already does.

      The lesson from the UIA bounding rectangle applies directly: platform code
      that cannot be observed working is code that looks right and silently is
      not. Whoever takes this on should start by arranging one verifiable
      target — a signed macOS bundle, or a Linux box with geoclue actually
      running — and implement against that rather than all three at once.
- [ ] **RTL caret geometry** — multi-line landed, but the caret still assumes
      left-to-right, so editing Arabic or Hebrew puts it in the wrong place.

**Exit.** Each is demonstrated in an example and covered by tests that do not
need a device.

**Status (2026-08-18).** Five of six done: Tree, Autocomplete, Reorderable,
draggable scrollbars and RTL caret geometry, plus the system tray with a macOS
backend. Geolocation remains, blocked on verification rather than on effort —
see above.

---

---

## M10 — Sparse strips, part 1: one pipeline, end to end ✅

**Goal.** One stubbed compute pipeline made real, holding the CPU reference.

**Why this shape.** The sparse-strips work is not research from nothing, which
is what PLAN implied before it was checked. `strip.wgsl` is written, the
tilecompute stage shaders exist (`pathtag_reduce`, `pathtag_scan`, `flatten`,
`coarse`, `fine`, `path_count`, `path_tiling`), and a traditional GPU vector
renderer already runs beside them. What is missing is that the pipelines are
stubs, with comments deferring to "when wgpu is ready", which it now is.

So the first milestone is a vertical slice, not a survey: take one rasterizer,
give it a real dispatch, and diff the result against the CPU. Everything after
that is repetition; this is the one that proves the approach and the harness
together.

**The target moved, and that is the main finding.** This milestone was written
against `createStripPipeline`'s `StubComputePipelineID(1)`. Checking before
building showed that was the wrong code to make real: `PipelineCache` — every
stub pipeline, all nine `Stub*ID` types — is constructed nowhere but its own
`renderer_test.go`. Making it real would have produced a working pipeline that
nothing calls, and the tests asserting `GetStripPipeline() != 0` would have gone
on passing either way.

The code that is actually wired is `GPUFineRasterizer`, and it had the more
interesting problem. It compiled its shader, built its bind-group layouts and
created three compute pipelines — and then computed coverage in a Go loop,
because of a comment reading "buffer binding needs HAL API extensions". That had
stopped being true: `CreateBuffer`, `CreateBindGroup` and a native compute pass
with `Dispatch` all exist, and `vello_compute.go` in the same package already
uses them. The comment outlived the limitation by long enough for the type to
look finished.

That is the worst possible shape, and worth naming: a type called
`GPUFineRasterizer`, holding real pipelines, returning *correct pixels* from the
CPU. Every test passed. Nothing was wrong with the output — only with where it
came from.

- [x] Replace the stub with a real compute pipeline and bind-group layout — the
      layouts were already fully described, so this was buffers, bind groups,
      dispatch and readback (`gpu_fine_dispatch.go`).
- [x] Feed it one simple filled path and read the coverage buffer back.
- [x] Diff against the CPU reference. `TestGPUFineMatchesCPU` runs a rectangle
      and a triangle through both paths under both fill rules and compares them
      within one coverage level.
- [ ] Record the cost against the CPU path. `BenchmarkDraw_FillRect/1000x1000`
      is 9.8ms on an M1 Ultra, against a 16.7ms frame — that number is the
      reason for the whole project, and it should move. **Deferred to M12**,
      where the measurement decides whether this becomes the default; there is
      nothing to compare yet, since the dispatch is not on the live path.

**Two real bugs fell out, neither of them GPU work.** Both had been invisible
because no test had ever built a pipeline on a real device:

1. **The fine shader could not run on Metal at all.** The module was created
   from SPIR-V, which is a Vulkan-only input. Metal's HAL takes its "no source"
   branch for it and returns a module with no library behind it, so the failure
   surfaced later and unhelpfully as `invalid compute shader module`. WGSL is
   the portable input — Vulkan compiles it through naga, Metal through its MSL
   writer — so the module is built from WGSL now, and the SPIR-V is still
   compiled because `SPIRVCode()` exposes it for verification.

2. **Tile order was randomised.** `buildTileData` grouped tiles into a map and
   then ranged over it, so the same scene produced its tiles — and therefore its
   whole coverage buffer — in a different order on every call. Nothing
   downstream could diff two runs or cache a tile, and for a project whose
   selling point is golden-image testing that is a bad property to have. Tiles
   are emitted in scanline order now, pinned by
   `TestFineTileOrderIsDeterministic`.

The second one is why the GPU and CPU appeared to disagree on 96 of 128 samples
when the dispatch first worked. They were computing identical pixels and writing
them to different slots.

**Exit.** ✅ A filled path rasterizes through the real compute pipeline on a
Metal device and matches the CPU reference within tolerance, for two fill rules
and two path shapes. The benchmark moves to M12 with the default decision.

**Risk to watch — and it was the right risk.** The stub returns success, so a
half-wired pipeline will look like it works and quietly draw nothing. The
defence held: `RasterizeGPU` never falls back, so a test cannot pass on CPU
pixels, and the comparison separately requires the buffer to contain ink and the
diagonal fixture to contain anti-aliased values — an all-zero result matches an
all-zero reference, and "the GPU wrote nothing" must not read as agreement.
`Rasterize` keeps a fallback for frames in flight, but logs when it takes it.

---

## M11 — Sparse strips, part 2: make the compute pipeline correct

**Goal.** The compute pipeline renders a scene that matches the CPU reference.

**This milestone changed shape once M10 could see the truth.** It was written as
"the remaining tilecompute stages made real, in dependency order", on the
assumption that the stages were stubs waiting to be implemented. They are not.
Every stage — `pathtag_reduce`, `pathtag_scan`, `flatten`, `path_count`,
`path_tiling`, `coarse`, `fine` — is written, and the dispatcher wires all of
them. The work is not construction, it is correctness.

What was actually wrong is that **none of it had ever run**. `CanCompute()` was
false on every machine, so `FillPath`'s compute branch was dead code,
`SelectPipeline`'s compute arm was unreachable, and `TestVelloComputeGolden`
skipped. Nothing reported this: `initGPU` logs the pipeline failure at Warn and
returns nil, and every existing test of `CanCompute()` asserted the *false*
path, so "compute is unavailable" read as a fact about the hardware rather than
as three bugs. It took a test that demanded the true path to find them.

- [x] **naga: parenthesise a select used as a binary operand.** `needsParens`,
      which binary operands go through, omitted `ExprSelect` while its sibling
      `needsParensInContext` listed it. `x + select(a, b, c)` was emitted as
      `x + c ? a : b` — grouped by C++ as `(x + c) ? a : b`, a different number
      from a shader Metal accepts with a warning.
- [x] **naga: bound a global that *is* a runtime-sized array.** Only a struct's
      last member was handled, so `var<storage> tiles: array<Tile>` yielded no
      bound, no bounds check was hoisted out of the atomic, and the access
      guarded itself inside the `&`. The address of a ternary is not an lvalue.
- [x] **Metal: report the real storage-buffer limit.** The HAL published the
      WebGPU baseline of 8 per stage; the argument table holds 31 and entries
      are assigned into it sequentially. The coarse stage binds 9.
- [x] Pin all three with tests that fail without the fix
      (`TestVelloComputeInitialisesOnRealDevice`,
      `TestSelectAsBinaryOperandIsParenthesised`).
- [ ] **Fix the fill bleed.** With the pipeline building, the golden tests run
      for the first time and fail: 12–29% of pixels differ from the CPU
      reference. It is characterised, not guesswork — on
      `compute_blue_square`, a 64×64 target with a 4×4 grid of 16×16 tiles:

      - The CPU reference fills `x=[10..53] y=[10..53]`, a 44×44 square.
        The GPU fills `x=[10..63] y=[10..63]` — the fill starts in the right
        place and then **runs to the right and bottom edges** instead of
        stopping.
      - Every differing pixel has ink in both images with different colour;
        none is missing-versus-present. So geometry arrives, and it is the
        *extent* of the fill that is wrong, not the path.
      - Rows above `y=32` match exactly. The bleed begins at **tile row 2 of
        4** and affects everything below.
      - It is not a global offset: the best single linear shift over the whole
        image is `d=0`.

      That shape — inside-ness persisting past the right edge of a shape, from
      one tile row onward — points at backdrop propagation or the winding
      accumulated across tiles in `coarse`/`path_count`, not at flattening.

      Two suspects are already **ruled out**. The fine stage dispatches
      `(WidthInTiles, HeightInTiles, 1)` = `(4, 4, 1)`, which is correct. And
      the new bounds check on the `tiles` atomics is not silently skipping the
      upper half: for this scene the grid is 16 tiles, `totalPathTiles` is 16,
      the buffer is 128 bytes and the emitted bound evaluates to 16. The
      "breaks at exactly half the tiles" coincidence made that worth measuring
      rather than assuming.
- [ ] Then work outward from the failing stage, keeping the CPU path as the
      reference at every step. `tilecompute.FlattenFill` and
      `RasterizeScenePTCL` are known-good CPU implementations of stages the GPU
      also has, so each can be diffed independently — the same trick that made
      M10 tractable.

**Exit.** `TestVelloComputeGolden` passes, and `gg.AutoSelectCompute` flips to
true in the same change.

**Note on the gate.** Making the pipeline buildable made it *selectable*, which
would have put a 20%-wrong renderer behind `PipelineModeAuto` for any complex
scene. `gg.AutoSelectCompute` holds the previous behaviour while keeping the
fixes; explicit `PipelineModeCompute` still works, because the path has to stay
reachable to be worked on. The gate is at the call site, not inside
`SelectPipeline`, so the heuristic stays a pure function and its tests keep
describing the policy to return to.

---

## M12 — Sparse strips, part 3: make it the default, or do not

**Goal.** Decide on evidence whether the new path replaces the current one.

**Why a milestone.** A second renderer that is not the default is a maintenance
cost with no user. The decision needs numbers and a correctness record, and
those only exist after M10 and M11.

- [ ] Benchmark both paths on the same scenes: large fills, gradients, text,
      blurred backdrops, and a scrolling list — the cases that hurt today.
- [ ] Run the full `gophics_gpu` suite against both on Metal and on Vulkan (the
      Pixel), since a compute pipeline is where backends diverge most.
- [ ] Decide: default, opt-in behind a tag, or removed. Removing is a real
      option and a better outcome than carrying an unused second renderer.
- [ ] Whichever way it goes, update PLAN §5 to say what was measured.

**Exit.** A decision recorded with the numbers behind it, and one renderer that
is clearly the default.
