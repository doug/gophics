# Background work

Two capabilities that are easily conflated and should not be: knowing the app
went to the background, and doing work while it is there. The first is small and
already designed. The second is the subject of most of this document, and the
design below is deliberately *not* a wrapper around the platform schedulers.

Status today: `ctx.Lifecycle()` is nil on Android and iOS
(`shell/mobile/lifecycle.go` returns nil), and nothing in the tree touches
WorkManager, JobScheduler, BGTaskScheduler or `beginBackgroundTask`. A gophics
goroutine runs exactly as long as the OS lets the process run, which after
backgrounding is seconds.

---

## 1. Start from what a developer is expressing

Nobody wants "run my code at 3pm". Two jobs account for nearly all of it:

1. **This work must eventually happen.** An upload, a queued message, an
   analytics batch. Don't care when — care that it completes, once, surviving
   app close, network loss and reboot.
2. **This data should be current when the user looks.** A feed, a mailbox.
   Also don't care when; the requirement is a property at open time.

Both are *durable intent*. Scheduling is one implementation detail of satisfying
them, and on iOS the least reliable one.

### Why a scheduler is the wrong abstraction

An API shaped like `Schedule(name, every: 15*time.Minute)` looks like it
delegates the problem to the platform. It does not. It leaves every app author
to solve the same five things, alone:

- idempotency, because both platforms kill work halfway
- retry and backoff
- deduplication, so backgrounding twice does not queue two syncs
- persistence across process death and reboot
- "did this already run?"

WorkManager solves those. BGTaskScheduler emphatically does not. A capability
that merely forwards to both inherits the worst of each, and the app pays the
difference — badly, and differently on each platform.

**So gophics should own the queue, and demote the platform mechanisms to what
they actually are: sources of the signal "you may run now".**

---

## 2. The capability

```go
// BackgroundWindow is implemented by a Window that can run durable work.
type BackgroundWindow interface {
    Background() Background
}

type Background interface {
    // Handle registers the handler for a kind of work. Must be called during
    // startup, before any work can be delivered — and because iOS binds task
    // identifiers at launch and refuses registrations afterwards.
    Handle(kind string, fn func(ctx context.Context, payload []byte) error)

    // Enqueue records durable work. It survives backgrounding, process death
    // and reboot, and runs at least once. It returns when the intent is
    // persisted, not when the work is done.
    Enqueue(kind string, payload []byte) error

    // KeepFresh asks that a kind be re-run opportunistically whenever its last
    // success is older than maxAge. Best effort, by definition.
    KeepFresh(kind string, maxAge time.Duration)

    // Pending reports outstanding work, so a UI can show "uploading 3 items"
    // or surface a failure instead of pretending everything is instant.
    Pending() []Job
}

type Job struct {
    Kind     string
    Attempts int
    LastErr  string
    NextTry  time.Time
}
```

Declared in `shell/background.go` so `capgen` generates the widget, app and
posted plumbing from it — the getter shape it requires is zero arguments and a
single interface result, which `Background() Background` satisfies.

### Four consequences, which are the design

**The payload is data, not a closure.** This is the constraint people do not
anticipate: work must survive the process dying, so it cannot capture app state.
A `[]byte` signature forces that into the open at compile time rather than at
3am on a user's phone. Apps encode with whatever they like; JSON is the obvious
default.

**The handler takes `context.Context`, never a widget `Ctx`.** When this runs
there may be no frames at all — surface released, render loop stopped, on
Android the Activity possibly destroyed while the process lives. Every other
capability delivers callbacks through the generated `Posted` wrapper onto the UI
goroutine, drained at the top of each frame; here that would not be delayed, it
would never run. The type says so, and `ctx.Done()` carries the platform's
deadline, which is also the expiration handler both platforms require.

**`Enqueue` returns when persisted, not when done.** That is the honest
contract, and it is what lets the API be identical on every platform.

**`Pending()` exists** because background work a user cannot see is background
work they do not trust. Most apps want to say "3 items waiting" somewhere.

