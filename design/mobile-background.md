# Background work on mobile

Two capabilities that are often conflated and should not be: knowing the app
went to the background, and running while it is there. The first is small and
already designed. The second is a genuine platform integration, and the design
below exists mostly to be honest about why.

Status today: `ctx.Lifecycle()` is nil on Android and iOS
(`shell/mobile/lifecycle.go` returns nil), and nothing in the tree touches
WorkManager, JobScheduler, BGTaskScheduler or `beginBackgroundTask`. A gophics
goroutine runs exactly as long as the OS lets the process run, which after
backgrounding is seconds.

---

## Why this is not just "spawn a goroutine"

Every other capability delivers its callbacks onto the UI goroutine, because
widget state is owned there. `capgen` enforces it: each capability gets a
`Posted<Cap>` wrapper that marshals callbacks through `Owner.Post`, which is
drained at the top of each frame (`core.drainPosted`).

Background work breaks that assumption at the root. **When a background task
runs there may be no frames at all** — the surface is released, the render loop
is stopped, and on Android the Activity may be destroyed while the process
lives. A callback posted to the UI goroutine in that state is not delayed; it
is never run.

So background work must not be modelled as a widget callback. It needs its own
execution context with no access to the widget tree, which is why the API below
hands the app a `context.Context` and a plain function rather than routing
through `Ctx`. This is the one place the framework's central contract has to be
deliberately *not* applied, and saying so in the API is better than letting
someone discover it when their handler silently does nothing.

---

## Phase A — `Lifecycle` on mobile

Small, already specified in the TODO in `shell/mobile/lifecycle.go`, and worth
doing first: it is what tells an app to persist state *before* it is killed,
which is the majority of what people actually want from "background support".

Map the host callbacks onto the existing three-state ladder:

| gophics | Android | iOS |
|---|---|---|
| `StateActive` | `onResume` | `sceneDidBecomeActive` |
| `StateInactive` | `onPause` | `sceneWillResignActive` |
| `StateBackground` | `onStop` | `sceneDidEnterBackground` |

Wiring follows the shape the Bridge already uses for `Focused`: an inbound
`Bridge.SetAppState(state int)` called from Kotlin/Swift, with `Bridge.Lifecycle()`
returning a real implementation instead of nil.

Note the existing `Bridge.Focused` is **window focus, not background**, and the
TODO says explicitly not to reuse it as a proxy. `onPause` fires for a dialog
over the app; `onStop` is the one that means "not visible".

**Exit.** `examples/capabilities` shows the state changing on a device as the
app is backgrounded and restored, and a demo persists state on
`StateBackground` and restores it on relaunch.

**Effort.** Small. Go side is an atomic plus a callback list; host side is four
one-line overrides per platform.

---

## Phase B — `BackgroundTask`

This is the real work, and it is mostly *not* Go.

### The fork, and which side of it people mean

"Background task" means two unrelated things, with different APIs, different
limits, and different review consequences. They are not substitutes, and most
apps eventually want both — but they are not equally weighted.

**When someone asks whether a framework supports background tasks, they mean
the deferred/periodic sense**: sync, fetch, upload while the app is not open.
The grace-period variant is real and useful, but nobody calls it a background
task; they call it "finishing the upload". So the deferred half is both the
harder work and the one that answers the question, and it is what M7 should
target unless there is a specific reason otherwise.

**1. Finish what you started** — a few seconds of grace to complete an upload or
flush a database after the user leaves.
- iOS: `beginBackgroundTask(withName:expirationHandler:)`, roughly 30 seconds.
- Android: no ceremony needed for a short tail, or a foreground service if it
  must be reliable.
- Cheapest to implement, covers the common case, no entitlements.

**2. Deferred and periodic** — sync every few hours, upload when on Wi-Fi.
- iOS: `BGAppRefreshTaskRequest` (~30s, opportunistic, the system decides when)
  and `BGProcessingTaskRequest` (longer, typically while charging).
  Identifiers must be declared in `Info.plist` under
  `BGTaskSchedulerPermittedIdentifiers` **and** registered before the app
  finishes launching — both are build-time facts, not runtime ones.
