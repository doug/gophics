// Package widget implements gophics's declarative layer: immutable widget
// values describing the UI, reconciled into a retained element tree that owns
// layout boxes. It is Flutter's widgets/ analog (PLAN.md M3, §4).
//
// The three widget kinds (mirroring Flutter, expressed as Go interfaces):
//
//   - Stateless: Build(ctx) describes content from configuration alone.
//   - Stateful: CreateState() yields mutable State whose Build runs on
//     SetState; state survives reconciliation while the widget type and key
//     match.
//   - render widgets (Text, Padding, Row, ...): bridge to layout.Box render
//     objects via the unexported renderWidget interface.
//
// Widgets are compared by concrete type and key during reconciliation.
// Attach a key with WithKey to preserve element/state identity across list
// reorders.
//
// # Package structure: reconciler core vs widget catalog
//
// The package is one import path but two layers:
//
//   - The reconciler core (element.go): Owner, the element tree, the
//     build/reconcile machinery, the renderWidget bridge, and the internals
//     behind Ctx. This is the engine; nothing here is widget-specific.
//   - The widget catalog (basic.go, layout_widgets.go, scroll/text/overlay/
//     navigator/... files): the built-in widgets, each a thin value type that
//     the core reconciles into render objects.
//
// The catalog depends on the core, never the reverse.
//
// # Extension policy (sealed)
//
// The render-object bridge is deliberately closed: renderWidget is unexported,
// so only this package defines widgets that map directly to layout boxes.
// This is policy, not a gap — it keeps layout/paint/hit-test invariants
// (constraints protocol, ink bounds, damage recording) inside one audited
// package. The supported extension points are:
//
//   - Composition: Stateless/Stateful widgets compose the catalog; this is
//     the primary way to build apps and covers almost everything.
//   - Custom painting: Canvas hands user code a paint.Canvas each frame.
//   - Input: Interactive attaches a Gestures (taps, drags, keys, focus) to
//     any subtree.
//   - Gesture dispatch: GestureTarget is the sealed seam through which the
//     app runner delivers pointer gestures; *InteractiveBox is its only
//     implementation, and the unexported marker method keeps it that way.
package widget