### At-least-once, with a dedup key

**Recommended: at-least-once.** Exactly-once is not deliverable across a
`SIGKILL` between "work done" and "queue updated", and an API promising it would
be lying. At-least-once is simple, honest, and pushes idempotency onto the
handler, which is the only place that can implement it.

`KeepFresh` deduplicates by kind — a refresh already queued is not queued again.
`Enqueue` does not deduplicate by default, since two uploads are usually two
uploads; an optional dedup key can be added later if a caller needs it.

---

## 3. The inversion: platforms as wakeup sources

Every platform mechanism reduces to one signal — *you may run now* — after which
the framework drains its own queue:

| Source | Platform |
|---|---|
| App launch / foreground | all |
| `WorkManager` worker fires | Android |
| `BGAppRefreshTask` / `BGProcessingTask` | iOS |
| Silent push | iOS, Android |
| Network regained | all |
| Backgrounding grace period | iOS `beginBackgroundTask` |

This buys three things that the scheduler-shaped API cannot:

- **iOS unreliability degrades instead of failing.** If the system never wakes
  the app — force-quit, Low Power Mode — the work stays queued and runs at next
  launch. For freshness that is precisely when it matters.
- **The same API works on web and desktop**, where it is nearly trivial: desktop
  just runs it, web on next load or in a Service Worker. Background work stops
  being a mobile escape hatch and becomes portable.
- **Push becomes an optimisation, not an architecture.** Silent push is simply a
  better wakeup source for the same queue. Ship without it, add it later,
  without changing a line of app code — which is not true of a scheduler API.

---

## 4. The queue

The part gophics now owns. Small, but it must be right, because everything above
rests on it.

- **Storage.** An append-only file under the app's data directory, one record
  per operation (enqueue, attempt, success, failure). Append-only survives being
  killed mid-write in a way that rewriting a JSON blob does not; compact on
  clean start.
- **Delivery.** On a wakeup: load, filter to due work, run handlers
  sequentially, record outcomes. Sequential rather than concurrent, because the
  wakeup budget is measured in seconds and parallel work under a deadline mostly
  produces more half-finished work.
- **Deadline.** Every wakeup carries one. The runner stops starting new jobs when
  the remaining budget looks insufficient, and cancels the running one via
  `ctx.Done()`.
- **Backoff.** Exponential with a cap, persisted, so a failing job does not burn
  every future wakeup.
- **Ordering.** FIFO within a kind. No cross-kind guarantee.

It is worth stating that this is the piece with the highest ratio of value to
platform knowledge: it is pure Go, entirely testable headlessly, and it is the
code every app using a naive scheduler API would have written worse.

---

## 5. Working back to platform mechanics

### Phase A — `Lifecycle` on mobile (do first)

Already specified in the TODO in `shell/mobile/lifecycle.go`. Map host callbacks
onto the existing three-state ladder:

| gophics | Android | iOS |
|---|---|---|
| `StateActive` | `onResume` | `sceneDidBecomeActive` |
| `StateInactive` | `onPause` | `sceneWillResignActive` |
| `StateBackground` | `onStop` | `sceneDidEnterBackground` |

Inbound `Bridge.SetAppState(state int)` from Kotlin/Swift, and a real
`Bridge.Lifecycle()` instead of nil. Do **not** reuse `Bridge.Focused` — it is
window focus, and fires for a dialog over the app; `onStop` is the one that
means "not visible".

This is also the queue's trigger for a flush-on-background attempt, and its
instrumentation: without lifecycle states there is no way to observe that
background work ran at the right moment.

### Host wiring

Follows `MediaHost` (`shell/mobile/media.go`), the established pattern for "Go
asks, the host performs, results come back through the Bridge":

```go
type BackgroundHost interface {
    // RequestWakeup asks the OS for a future opportunity. Advisory.
    RequestWakeup(earliestSec int, needsNetwork bool)
}
func (b *Bridge) SetBackgroundHost(h BackgroundHost)

// inbound, when the OS grants a window:
func (b *Bridge) RunDueWork(deadlineSec int) // drains the queue
```