- Android: WorkManager. Periodic work has a **15-minute minimum interval**, a
  Worker subclass, and constraints (network, charging, idle). Doze and App
  Standby buckets mean "every 15 minutes" is an aspiration, not a schedule.
- More work, and the scheduling is advisory on both platforms.

Anything continuous — audio playback, turn-by-turn location — is a third thing
again: entitlement-gated background modes on iOS and a typed foreground service
on Android, both with App Store or Play review implications. Out of scope here;
it belongs to whichever capability owns that domain (audio, geolocation).

### What the deferred half cannot promise

Committing to deferred/periodic means committing to an API that is honest about
being advisory, because on neither platform is it a schedule.

**iOS is opportunistic and may simply never run.** `BGAppRefreshTask` is
scheduled by the system against its own model of battery, usage and network. Two
cases produce no execution at all, and both are common:

- If the user force-quits the app from the app switcher, iOS stops scheduling
  background refresh for it until the app is launched manually again.
- Low Power Mode, and the per-app Background App Refresh switch in Settings,
  disable it outright.

**Android is more dependable but still not a clock.** WorkManager honours a
15-minute floor for periodic work, Doze batches wakeups into maintenance
windows, and App Standby buckets throttle rarely-used apps. Several OEM Android
skins kill background work far more aggressively than stock, which is a
well-known and unfixable-from-inside problem.

The API must therefore never look like `time.Ticker`. `Schedule.Earliest` is a
floor, not a target, and the documentation should say plainly that a task may
run late, may be coalesced, and may not run at all. An app that needs
correctness must treat every run as opportunistic: idempotent, resumable, and
safe to be killed halfway, since both platforms will do exactly that.

### The part that changes the architecture: push

For the commonest reason people want periodic background work — *keep the data
fresh* — iOS's intended answer is not the task scheduler. It is **silent push**
(`content-available: 1`), where the server decides there is new data and wakes
the app. BGAppRefresh is designed for prefetching around habitual usage, not for
delivering timely updates, and treating it as a sync mechanism is the single
most common way apps end up with stale data on iOS.

gophics has no push capability. `shell/notify.go` is **local notifications
only** — `Notifier.Notify` posts on-device; nothing registers for APNs or FCM
tokens or handles a remote payload.

That is worth knowing before M7 is scoped, because it means:

- A background *scheduler* alone will not deliver reliable sync on iOS, however
  well implemented.
- The complete answer to "keep my app's data fresh" is probably
  scheduler + push, and push is its own capability (token registration,
  server-side certificates or FCM, payload handling, and a background delivery
  path that has the same no-UI-goroutine problem described above).
- If the actual requirement is timely data rather than periodic work, **push may
  be the more valuable capability to build first**, and it is roughly the same
  size.

### Proposed API

Declared in `shell/backgroundtask.go`, so `capgen` generates the widget, app
and posted plumbing automatically. Note the getter shape the generator
requires: zero arguments, single interface result.

```go
// BackgroundTaskWindow is implemented by a Window that can run deferred work.
type BackgroundTaskWindow interface {
    BackgroundTasks() BackgroundTasks
}

type BackgroundTasks interface {
    // Register declares a handler for a named task. It must be called during
    // startup, before the app can be backgrounded: both platforms bind task
    // identifiers at launch, and iOS refuses registrations afterwards.
    Register(name string, fn func(ctx context.Context) error)

    // Schedule asks the OS to run a registered task. When is a hint, not a
    // promise — the system decides, and may never run it at all.
    Schedule(name string, when Schedule) error

    // Cancel withdraws pending runs of a task.
    Cancel(name string)
}

type Schedule struct {
    // Earliest is the soonest the task should run. Android rounds up to its
    // 15-minute floor for repeating work.
    Earliest time.Duration
    // Repeat asks for periodic execution rather than a single run.
    Repeat bool
    // RequiresNetwork and RequiresCharging map onto WorkManager constraints
    // and BGProcessingTaskRequest's flags.
    RequiresNetwork  bool
    RequiresCharging bool
}
```

