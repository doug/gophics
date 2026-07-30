package app

import "github.com/doug/gossamer/widget"

// reloadCell holds the current root builder for the hot-reload boundary. It is
// swapped in place, so a rebuild picks up new code without replacing the
// element tree above it.
type reloadCell struct{ build func() widget.Widget }

// reloadHost is a stateless widget whose Build delegates to the cell. Because
// it sits just under the root, a rebuild re-runs the latest builder while the
// element tree — and every mounted State — persists across the swap. This is
// the same shape as Flutter's hot reload: re-run build() on the existing tree.
type reloadHost struct{ cell *reloadCell }

func (r reloadHost) Build(widget.Ctx) widget.Widget { return r.cell.build() }
