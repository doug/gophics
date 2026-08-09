//go:build js && wasm

// Web implementation of the shell links capability (shell/links.go).
//
// Initial() is the current page address (location.href) — the URL the app was
// loaded with, deep-link query/hash included. OnLink fires on in-page history
// navigation (hashchange for "#/route" changes, popstate for History API
// back/forward and pushState-based routers), reporting the new location.href.
// location is available in every browser, so this capability is always returned
// on web. The listeners live for the page lifetime, so their js.Funcs are not
// released.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Links makes the web window a shell.LinksWindow.
func (w *window) Links() shell.Links {
	return &webLinks{initial: href()}
}

type webLinks struct {
	initial string
	cbs     []func(string)
	wired   bool // hashchange/popstate listeners registered lazily on first OnLink
}

func (l *webLinks) Initial() string { return l.initial }

func (l *webLinks) OnLink(f func(string)) {
	if f == nil {
		return
	}
	l.cbs = append(l.cbs, f)
	if l.wired {
		return
	}
	l.wired = true
	win := js.Global().Get("window")
	if win.IsUndefined() {
		return
	}
	// One listener pair fans out to every registered callback with the current
	// address. hashchange covers "#/route" routers; popstate covers History API
	// navigation (back/forward and pushState-based routers).
	fire := js.FuncOf(func(js.Value, []js.Value) any {
		u := href()
		for _, cb := range l.cbs {
			cb(u)
		}
		return nil
	})
	win.Call("addEventListener", "hashchange", fire)
	win.Call("addEventListener", "popstate", fire)
}

// href reads location.href, or "" when location is unavailable.
func href() string {
	loc := js.Global().Get("location")
	if loc.IsUndefined() || loc.IsNull() {
		return ""
	}
	if h := loc.Get("href"); h.Type() == js.TypeString {
		return h.String()
	}
	return ""
}