Three properties the API deliberately enforces:

- **The handler takes a `context.Context`, not a `widget.Ctx`.** It cannot
  touch the widget tree, and the type says so. The context carries the
  platform's deadline, so `ctx.Done()` is the expiration handler both platforms
  demand.
- **The handler returns an error**, because both platforms want to be told
  whether the work succeeded — iOS `setTaskCompleted(success:)`, Android
  `Result.success()`/`retry()`.
- **`Register` is separate from `Schedule`**, mirroring the platforms rather
  than papering over them: iOS binds identifiers at launch and a closure
  registered later is never invoked.

### Host wiring

Follows `MediaHost` exactly (`shell/mobile/media.go`) — the established pattern
for "Go asks, the host performs, results come back by reqID on the UI thread":

```go
type BackgroundHost interface {
    Schedule(name string, earliestSec int, repeat, network, charging bool)
    Cancel(name string)
}
func (b *Bridge) SetBackgroundHost(h BackgroundHost)
// inbound, from the host when the OS starts a task:
func (b *Bridge) RunBackgroundTask(name string, deadlineSec int) // → the handler
func (b *Bridge) BackgroundTaskFinished(name string, ok bool)    // host reports back
```

One deviation from the MediaHost rule is required and must be documented at the
call site: `RunBackgroundTask` runs the handler on **its own goroutine**, not
the UI goroutine, precisely because the UI goroutine may not be running. It is
the only Bridge entry point that does so.

Host side per platform:
- **Android** — a `Worker` subclass that calls `RunBackgroundTask`, plus
  WorkManager scheduling and manifest registration.
- **iOS** — `BGTaskScheduler.register` at launch for each identifier, the
  identifiers listed in `Info.plist`, and `setTaskCompleted` on return.

Both are template changes in `internal/cli/templates/mobile/`, which means the
`gophics` CLI must learn to write the task identifiers into `Info.plist` and
`AndroidManifest.xml` from app config. That is the piece most likely to be
underestimated: it is build-system work, not API work.

### Testing

The Go side is testable headlessly with a fake host — registration, scheduling,
cancellation, deadline propagation, and that a handler never sees the widget
tree. That is where the tests should live, as with the AT-SPI tree logic.

Device testing needs the platform's own triggers, and they are awkward by
design:
- iOS: the debugger-only
  `_simulateLaunchForTaskWithIdentifier:` incantation, since the real scheduler
  may wait hours.
- Android: `adb shell cmd jobscheduler run -f <pkg> <jobid>`.

Both belong in the packaging README next to the 16 KB alignment note.

**Exit.** A demo app schedules a task, is backgrounded, and the task is observed
to run on both a physical Android device and a physical iPhone, with its result
visible on next launch.

**Effort.** Substantial, and mostly host-side: two schedulers, two manifest
generators, and a testing story that depends on platform tooling. The Go
surface above is perhaps a day; the rest is the milestone.

---

## Recommended order

1. **Phase A (Lifecycle)** — small, self-contained, and delivers most of the
   practical value, since "save before you die" is what most apps need.
2. **Decide what the requirement actually is.** Periodic work (a scheduler), or
   timely data (push, with a scheduler as a fallback)? They look like the same
   ask and are not. If it is timely data, build push first — same size, better
   answer, and on iOS it is the only reliable one.
3. **Phase B**, scoped to the answer. Deferred/periodic is the default reading
   of "background task" and the harder half; plan for the CLI manifest work and
   the device-trigger tooling, which are where the time actually goes.

Doing 1 first is not a stalling tactic: without lifecycle states there is no way
to observe that a background task ran at the right moment, so Phase A is also
Phase B's instrumentation.
