# Health app — native providers (HealthKit / Health Connect)

Phase 2 of the health showcase: feed **real device data** into the same Go
widget tree that runs on desktop and web. Health stores are **on-device only**
(no cloud API for Apple Health; Health Connect is on-device Android), so this is
inherently the mobile build's story — one Go UI, pixel-identical, real native
data on the phone.

## Architecture

```
              ┌─────────────────────────────────────────┐
   HealthKit  │  native host (Swift / Kotlin)            │
   Health     │  · request permission                    │
   Connect  ──┼─▶ read history + observe live samples    │
              │  · call the gomobile bind:               │
              │       PushSample + SetAuthorized          │
              └───────────────┬─────────────────────────┘
                              ▼
        examples/health/mobile   (gomobile bind, healthmobile pkg)
                              ▼
        healthui.DeviceProvider  (thread-safe, mutex-guarded)
                              ▼
        healthui.App  ── the SAME widget tree as desktop/web
```

The Go side is **done and tested** (`examples/health/ui`, `.../mobile`): the app
takes a `Provider`; `DeviceProvider` receives pushes; the bind exposes
`Start`, `PushSample`, `SetAuthorized`, `Touch`, and the render bridge. The
**Android host is built and verified** at `examples/health/android` (real Health
Connect steps on a Pixel 10 Pro — see below); the iOS host still needs building.
The hosts mirror the working ones in `examples/hn/ios` and `examples/hn/android`.

**gomobile can't bind a `[]float64` parameter** (only `[]byte`), so there is no
batch `PushSeries` — a whole series is backfilled by calling the scalar
`PushSample` in a loop, oldest→newest (the provider is fresh each `Start`).

## Metric mapping

| `healthui.Metric` | code | HealthKit | Health Connect | unit pushed |
| --- | --- | --- | --- | --- |
| `HeartRate` | 0 | `HKQuantityType(.heartRate)` | `HeartRateRecord` | bpm |
| `Steps` | 1 | `HKQuantityType(.stepCount)` | `StepsRecord` | cumulative count |
| `Weight` | 2 | `HKQuantityType(.bodyMass)` | `WeightRecord` | kg |
| `Sleep` | 3 | `HKCategoryType(.sleepAnalysis)` | `SleepSessionRecord` | hours |

`PushSample(m, t, v, capN)` — `t` is the metric-relative x used by the charts
(seconds for live HR, days-ago for weight/sleep, hour for steps); `capN` bounds
retained history (e.g. 60 for the HR window, 0 = keep all). Loop it oldest→newest
to backfill a whole range.

## Build the bind artifacts

```sh
go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init
# iOS framework:
gomobile bind -target=ios     -o examples/health/ios/Healthmobile.xcframework       ./examples/health/mobile
# Android library:
gomobile bind -target=android -androidapi 26 -o examples/health/android/app/libs/healthmobile.aar ./examples/health/mobile
```

The generated symbol names come from gomobile (Go `Start` → Swift
`HealthmobileStart`, Kotlin `Healthmobile.start`). Adjust the calls below to
match what the binding emits.

## iOS — HealthKit

1. **Capability + usage string.** Add the *HealthKit* capability to the target
   (writes `com.apple.developer.healthkit` to the entitlements) and add to
   `Info.plist`:
   ```xml
   <key>NSHealthShareUsageDescription</key>
   <string>Health shows your heart rate, steps, weight, and sleep.</string>
   ```

