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

- [ ] Store an API token: `gh auth login` (choose HTTPS when asked, or paste
      a PAT), or export `GH_TOKEN`. SSH alone is not enough for the API.
- [ ] Confirm whether the workflows have ever run, and on which commits.
- [ ] Check Settings ▸ Actions — a private repo can have Actions disabled
      entirely, which would explain a guard that never fired. `pages.yml`
      carries a note that Pages must be enabled by hand, so it is plausible
      neither was ever turned on.
- [ ] Confirm the new `docs/build-embeds.sh -check` gate runs and can fail.
- [ ] If Actions is disabled or unbilled for a private repo, decide: enable
      it, or move the gates into a pre-push hook so they run somewhere.

**Exit.** A named CI run is green on `main`, and the oversized-file gate has
been shown to fail on a deliberately oversized file.

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
