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

## M1 — Prove the build gates actually run

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
- [ ] `test (framework)` failure: **not reproducible locally.** Passes with
      `-race` on darwin/arm64 and in a linux/arm64 container, with both the
      stale and the regenerated code. CI is linux/amd64; emulating that under
      podman is too slow to be practical. Needs the job log.
- [x] A red run now stops something: `.githooks/pre-push` runs
      `scripts/gates.sh` — the same script CI's lint job runs, so the two
      cannot drift — and refuses the push if any gate fails (~2s). Install per
      clone with `git config core.hooksPath .githooks`; `--no-verify` bypasses.
      Verified by breaking each gate in turn, including a real push that the
      hook rejected.
- [ ] Branch protection requiring CI is still the stronger fix: a hook is
      per-clone, opt-in and bypassable, and it cannot run the test suite. Worth
      turning on before this repo goes public or gains a second contributor.

**Exit.** A named CI run is green on `main`, and the oversized-file gate has
been shown to fail on a deliberately oversized file (done — see above).

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
- [ ] Gamepad on Windows (XInput) — the remaining desktop gap.
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

**Exit.** Battery meets it. `examples/capabilities` shows live values on web
and on every desktop, and returns nil rather than a silent zero elsewhere.
Gamepad meets it everywhere but Windows; geolocation is web-only.

---

## M5 — Accessibility on Linux and Windows

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

- [ ] Implement AT-SPI. The spike says go ahead; this is the large piece.
- [ ] Wire macOS announcements, currently a documented no-op because AppKit
      routes live-region speech through a C function rather than a method.
- [ ] Validate iOS on-device with VoiceOver — the one platform verified only
      in the simulator.

**Exit.** Orca on Linux reads and activates the widget catalogue; the same
for Narrator on Windows.
