# ADR 0001: float32 geometry in logical pixels

**Status**: accepted · **Date**: 2026-07-23

## Decision

All geometry (`geom` package and everything above it) uses `float32`, in
logical pixels. Physical device pixels appear only at the shell/gpu boundary
via a scale factor.

## Context

Flutter uses `double` (Dart has no 32-bit float). Go gives us the choice.

## Rationale

- The entire substrate is float32: gogpu/wgpu, gg, WGSL shader interfaces,
  and gophics's `Vec2`. Converting at every boundary is noise and cost.
- Gio and Cogent Core both settled on float32 for the same reason.
- Precision is sufficient for the domain: float32 has 24 mantissa bits —
  exact integers to 16.7M, ~0.001px resolution at coordinate 10,000. UI
  coordinates live within a window's bounds; accumulated-error risk sits in
  *transform chains*, not stored coordinates.

## Consequences

- Deep transform compositions (nested rotations in animations) should
  compose from authoritative values per frame rather than incrementally
  mutating a matrix; document this in `anim` when it exists.
- Layout arithmetic that Flutter does in double (flex distribution,
  intrinsics) inherits float32 rounding. Flutter's own layout tests will
  surface any case where this actually matters (M2); revisit only on
  evidence.
- If a real case emerges, the escape hatch is float64 *inside* one
  computation, float32 at rest — never a type change in `geom`.
