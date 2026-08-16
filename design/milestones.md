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

Blocked on a working GitHub token: `gh auth status` reports the stored token
is invalid, which is also why the API returns 404 for this repo.

- [ ] Restore `gh` auth (`gh auth login`), or check the Actions tab directly.
- [ ] Confirm whether the workflows have ever run, and on which commits.
- [ ] If the oversized-file gate never fired for the binary, find out why —
      test it deliberately by committing a large file on a scratch branch.
- [ ] Confirm the new `docs/build-embeds.sh -check` gate runs and can fail.
- [ ] If Actions is disabled or unbilled for a private repo, decide: enable
      it, or move the gates into a pre-push hook so they run somewhere.

**Exit.** A named CI run is green on `main`, and the oversized-file gate has
been shown to fail on a deliberately oversized file.

---

## M2 — Retest GPU text on Android

**Goal.** Establish whether the Vulkan text bug still exists.

**Why now.** PLAN §6.4 records "GPU text draws as solid blocks (glyph
positions correct, coverage wrong)" as Vulkan-only and blocking any
text-bearing Android UI. But after the surface-format fix, every web demo —
including text-heavy ones — rendered correctly on a Pixel 10 Pro over WebGPU,
and the native HN app rendered crisp text too. Same subsystem, same class of
bug: an attachment format that did not match what the pipeline was compiled
for. It is plausible this is already fixed and the plan is out of date.

- [ ] Build and run a text-heavy example natively on the Pixel (the HN app
      is already installed and building).
- [ ] Confirm whether glyphs render correctly on the Vulkan path.
- [ ] Test the second listed bug too: rotated sprites vanishing on the
      direct-surface path.
- [ ] Update PLAN §6.4 either way — strike the bugs, or record what is
      actually wrong now with a reproduction.

**Exit.** PLAN §6.4 matches reality, with device evidence for each claim.

---

## M3 — Ship the 16 KB alignment fix everywhere it is needed

**Goal.** Any documented way of building an Android APK produces one that
runs without the compatibility dialog.

**Why now.** The `-Wl,-z,max-page-size=16384` flag lives only in
`internal/cli/mobile.go`, so it applies when building through the `gophics`
CLI. `examples/tally/package/android.sh` calls `gomobile bind` directly and
misses it, as does anyone following the README. On a Pixel 10 the resulting
APK shows Android's "not 16 KB compatible" dialog on launch. Verified this
session: adding the flag and reinstalling clears it.

Small, and it removes a first-run failure on current hardware.

- [ ] Add the flag to `examples/*/package/android.sh`, or have those scripts
      shell out to the `gophics` CLI so there is one code path.
- [ ] Check the iOS scripts for the same class of divergence.
- [ ] Note in the packaging README that a plain `gomobile bind` is not enough.

**Exit.** An APK built by the packaging script launches on the Pixel with no
compatibility dialog, and both `.so` files report `0x4000` LOAD alignment.

---

## M4 — Fill in the stubbed platform capabilities

**Goal.** Battery, gamepad and geolocation return real data, or are honestly
absent, on every platform.

**Why now.** Nine `TODO(platform)` markers, all the same shape: the capability
is declared and wired but returns nothing on desktop, mobile and web. The
capability layer is a headline claim in `design/positioning.md` — "every
platform service is a plain `ctx.<Cap>()` that degrades cleanly" — and three
of them degrading to silence undercuts it. Each is small and independent, so
this milestone can land in pieces.

- [ ] Battery: IOKit `IOPowerSources` (macOS), `upower`/sysfs (Linux),
      `GetSystemPowerStatus` (Windows), `BatteryManager` (Android),
      `navigator.getBattery` (web).
- [ ] Geolocation: CoreLocation, geoclue, Win32, FusedLocationProvider.
- [ ] Gamepad: the web Gamepad API first — it is the cheapest and already
      has a demo to prove it in (`examples/capabilities`).
- [ ] For anything that stays unimplemented, return nil from the capability
      rather than a working-looking stub, as the file pickers now do.

**Exit.** `examples/capabilities` shows live values for each on at least one
platform, and returns nil (not a silent zero) everywhere else.

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

The largest item here. Windows UIA in particular is a COM server, which is
substantial in pure Go; worth a spike before committing to it.

- [ ] Spike: can AT-SPI be driven over D-Bus from pure Go? (Likely yes —
      it is a D-Bus protocol, and there is no C API requirement.)
- [ ] Spike: what does a minimal UIA provider need in pure Go, and is the
      COM surface tractable without CGo?
- [ ] Implement AT-SPI first if the spike is favourable.
- [ ] Wire macOS announcements, currently a documented no-op because AppKit
      routes live-region speech through a C function rather than a method.
- [ ] Validate iOS on-device with VoiceOver — the one platform verified only
      in the simulator.

**Exit.** Orca on Linux reads and activates the widget catalogue; the same
for Narrator on Windows.