2. **Reader.** In the Swift host (alongside `GophicsApp.swift` from the hn iOS
   template), request authorization, backfill history, and observe the live
   heart rate. Illustrative — verify on device:
   ```swift
   import HealthKit

   final class HealthReader {
       let store = HKHealthStore()
       let hr    = HKQuantityType(.heartRate)
       let steps = HKQuantityType(.stepCount)
       let mass  = HKQuantityType(.bodyMass)
       let sleep = HKCategoryType(.sleepAnalysis)

       func start() {
           let read: Set = [hr, steps, mass, HKObjectType.categoryType(forIdentifier: .sleepAnalysis)!]
           store.requestAuthorization(toShare: [], read: read) { ok, _ in
               HealthmobileSetAuthorized(ok)
               guard ok else { return }
               self.backfillWeight()
               self.observeHeartRate()
               // …steps (statistics query, cumulative today), sleep (category samples)…
           }
       }

       // 30 days of weight → PushSample(2, daysAgo, kg, 0) per reading (oldest
       // first; no batch PushSeries — gomobile can't bind a []float64).
       func backfillWeight() {
           let cal = Calendar.current
           let start = cal.date(byAdding: .day, value: -30, to: Date())!
           let q = HKSampleQuery(sampleType: mass,
                                 predicate: HKQuery.predicateForSamples(withStart: start, end: Date()),
                                 limit: HKObjectQueryNoLimit,
                                 sortDescriptors: [NSSortDescriptor(key: HKSampleSortIdentifierStartDate, ascending: true)]) { _, samples, _ in
               for s in (samples as? [HKQuantitySample] ?? []) {
                   let daysAgo = -Date().timeIntervalSince(s.startDate) / 86_400
                   HealthmobilePushSample(2, daysAgo,
                       s.quantity.doubleValue(for: .gramUnit(with: .kilo)), 0) // Metric.Weight
               }
           }
           store.execute(q)
       }

       // Live bpm via an anchored query that keeps delivering updates.
       func observeHeartRate() {
           let bpm = HKUnit.count().unitDivided(by: .minute())
           var t = 0.0
           let q = HKAnchoredObjectQuery(type: hr, predicate: nil,
                                         anchor: nil, limit: HKObjectQueryNoLimit) { _, samples, _, _, _ in
               for s in (samples as? [HKQuantitySample] ?? []) {
                   HealthmobilePushSample(0, t, s.quantity.doubleValue(for: bpm), 60) // Metric.HeartRate, cap 60
                   t += 1
               }
           }
           q.updateHandler = q.resultsHandler
           store.execute(q)
       }
   }
   ```
   Call `HealthmobileStart("Apple Health")` first (from the host's launch), then
   `HealthReader().start()`.

3. **Simulator note.** The iOS Simulator has no HealthKit data and no GPU present
   (see [mobile-gpu-bringup.md](mobile-gpu-bringup.md)); use the desktop/web
   build with the synthetic provider for UI iteration, and a real iPhone for the
   HealthKit path.

## Android — Health Connect

1. **Dependency** (`app/build.gradle`):
   ```gradle
   implementation "androidx.health.connect:connect-client:1.1.0-alpha07"
   ```

2. **Manifest permissions** (`AndroidManifest.xml`) — declare the read
   permissions and the rationale activity Health Connect requires:
   ```xml
   <uses-permission android:name="android.permission.health.READ_HEART_RATE"/>
   <uses-permission android:name="android.permission.health.READ_STEPS"/>
   <uses-permission android:name="android.permission.health.READ_WEIGHT"/>
   <uses-permission android:name="android.permission.health.READ_SLEEP"/>
   <!-- plus the <activity-alias> for ACTION_SHOW_PERMISSIONS_RATIONALE -->
   ```

3. **Reader.** In the Kotlin host (alongside the hn Android template), after the
   permission grant, backfill and read. Illustrative — verify on device:
   ```kotlin
   import androidx.health.connect.client.HealthConnectClient
   import androidx.health.connect.client.records.*
   import androidx.health.connect.client.request.ReadRecordsRequest
   import androidx.health.connect.client.time.TimeRangeFilter
   import java.time.Instant

   suspend fun pump(client: HealthConnectClient) {
       Healthmobile.setAuthorized(true)

       // 30 days of weight → pushSample(2, daysAgo, kg, 0) per reading, oldest
       // first (no batch pushSeries — gomobile can't bind a []float64).
       val now = Instant.now()
       client.readRecords(
           ReadRecordsRequest(WeightRecord::class,
               TimeRangeFilter.between(now.minusSeconds(30L * 86_400), now))
       ).records.sortedBy { it.time }.forEach {
           val daysAgo = -(now.epochSecond - it.time.epochSecond) / 86_400.0
           Healthmobile.pushSample(2, daysAgo, it.weight.inKilograms, 0) // Metric.Weight
       }

       // Latest heart-rate samples → pushSample(0, t, bpm, 60).
       var t = 0.0
       client.readRecords(
           ReadRecordsRequest(HeartRateRecord::class,
               TimeRangeFilter.between(now.minusSeconds(60), now))
       ).records.forEach { rec ->
           rec.samples.forEach { s ->
               Healthmobile.pushSample(0, t, s.beatsPerMinute.toDouble(), 60)
               t += 1.0
           }
       }
       // …steps (StepsRecord, aggregate today), sleep (SleepSessionRecord duration)…
   }
   ```
   For a live feed, poll `readRecords` on a timer or use Health Connect's
   changes API; call `Healthmobile.start("Health Connect")` at launch. The full
   working host — the render/surface/touch plumbing, the permission flow, and the
   reader for all four metrics — is committed at `examples/health/android` (build
   + run it with `gophics run -p android ./examples/health/mobile`).

## Status

- **Done + tested (Go):** the `Provider` seam, `DeviceProvider`, the gomobile
  bind surface, and unit/-race tests. Builds native, `-tags nogpu`, and js/wasm.
- **Android host — DONE + device-verified.** `examples/health/android` reads
  Health Connect and renders on the GPU; confirmed on a Pixel 10 Pro showing real
  steps (10,991 today from 282 step records). Heart rate / weight / sleep read
  correctly too — they just show "no data" unless a watch/scale/sleep-tracker
  feeds Health Connect. Requires the user to grant the Health Connect permission
  sheet at first launch.
- **iOS host — still to build:** the HealthKit reader above, the entitlement +
  usage string, and `gomobile bind -target=ios`. The Simulator lacks health data
  (fall back to the synthetic provider); best iterated on an iPhone.
