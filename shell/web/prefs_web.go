//go:build js && wasm

// Web implementation of the preferences capability (shell/prefs.go) using
// localStorage. Keys are namespaced with a prefix so an app's settings can be
// enumerated and cleared without disturbing anything else the page stores
// (including the SecureStorage capability, which shares localStorage here).
package web

import (
	"fmt"
	"sort"
	"strings"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// prefsPrefix namespaces preference keys within localStorage.
const prefsPrefix = "gophics.pref."

// Preferences returns a store only when localStorage is available (it isn't in
// some private-mode / sandboxed contexts).
func (w *window) Preferences() shell.Preferences {
	ls := js.Global().Get("localStorage")
	if ls.IsUndefined() || ls.IsNull() {
		return nil
	}
	return &webPrefs{ls: ls}
}

type webPrefs struct{ ls js.Value }

func (p *webPrefs) Get(key string) (string, bool) {
	v := p.ls.Call("getItem", prefsPrefix+key)
	if v.IsNull() {
		return "", false
	}
	return v.String(), true
}

func (p *webPrefs) Set(key, value string) (err error) {
	// setItem throws on quota-exceeded / disabled storage; a JS throw surfaces as
	// a Go panic, so recover it into an error rather than crashing the app.
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("web: prefs set %q: %v", key, r)
		}
	}()
	p.ls.Call("setItem", prefsPrefix+key, value)
	return nil
}

func (p *webPrefs) Delete(key string) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("web: prefs delete %q: %v", key, r)
		}
	}()
	p.ls.Call("removeItem", prefsPrefix+key)
	return nil
}

func (p *webPrefs) Keys() []string {
	n := p.ls.Get("length").Int()
	keys := make([]string, 0, n)
	for i := 0; i < n; i++ {
		k := p.ls.Call("key", i)
		if k.IsNull() {
			continue
		}
		if s := k.String(); strings.HasPrefix(s, prefsPrefix) {
			keys = append(keys, strings.TrimPrefix(s, prefsPrefix))
		}
	}
	sort.Strings(keys)
	return keys
}
