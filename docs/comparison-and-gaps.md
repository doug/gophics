# Gossamer vs Flutter / React Native — and what's missing

An honest assessment (2026-07-26) of where gossamer stands against the two
dominant cross-platform frameworks, driven by asking: *what would it take to
build real apps?*

## Why Flutter succeeds

1. **Everything is a widget.** One uniform composition model, deeply
   learnable. — *Gossamer has this.*
2. **Own-rendering pipeline.** Flutter draws every pixel itself (Skia/
   Impeller), so apps are pixel-identical on every platform and it isn't
   hostage to native widget quirks. — *Gossamer shares this architecture;
   it's the structural match and the hard part, already done.*
3. **Hot reload.** Sub-second edit→see-it iteration via the Dart VM. The
   single most-loved DX feature in the ecosystem. — *Gossamer cannot match
   this in Go (see PLAN §6.3); permanent gap, partially offset by fast
   rebuild + state snapshot.*
4. **A vast, polished widget catalog.** Material + Cupertino: hundreds of
   accessible, animated, themed widgets. You *assemble* an app. — *Gossamer
   has ~25 widgets. You still *build* primitives. This is the biggest
   feature gap and it's pure long-tail grind, not architecture.*
5. **Implicit animations & transitions.** `AnimatedContainer`, `Hero`,
   `AnimatedOpacity` — polish for near-free. — *Gossamer has explicit
   controllers only; no implicit-animation layer yet.*
6. **Layout depth.** Slivers, intrinsics, flex, wrap, grid, custom
   multi-child layout. — *Gossamer has flex + boxes; missing grid, wrap,
   responsive layout, intrinsics.*
7. **Tooling.** DevTools, widget inspector, performance overlay. — *None
   yet in gossamer; the semantics tree + offscreen render make an inspector
   very buildable.*
8. **Ecosystem.** pub.dev, thousands of packages. — *Gossamer inherits the
   Go ecosystem — strong for infra/networking/data, thin for UI.*

## Why React Native succeeds

1. **React + JavaScript.** Enormous existing developer pool and a familiar
   component/hooks mental model. — *Gossamer's audience is Go developers, a
   smaller but real pool that Flutter/RN don't serve well.*
2. **Native widgets.** RN renders *actual* platform components, so it looks
   native and inherits platform behavior/accessibility for free — the
   opposite trade from Flutter/gossamer (native feel vs. cross-platform
   consistency and control).
3. **Fast Refresh**, huge **npm** ecosystem, and **web-knowledge transfer**
   (flexbox, CSS-like styling).

## Gossamer's honest position

Gossamer has the **architecture** right (own-rendering pipeline, widget
composition, constraint layout, damage-tracked scenes, real text shaping,
four live platforms) — the part that's hard to get right. It lacks
**breadth** (the widget/animation/layout long tail) and **DX/tooling** (hot
reload, inspector). Breadth is "just work," incremental and unblocked.
DX is structural.

Its differentiators stand (PLAN §1.1): one static Go binary, embeddable in
any Go program, real goroutine concurrency, headless `go test`, no codegen,
and the Go infra ecosystem. It wins where Flutter is weakest — tools,
embedded UIs, server-side rendering — not where Flutter is strongest
(consumer mobile with a big design team).

## What real apps need — gap matrix

| Archetype | Needs | Have | Missing |
| --- | --- | --- | --- |
| **Chat** | grow-from-bottom list, bubbles, avatars, input, send | Scroll+fling, TextField, Decorated, Image | reverse/bottom-anchored list, circular image clip, network image, keyboard-avoid |
| **Settings / forms** | toggles, sliders, pickers, sections | rows, TextField, Card | **Switch, Checkbox, Slider, Radio, Dropdown**, Divider, **overlay/menu** |
| **Dashboard** | grids, cards, responsive layout | Flex, Card, Scroll | **Grid, Wrap, AspectRatio, LayoutBuilder (responsive)**, Spacer |
| **Media / photo grid** | image grid, async load, full-screen, refresh | Image, Navigator, LazyList | **network image (decode+cache)**, Grid, pull-to-refresh, hero transition, onEndReached |

## Cross-cutting gaps, prioritized

Done (2026-07-26):

1. ~~**Overlay system**~~ — `widget.OverlayHost` (auto-installed at the app
   root) + `theme.ShowDialog`/`ShowMenu`, dismiss on scrim/Escape.
2. ~~**Form controls**~~ — `theme.Switch/Checkbox/Slider/Radio`.
3. ~~**Layout primitives**~~ — `Grid`, `Wrap`, `AspectRatio`, `Fill`,
   `Stack`, `Spacer`. (`LayoutBuilder` for responsive still to do.)
4. ~~**Async / network images**~~ — `widget.NetworkImage` (decode + single-
   flight cache + placeholder/error). Surfaced and fixed two framework
   bugs: the loading→content reconciler swap and a mount-time Post race.

