//go:build !js && !android && !ios && !gossamer_term

package app

import (
	"github.com/doug/gossamer/shell"
	"github.com/doug/gossamer/widget"
)

// RunReloadable runs the app like Run, but rebuilds the root from build() each
// time a new builder arrives on the reload channel. The element tree and
// framework-managed State survive the rebuild; it is the in-process hot-reload
// boundary.
//
// Reserved, not currently CLI-wired. The former `gossamer dev --hot` drove this
// from Go plugins, which does NOT work: Go's plugin loader refuses to load a
// second plugin containing a different version of an already-loaded package
// ("plugin was built with a different version of package X") — precisely what an
// edited UI package is. So plugin reload is retired; the desktop dev loop is
// rebuild + hot-restart (`gossamer dev -p desktop`), and the fast loop is web
// live-reload (`gossamer dev -p web`).
//
// This boundary stays because it is the integration point for the one approach
// that could deliver true, state-preserving reload: drive build() from a Go
// interpreter (e.g. Yaegi) instead of a plugin. The UI code would run in a
// persistent VM whose type identities are stable across edits, so app State
// would survive, like Flutter. Deferred deliberately — Yaegi is a heavy
// dependency, slower than compiled code, and (the real risk) its generics
// support is weak, which gossamer's generic State/Provide API leans on hard.
// Nothing about the GPU stack is in the way: the framework stays compiled and
// is called through bindings; only the app's plain-Go UI code is interpreted.
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
