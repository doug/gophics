//go:build js && wasm

package main

import "syscall/js"

const lsKey = "gossamer-solitaire"

// lsStore autosaves the game to the browser's localStorage.
type lsStore struct{}

func platformStore() store { return lsStore{} }

func (lsStore) save(data []byte) {
	if ls := js.Global().Get("localStorage"); ls.Truthy() {
		ls.Call("setItem", lsKey, string(data))
	}
}

func (lsStore) load() ([]byte, bool) {
	ls := js.Global().Get("localStorage")
	if !ls.Truthy() {
		return nil, false
	}
	v := ls.Call("getItem", lsKey)
	if !v.Truthy() {
		return nil, false
	}
	return []byte(v.String()), true
}
