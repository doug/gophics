# Example application ideas

A running shortlist of open-source clones worth building on gophics — apps that
provide real value, run standalone (no ongoing server/compute cost), and play to
the platform's grain. Captured from a brainstorm; revisit and prune as we build.

## Selection filter (why these, not others)

Gophics's wedge: **one small Go binary (no Electron), instant launch, GPU-smooth
scrolling, offline-first, one codebase across desktop/web/mobile, and data that's
just your own files** (the web build reads/writes real local files via File System
Access). The best clones are apps that are:

1. Used **daily**.
2. Currently **subscription-, ad-, or cloud-locked** — or recently orphaned.
3. Shaped like **text + lists + light canvas** (the framework's strengths).
4. Genuinely **better when local and everywhere**.
5. **Zero server** — pure local compute + files; only software maintenance, no hosting.

The sharpest sub-pattern: apps whose whole business is **rent on something that
could just be a local binary** — especially online tools that make you **upload
your files** (that's the grift *and* a privacy hazard). A local clone deletes the
server, the cost, and the privacy problem at once.

---

## A. Own-your-data productivity (best fit; seeds exist)

- **Tasks** — Things / Todoist / TickTick. Seed: `examples/todo`. Projects/areas,
  Today/Upcoming, natural-language dates, recurring, swipe-to-complete. Flagship,
  great on both platforms.
- **Notes / PKM** — Obsidian / Bear. Seed: `examples/notes`. Daily notes, backlinks
  graph (Canvas), tags, wikilinks. Sync = your folder (iCloud/Git). Uses the
  text-editing + caret-into-view work directly.
- **Budget** — YNAB / Actual. Envelope budgeting, transaction LazyLists, trend
  charts (Canvas), CSV/OFX import. Replaces ~$109/yr; pairs with `finance-cli`. The
  visualize-half of this ("**Ledger**") is the chosen driver for a built-in Swift
  Charts–style `chart` package — see **`design/charts-plan.md`**.
- **Journal** — Day One. Text-heavy, calendar, "on this day", local encrypted files.
  Replaces a subscription for deeply personal data. (See also the media-journal in
  section G.)
- **Habit / streak tracker** — Streaks / HabitKit. Highest value-per-effort:
  contribution-graph heatmap is a perfect Canvas showcase; the rest is simple lists
  and daily taps. Fast, delightful, mobile-friendly.

## B. Read & consume (text-render + infinite-scroll showcase; timely)

- **Read-later** — Pocket / Omnivore. Timely: Omnivore shut down and stranded users.
  Save → reader view → highlights → offline. GPU-smooth long-scroll + text rendering
  is the whole product. (Needs web fetch/parse — see gaps.)
- **RSS reader** — NetNewsWire / Reeder. LazyList is built for feeds; reader view
  leans on text. OPML import, local cache. (Needs web fetch.)
- **Mastodon / Bluesky client** — reverse/infinite timeline already demoed in
  `examples/gallery`. Text + images + threads. Fast, no-tracking client.

## C. Developer / power tools (desktop-strong)

- **JJ / Jujutsu GUI** — *the sharpest idea here.* The space is nearly empty (jj has
  only TUIs + one early Tauri app), the ecosystem is growing, and jj's model fits a
  GUI better than Git's (no index; working copy is a commit; auto-rebase; first-class
  conflicts; operation log). Perfect shape: commit **DAG on Canvas**, **diffs** as
  text, **lists** for changes/bookmarks/revsets, file **tree**.
  - Killer feature nobody's nailed: a **visual `jj op log` / `jj undo`** — a
    scrubable time-travel timeline. Plus **drag-to-rebase**, inline split/squash/
    describe.
  - Architecture: porcelain over the `jj` binary — `os/exec`, parse
    `jj log -T <template>` / `status` / `diff` / `op log`, map actions to subcommands.
    Zero server. Personal dogfood (we use jj; see the `jj`/`jjj` skills).
  - Challenges: stable structured output from a fast-moving CLI; diff/merge/conflict
    *editing* is the hard 20%. Desktop-first.
- **API client** — Bruno / Postman. Bruno's pitch is *local-first, requests-as-files*
  — dead-on for gophics; timely as Postman pushes to the cloud. Forms + JSON text +
  history lists.
- **Git GUI** — Tower / Fork. Commit/diff lists, diff view, branch tree. (JJ GUI is
  more differentiated.)
- **DB GUI** — TablePlus / Beekeeper. `layout/grid` results, SQL editor, connections.
- **Snippet / clipboard manager** — Paste / Alfred powerpack. Local, desktop, paid
  incumbents.

## D. "Rent on a local utility" — ad/freemium local tools (strictly-better clones)

### "Stop uploading my private files" (online → local; purest win)
- **PDF toolkit** — Smallpdf / iLovePDF / Adobe Acrobat. Merge/split/compress/rotate/
  reorder/watermark/sign — locally, no upload. Go's `pdfcpu`. Huge constant demand;
  privacy + price + speed win. **Top pick for this category.**
- **Image compress/convert** — TinyPNG / online converters (which upload). Batch
  resize/convert/compress JPEG/PNG/WebP/HEIC. Go `image` libs. Local, private.

### "Stop subscribing to a local utility"
- **Food + weight tracker** — MyFitnessPal. One of the most *resented*
  enshittifications; local log + bundled offline food DB, no ads, no selling health
  data. Lists + forms + charts.
- **Flashcards / spaced repetition** — Quizlet / AnkiMobile ($25). Scheduling is 100%
  local; text + lists + stats heatmap. Student demand + enshittification backlash.
- **Workout logger** — Strong / Hevy. Sets/reps, rest timer, progress charts. Local.
- **EPUB reader / library** — Kindle lock-in / clunky Calibre. EPUB is HTML/text —
  leans on the text-rendering + GPU-scroll engine. Local library, highlights, no DRM.
  Strong *flagship* to show off rendering. No good OSS one exists.
- **Breathing / meditation timer** — Calm / Headspace ($70/yr). For the "timer +
  calming visual" chunk of usage: Canvas breathing animation + streaks, no content
  subscription. (Guided *audio* needs the audio layer — see gaps.)

### "Stop selling my intimate data" (privacy as the feature)
- **Period / cycle + mood tracker** — notorious for selling data; "never leaves your
  device" is a genuine pitch. Calendar (Canvas) + simple logs. Handle tastefully.

### Zero-server creative (Canvas showcases)
- **Whiteboard** — Excalidraw / Apple Freeform. Shapes, arrows, text, freehand.
  Delightful, entirely local.

## E. Games (clean clones of ad-riddled classics; best mobile play *today*)

Casual/turn-based games need only **touch + Canvas + animation + layout/grid** — all
present — so they sidestep the sensor/camera gaps and are the strongest *mobile*
target right now. Incumbents are famously predatory (video ads between hands, fake
currency, energy timers on card games), so ad-free/offline/gorgeous is strictly
better. Also great dogfood for hardening drag/animation/input. (Not a game *engine* —
no sprites/physics — but these are UI-games.)

The purest offenders here charge **rent on a daily puzzle**: generating a 9×9 grid
or checking a five-letter word is a few KB of offline code, yet it ships behind a
subscription, 30-second unskippable video ads, or an **energy timer** ("wait 30
minutes or pay $0.99"). Stripped of the artificial scarcity, these are delightful,
self-contained, and *shipped* — `examples/solitaire` is the first (deal → play →
auto-complete → win cascade, all local). The deep framework plan behind the games
workstream — input model, audio mixer, sprite/path primitives, mobile GPU present, the
`game` package, staging and risks — lives in **`design/games-plan.md`** (this section is
the menu; that doc is the build plan).

- **Solitaire collection** — Klondike (shipped) → Spider/FreeCell/Pyramid on one
  engine. Even MS Solitaire has ads + a subscription now. Card drag + deal/flip
  animations.
- **Classic card games vs. local AI** — Hearts, Spades, Euchre, Gin Rummy,
  **Cribbage** (the board is a Canvas gift; devoted fans; terrible incumbents).
- **Daily word & logic (the NYT Games bundle)** — deterministic-by-date, bundled
  data, no account — the sharpest "zero server, pure rent" shape:
  - **Wordle-style** guess game (bundled word list).
  - **Connections** — sort 16 words into 4 groups; a tight grid + selection +
    shuffle + reveal animation. Tiny logic, big daily habit.
  - **Mini crossword** — grid + clue list + text entry; leans on `layout/grid`.
- **Sudoku / Nonogram** — grid + pencil marks + local generator/solver. Negligible
  compute; the whole "premium" incumbent is a paywalled generator.
- **Match-3 / falling-tile** — Candy Crush / Two Dots / Royal Match, minus the
  energy timers and microtransactions engineered to monetise frustration. Grid swap
  + gravity + match-cascade + juice: the strongest **animation/Canvas dogfood** in
  this list (harder than the card games; a good stress test for the paint/anim path).
- **Brain-training minigame pack** — Lumosity / Peak / Elevate stripped of the
  pseudo-neuroscience subscription: a set of 60-second reaction / memory / pattern /
  mental-math minigames behind one results screen. Exercises **fast input +
  per-game animation + a stats/heatmap screen**; naturally grows one minigame at a
  time.
- **2048**, **Minesweeper** — grid + simple rules; classic quick wins.
- **Tile roguelike / dungeon crawler** — *shipped* (`examples/roguelike`): the
  driver for `paint.DrawSprite`. Procedural tileset generated in Go (one shared
  atlas, no assets), room/corridor dungeon, LOS fog-of-war, a minimal d20 combat
  core. Bitmap/atlas showcase (vs. solitaire's vector cards). Deeper rules could
  build on the CC-BY SRD 5.2 later.

## F. Simple mobile utilities

- **Work today (compute/UI only):** calculator, unit/currency converter, tip/bill
  splitter, timer/stopwatch/interval, dice / random picker / decision wheel (Canvas),
  password generator, QR **generator**, countdown, checklist. Each is its own ad-farm
  today → a clean **"utility belt"** app can replace a dozen, all local.
- **Gated on device APIs (build capability first):** compass (magnetometer),
  level/ruler (accelerometer), document scanner & QR **scanner** (camera), flashlight
  (torch), reminders (background notifications). Juiciest targets (CamScanner is
  maximally hateable) but each needs mobile plumbing.

## G. Capability-driving apps (forcing functions for core platform work)

Pick an app whose value *requires* a capability we haven't built, so building it
hardens a **reusable core layer**, not a one-off.

- **Multimedia capture journal** — private, local **Day One**: text + inline **photo**
  (camera) + **voice memo** (mic record + playback, waveform on Canvas). The cleanest
  single app that forces **camera + audio-in + audio-out** in their most general forms,
  while being lovable and on-thesis. Builds on `examples/notes`.
  - Stretch: **1 Second Everyday** (adds *video* capture) — same capability work,
    more delight, more viral.
- Forces into core (the real prize): `shell.Camera` (permission + still capture; later
  preview/video) and `shell.Audio` (out: play PCM/buffer; in: record mic → PCM + level
  metering).
  - **Sequence web-first:** browser gives all three with no native code
    (`getUserMedia`, `MediaRecorder`, Web Audio) — prove the gophics-side API + UX in
    the browser, then implement the same `shell` interface on **gomobile**
    (AVFoundation/CameraX + AVAudioEngine/AudioTrack). Desktop audio via `oto`/`malgo`;
    desktop camera optional (CGo/AVFoundation/V4L2 — annoying, phone-first anyway).
  - Tiers: **T1** still-photo + voice record/playback (buffered). **T2** video +
    *low-latency* audio (tuner/metronome/games — a real notch harder; keep out of T1).
  - Unlocks afterward: document scanner, QR scanner, voice recorder, tuner/metronome,
    podcast player, language flashcards (record-and-compare), game sound.

---

## Platform gaps that gate ideas

- **Web networking** — HTTP fetch + parse for read-later/RSS/API-client is easy on
  desktop (stdlib) but on the *web* build means `fetch` + CORS proxies. Tier A/D/E
  apps sidestep this entirely.
- **Audio in/out** — no audio API yet. Gates recorder, tuner, metronome, podcast
  player, guided meditation, game sound. Low-latency is a harder tier than buffered.
- **Camera** — no capture API yet. Gates scanner, QR scanner, media journal, video.
- **Sensors** (accel/magnetometer/light), **torch**, **background notifications**,
  **haptics**, **contacts** — gate a chunk of native mobile utilities.
- **Mobile maturity** — `run_mobile.go` exists but gomobile is the least-proven target.
  Solid today for read/write/list apps and touch-only games; the above capabilities
  need building.

## Fit-by-shape cheat sheet

- **Text-heavy** → notes, journal, read-later, EPUB reader, diff views (JJ/Git GUI).
- **Lists / feeds (LazyList)** → tasks, RSS, budget txns, workout log, timelines.
- **Canvas** → habit heatmap, charts, whiteboard, games, commit DAG, waveform,
  breathing animation.
- **Grid** → sudoku, minesweeper, DB results, spreadsheets.
- **Local files (FSA/os)** → everything local-first; PDF/image tools; PKM.

## Suggested starting points

- **Fast high-signal proof:** habit tracker (Canvas heatmap, tiny), a daily puzzle
  (Connections / Sudoku — deterministic, no server, natural follow-on to solitaire),
  or more of the solitaire/card collection.
- **Juiciest animation dogfood:** match-3 (grid swap + gravity + cascade) — bigger,
  but a real workout for the paint/anim path.
- **Flagship, both platforms:** tasks (from the `todo` seed) or EPUB reader
  (shows off the rendering engine).
- **Differentiated + personal:** JJ / Jujutsu GUI.
- **Purest "strictly better, zero server":** local PDF toolkit.
- **Core-capability forcing function:** multimedia capture journal (→ camera + audio).

## In flight / queued (session log)

- **Health dashboard** — IN PROGRESS (`examples/health/`). Live metric cards
  (real-time heart rate, steps, weight, sleep) over a `Provider` interface.
  Phase 1: synthetic live provider (desktop/web). Phase 2: bind the same
  interface to HealthKit (iOS) / Health Connect (Android) — health stores are
  on-device only, so real live data is the mobile build's showcase: one Go UI,
  pixel-identical, real native data on the phone.
- **WebRTC calling app** — QUEUED. A 1:1 (or small-group) call app to exercise
  the WebRTC stack end-to-end *and* the audio/video capture + playback paths:
  camera preview, mic capture, peer connection/signalling, remote video render,
  call controls (mute/hangup/switch camera). Doubles as the forcing function for
  the media capabilities (`shell.Camera`, `shell.Audio`) and a real-time
  networking dogfood. Big, but a strong differentiated showcase once mobile
  camera/audio bring-up lands.