Note how much smaller the host surface is than in a scheduler design: the host
no longer needs to know about task names, payloads, repetition or constraints.
It asks for a wakeup and reports one. That is the whole contract, and it is why
this design is cheaper on the platform side despite owning more.

One deviation from the MediaHost rule must be documented at the call site:
`RunDueWork` runs handlers on **its own goroutine**, not the UI goroutine,
precisely because the UI goroutine may not be running. It is the only Bridge
entry point that does so.

- **Android** — a `Worker` subclass calling `RunDueWork`, plus WorkManager
  scheduling and manifest registration.
- **iOS** — `BGTaskScheduler.register` at launch, identifiers in `Info.plist`
  under `BGTaskSchedulerPermittedIdentifiers`, and `setTaskCompleted` on return.
  Also `beginBackgroundTask` on backgrounding for the grace-period drain, which
  in this design is not a separate feature but simply another wakeup source.

Both are template changes in `internal/cli/templates/mobile/`, which means the
`gophics` CLI must write the identifiers into `Info.plist` and
`AndroidManifest.xml`. That is the piece most likely to be underestimated: it is
build-system work, not API work.

### What the platforms cannot promise

The API must never resemble a ticker, because neither platform offers a
schedule.

**iOS is opportunistic and may never run.** Two common cases produce no
execution at all: the user force-quitting from the app switcher, after which iOS
stops scheduling refresh for that app until it is launched by hand; and Low
Power Mode or the per-app Background App Refresh switch.

**Android is steadier but not a clock.** WorkManager honours a 15-minute floor
for periodic work, Doze batches wakeups into maintenance windows, App Standby
buckets throttle rarely-used apps, and several OEM skins kill background work
aggressively.

The durable queue is the answer to all of it: work that cannot run now runs
later, and "later" including "at next launch" is acceptable for both jobs in §1.

### Push

For the commonest motivation — keep the data fresh — iOS's intended mechanism is
**silent push** (`content-available: 1`), not the scheduler. BGAppRefresh is for
prefetching around habitual use, and treating it as sync is the standard way
apps end up stale.

gophics has no push capability: `shell/notify.go` is local notifications only —
nothing registers for APNs or FCM tokens or handles a remote payload. Under this
design push is a wakeup source rather than an architecture, so it can be added
later without disturbing app code — but if the real requirement is *timely* data
rather than periodic work, push is roughly the same size and is the only
reliable answer on iOS, and should be built first.

---

## 6. Testing

The queue is pure Go and should carry the weight of the testing: enqueue
survives a simulated process death, at-least-once holds when killed between work
and acknowledgement, backoff is persisted, the deadline cancels a running
handler, `KeepFresh` deduplicates, and a handler is structurally unable to reach
the widget tree. None of that needs a device, which is the point.

Device testing needs the platform's own triggers, and they are awkward by
design:

- iOS: the debugger-only `_simulateLaunchForTaskWithIdentifier:` incantation,
  since the real scheduler may wait hours.
- Android: `adb shell cmd jobscheduler run -f <pkg> <jobid>`.

Both belong in the packaging README beside the 16 KB alignment note.

---

## 7. Order

1. **Phase A, lifecycle.** Small, self-contained, delivers most of the practical
   value on its own, and is the instrumentation for everything after it.
2. **The queue**, headless and fully tested, with app launch and foreground as
   its only wakeup sources. At this point background work already functions —
   durably — on every platform gophics supports, without a single platform
   scheduler.
3. **Wakeup sources**, cheapest first: backgrounding grace period, then
   WorkManager, then BGTaskScheduler, then push if the requirement is timely
   data.

Step 2 is what makes this design worth the extra ownership: it is shippable and
useful before any platform-specific work exists, and each wakeup source after it
is an independent improvement rather than a prerequisite.
