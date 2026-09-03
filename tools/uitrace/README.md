# uitrace — measuring feel instead of describing it

gophics's scroll physics constants were annotated *"tuned to NSScrollView;
feel-test"*. This tool replaces the feel-test with a curve.

## How it works

**Record once, replay everywhere.** A gesture is a list of finger deltas with
timestamps. A native twin records one real flick and the offset its scroll view
showed per frame through the momentum. gophics is handed the identical finger
deltas through `app.Headless`, steps its frame clock at the same rate, and
records its own offset per frame. Same input in, two curves out.

`trace.Metrics` reduces a curve to numbers — release velocity, decay time
constant (least-squares fit of ln|v|, with R² so a non-exponential curve shows
itself), settle time, momentum distance — and `uitrace compare` prints two
traces side by side.

```
uitrace fling   [-v0 -2400] [-dur 0.1] [-hz 120] [-physics ios|android] [-out out] [-frames] [-video]
uitrace replay  [-hz 120] [-out out] [-frames] [-video] trace.json
uitrace compare a.json b.json
```

Every run writes `trace.json` (the contract below), `offsets.csv` (t, offset,
velocity — for a spreadsheet or a plot), and `metrics.txt`. `-video` adds the
frames and an mp4 (ffmpeg) or GIF (standard library) — the curve you can watch.

## The trace contract

`trace.Trace`, as JSON. A native twin writes `source`, `hz`, `input`,
`offset`, `release_t`. The sign convention is the only hard part: **input
deltas are finger movement in screen coordinates** (an upward flick is
negative), **offset is scroll position** (increasing as content moves up). An
upward flick therefore produces an increasing offset.

```json
{
  "source": "macos-appkit",
  "hz": 120,
  "notes": "MacBook Pro 14 2023, macOS 15.6, trackpad, one flick",
  "input":  [{"t": 0.0083, "v": -18.0}, {"t": 0.0167, "v": -21.5}, ...],
  "offset": [{"t": 0.0000, "v": 0.0},   {"t": 0.0083, "v": 18.0}, ...],
  "release_t": 0.1084
}
```

`input` events carry their own timestamps because real input arrives in bursts
and is not frame-aligned; replay delivers each event before the first frame at
or past its time, which is how a real shell does it too.

## What it has found

The first trace it produced showed the release frame moving 38px between
neighbours of 20 and 17: the finger's last delta and the first momentum step
landed in the same frame, a ~1.9× kick at every release. Fixed in
`widget/scroll.go` (`flinger.fresh`), with `kick_test.go` holding the bound.

It also confirmed that gophics's fling decays with τ = 0.499s at R² = 1.000 —
exactly what `flingFriction = 2.0` declares — which is the measurement chain
validating itself before it is pointed at anything native.

### The first native recording (macOS 26.6, trackpad, one upward flick)

|                   | macOS      | gophics    |
|-------------------|------------|------------|
| decay τ           | 0.186 s (R² 0.954) | 0.498 s (R² 0.999) |
| settle time       | 0.79 s     | 2.28 s     |
| fling start       | ~3800 px/s (measured) | 1927 px/s (fit, matches measured) |
| momentum distance | 1005 px    | 941 px     |

Same finger input. gophics's fling starts at half Apple's speed and decays at a
third the rate, and the two errors cancel into almost the same travel — the
content lands in the same place and takes three times as long to get there.
That is the "very slow end of the flick" a user feels before anyone can name
it. The half-speed start is the velocity estimator: an EMA with a 40ms time
constant over a four-event, 84ms finger phase has not converged, and macOS
evidently uses something closer to the last instantaneous velocity.

Apple's curve is also not a single exponential: R² 0.95–0.96 on two gestures,
τ 0.178 and 0.186, decaying faster late than early — which is why an
exponential fit's intercept (5984) overshoots the measured start, and why the
table reports the measured value for macOS.

A macOS recording is the closest reference available without a device, and
it is informational: gophics's fling runs on touch platforms, and the iOS twin
is the reference that binds. Whether a touch fling should feel like a trackpad
flick is a product decision the numbers now make possible to have.

### The iOS recording (iOS 26.5 Simulator, iPhone 17 Pro, one upward flick)

The reference that binds — gophics's fling runs on touch platforms. Same
finger input:

|                   | iOS UIKit  | gophics, 40ms EMA | gophics, 20ms EMA |
|-------------------|------------|-------------------|-------------------|
| decay τ           | 0.518 s    | 0.502 s           | 0.502 s           |
| settle time       | 2.77 s     | 2.78 s            | 2.78 s            |
| fling start (fit) | 6061 px/s  | 5063 (−16%)       | 5700 (−6%)        |
| momentum distance | 3348 px    | 2563 (−23%)       | 2880 (−14%)       |

**The decay constant is right on its home platform.** UIKit's documented
0.998/ms is real: τ 0.518 measured against 0.502 declared, settle times within
20ms. What was wrong was the hand-off: the velocity estimator, a 40ms EMA,
had not converged by the end of a 130ms finger phase and started the fling
16% slow, so it travelled 23% short. Sweeping the real estimator through the
real replay put 20–25ms inside the harness's bands; `velocityTau` is now 20ms.

The residual is UIKit's own trick. The recording shows the finger slowing
before it lifts (138 → 84 → 2 px per frame), two still frames, then momentum
starting at 6,600 px/s — almost exactly the mean of the last two *moving*
frames. UIKit ignores the slowdown of a hand leaving the glass; an EMA cannot,
and that is the next refinement if a device recording asks for it. iOS is not
a perfectly clean exponential either (R² 0.943), mostly from that two-frame
hold at release.

### Platform physics

The twins settled it: an iPhone decays exponentially at τ ≈ 0.5s, a Mac
trackpad at τ ≈ 0.19s and not quite exponentially, and Android's `OverScroller`
is a third model — a position spline whose duration and distance grow with the
log of the release velocity. A user's reference for "native" is the device in
their hand, so `shell.ScrollPhysics` carries the curve: the mobile bridge picks
it from `GOOS`, the web shell from the user agent, and `app.Config.ScrollPhysics`
pins one for an app that wants a single identity everywhere. macOS keeps
passing the OS's own momentum through. `-physics android` replays a recording
under the spline; an Android twin (an emulator, the same contract) is what
would make that reference bind.

## Where the constants actually come from

The comment on `flingFriction` cites NSScrollView, but 0.998 per millisecond is
`UIScrollView.DecelerationRate.normal` — a UIKit value. On a Mac trackpad the
desktop shell passes the OS's own momentum events through, so these constants
matter for *touch*: mobile and web-touch, where there is no OS momentum and
gophics imitates Apple's curve itself. The native twin settles which curve a
real flick follows.

## Phases

1. **This tool** — the gophics side. Done.
2. **macOS AppKit twin** (`tools/native-twin/macos`) — a Swift `NSScrollView`
   that logs finger-phase deltas and per-frame offsets to the contract above;
   recorded traces become testdata; a Go test compares curves with tolerance.
3. **iOS twin** (`tools/native-twin/ios`) — a UIKit `UIScrollView` in the
   Simulator, same contract, printed through `simctl launch --console`. Done;
   its recording is the binding reference.
4. **Other dimensions** — gesture thresholds (tap slop, long-press,
   drag-start), then text selection behavior.