Done (cont.):

5. ~~**Scroll features**~~ — `Scroll`/`LazyList` `OnEndReached` (infinite
   feeds) + `ScrollController` (JumpTo/AnimateTo, live offset). Still to
   do: reverse/bottom-anchored, scrollbar, pull-to-refresh.
6. ~~**Implicit animations**~~ — `Animated[T]` + `AnimateColor/Insets/
   Float`, tween on value change. Still to do: hero transitions.

Done (cont.):

7. ~~**Gestures**~~ — `Handler.OnLongPress` (dt-driven, cancels tap).
   Still to do: double-tap, swipe/dismiss.
8. ~~**Text overflow**~~ — `MaxLines` + `Ellipsis` on Text/TextBox. Still
   to do: selectable static text.
11. ~~**Responsive**~~ — `LayoutBuilder` (state-based, one-frame settle).

Done (cont.):

10. ~~**Tooling**~~ — `Core.InspectTree` (render-tree dump), `Config.Debug`/
    `SetDebugPaint` (box-bounds overlay), `Core.FrameStats`/GOSSAMER_PACING
    (frame timing), and `Core.SetInspect` — an interactive widget inspector
    that highlights the box under the pointer and labels it with type + size
    (Flutter's inspector; `layout.DeepestAt`/`InspectOverlay`).
12. ~~Group opacity~~ (`Opacity` + `AnimateFloat` fades), ~~double-tap~~
    (deferred single-tap disambiguation).
9. ~~**Accessibility bridge**~~ — the semantics tree now flattens to a flat,
    ID-addressed `app.A11yNode` tree (`Core.A11yTree`/`A11yActivate`/
    `A11yHitTest`), exposed through `shell/mobile.Bridge` and consumed by
    Android's `AccessibilityNodeProvider` (virtual view hierarchy). Verified
    on device: TalkBack sees every HN story row as a clickable node with the
    right content description and bounds; `ACTION_CLICK` fires the widget's
    `OnActivate`. `SemInfo` carries `OnActivate`/`Checked`/`Disabled`/
    `Selected`/`Hint`. iOS `UIAccessibility` host landed too: `GossamerView`
    exposes `accessibilityElements` as `GossamerA11yElement`s (label/value/
    hint, screen-converted frames, `.button`/`.staticText` traits) built from
    the same `HnmobileA11y*` accessors, with `accessibilityActivate()` wired
    to `A11yActivate`. Builds and runs on the iOS simulator; full on-device
    VoiceOver inspection (the iOS parallel to Android's `uiautomator dump`)
    is the one remaining validation step.

Remaining:

13. **Follow-ups**: ~~pull-to-refresh~~ (Scroll/LazyList `OnRefresh`+
    `Refreshing`, rubber-band overscroll, spoke spinner; wired into the HN
    feed). ~~swipe-to-dismiss~~ (`Dismissible`: finger-follow, threshold/
    flick trigger, spring-back, revealed background panel; wired into the
    todo example). ~~reverse/bottom-anchored lists~~ (`Scroll.Reverse` +
    `LazyList.Reverse`: end-anchored offset origin, so newest-at-bottom and
    stay-pinned-on-append fall out for free; virtualized; OnEndReached fires
    at the oldest end for history loading — the chat-log layout).
    ~~selectable static text~~ (`SelectableText`: drag-select with glyph-
    midpoint hit testing, highlighted range, Cmd/Ctrl+C copy joining wrapped
    lines). ~~visual inspector UI~~ (`Core.SetInspect`, see tooling above).
    ~~hero transitions~~ (see below). Still: on-device VoiceOver validation
    of the iOS a11y host.

**Hero transitions — landed.** First added the prerequisite: a Canvas
affine transform (`paint.Canvas.PushTransform`/`PopTransform`, `Transform`,
`MapRect`) on the gg backend, recorded by the scene layer as a full-repaint
layer op (like opacity groups, since a transform reshapes every inner op's
bounds), with `layout.Transformed`/`widget.Transform` exposing it. On top,
`widget.Hero{Tag, Child}` registers its painted rect into a per-page
`heroRegistry` provided by the Navigator during a transition; the Navigator
recovers each hero's at-rest rect (undoing the page slide via a paint-time
`heroPageW` that records the slide fraction alongside the rects), suppresses
both real heroes, and flies an overlay copy from source rect to destination
rect with `MapRect` (size + position interpolated). Every page is wrapped in
a stable `Provide[*heroRegistry]` so pages keep their state across role
changes. Verified headless via pixel checks: the element flies center→corner
on push and returns on pop, and non-hero navigation still preserves feed
state.

All 12 originally-identified cross-cutting gaps are now addressed — the
accessibility bridge landed on both platforms (Android verified on device
via `uiautomator dump`, iOS building and running with the same Go a11y
surface). What remains is a polish tail. Gossamer now has the breadth to
build the four app archetypes end to end.
