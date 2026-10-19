//go:build js && !gophics_term

package app

import (
	"encoding/json"
	"log"
	"syscall/js"
)

// Web hot restart.
//
// `gophics dev -p web` rebuilds the wasm and the tab reloads, and until this
// file existed that dropped every scroll offset, open page and half-typed
// field — on the platform with the fastest rebuild, where the loss is felt
// most often. The documentation claimed otherwise; it was wrong.
//
// The hand-off is sessionStorage rather than a file, because a wasm app has no
// filesystem it shares with its successor but it does keep sessionStorage
// across a reload. The lifetime is also exactly right: per tab, so two dev
// tabs do not fight, and gone when the tab closes.
//
// sessionStorage is the gate as well. A shipped app has no way to turn this
// on: the marker key is written by the reload client the dev server injects
// into index.html (internal/cli/sse.go) and by nothing else, so a wasm build
// served from anywhere else finds no marker and never touches storage. The
// injected script runs while the document is parsed, long before the
// WebAssembly.instantiateStreaming promise that calls main can resolve — and
// because the marker survives the reload, even a first load that somehow lost
// that race is only one reload behind.
const (
	devMarkerKey = "gophics:dev"      // set by the dev server's reload client
	devStateKey  = "gophics:devstate" // the snapshot itself
)

// setupDevState restores the snapshot the previous page left behind and
// arranges for the next unload to leave one.
//
// The snapshot is taken on pagehide rather than on a message from the dev
// server, which costs no JS-to-Go protocol and has the side benefit of
// covering a reload the developer triggers by hand. pagehide rather than
// beforeunload because a browser that freezes a page instead of unloading it
// fires the former and skips the latter.
func setupDevState(sh *shellHandler) {
	store, ok := devStorage()
	if !ok {
		return
	}
	if v, ok := devGetItem(store, devMarkerKey); !ok || v != "1" {
		return // not served by `gophics dev`
	}
	if v, ok := devGetItem(store, devStateKey); ok {
		devRemoveItem(store, devStateKey) // a hand-off is read once
		restoreDevSnapshotBytes(sh, []byte(v))
	}

	// Never Released: the callback has to outlive setupDevState and lives as
	// long as the page does, which is also when the Go runtime stops.
	cb := js.FuncOf(func(js.Value, []js.Value) any {
		snap := sh.core.Owner.SnapshotState()
		if len(snap) == 0 {
			return nil
		}
		data, err := json.Marshal(snap)
		if err != nil {
			log.Printf("gophics dev: snapshot state: %v", err)
			return nil
		}
		devSetItem(store, devStateKey, string(data))
		return nil
	})
	js.Global().Call("addEventListener", "pagehide", cb)
}

// Every sessionStorage call below is recovered, because every one of them
// throws rather than returning an error in a tab with site data blocked, in a
// sandboxed iframe, and in a few private-browsing modes. A dev convenience
// must not be able to take the app down, and the app has to render correctly
// without it.

func devStorage() (v js.Value, ok bool) {
	defer func() {
		if recover() != nil {
			v, ok = js.Undefined(), false
		}
	}()
	s := js.Global().Get("sessionStorage")
	if !s.Truthy() {
		return js.Undefined(), false
	}
	return s, true
}

// devGetItem reports ok=false for a key that is absent, which sessionStorage
// answers with null — distinguished by type, since a null js.Value stringifies
// to "<null>" rather than "".
func devGetItem(store js.Value, key string) (s string, ok bool) {
	defer func() {
		if recover() != nil {
			s, ok = "", false
		}
	}()
	v := store.Call("getItem", key)
	if v.Type() != js.TypeString {
		return "", false
	}
	return v.String(), true
}

func devSetItem(store js.Value, key, val string) {
	defer func() {
		if r := recover(); r != nil {
			// A quota failure is the likely one, and it means this reload
			// loses state rather than that something is broken.
			log.Printf("gophics dev: store state: %v", r)
		}
	}()
	store.Call("setItem", key, val)
}

func devRemoveItem(store js.Value, key string) {
	defer func() { _ = recover() }()
	store.Call("removeItem", key)
}
