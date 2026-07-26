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

Remaining:

7. **Gestures**: long-press, double-tap, swipe/dismiss.
8. **Text**: selectable static text, overflow ellipsis, max lines.
9. **Accessibility bridge**: wire the existing semantics tree to
   VoiceOver/TalkBack/AccessKit.
10. **Tooling**: widget inspector (semantics tree → browser), perf overlay.
11. **Responsive**: `LayoutBuilder` (build from constraints) — one-frame
    state-based version is easy; true layout-time build is the Flutter way.
