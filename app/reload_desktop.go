//go:build !js && !android && !ios && !gossamer_term

package app

import (
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/widget"
)

// RunReloadable runs the app like Run, but rebuilds the root from build() each
// time a new builder arrives on the reload channel — the hot-reload boundary
// behind `gossamer dev --hot`. The element tree and framework-managed State
// survive the rebuild.
//
// Caveat (Go, not gossamer): app-defined State survives a reload only while its
// widget types keep their identity. An in-process swap preserves it; a Go
// plugin swap recompiles the app's types, giving them new identities, so the
// reconciler treats them as new and app State resets. Framework/host state
// (and everything the app keeps outside its widget State) persists.
//
// TODO(hot-reload, if we ever need true state preservation): drive this same
// boundary from a Go interpreter (e.g. Yaegi) instead of a plugin — the UI code
// runs in a persistent VM whose type identities are stable across reloads, so
// app State would survive, like Flutter. Deferred deliberately: Yaegi is a
// heavy dependency and slower than compiled code, against the minimal-deps
// stance. The boundary here is the integration point when that trade-off is
// worth making; nothing else would change.
func RunReloadable(build func() widget.Widget, cfg Config, reload <-chan func() widget.Widget) error {
	cell := &reloadCell{build: build}
	h, err := NewHandler(reloadHost{cell}, cfg)
	if err != nil {
		return err
	}
	sh := h.(*shellHandler)
	go func() {
		for b := range reload {
			next := b
			sh.core.Post(func() {
				cell.build = next
				sh.core.Owner.RebuildAll()
			})
			sh.core.Owner.RequestFrameThreadSafe()
		}
	}()
	return desktopRun(h, shell.Config{Title: cfg.Title, Size: cfg.Size, Resizable: true})
}
