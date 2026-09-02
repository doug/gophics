//go:build js && wasm

// Handle storage for the folder capability (shell/folder.go).
//
// A FileSystemDirectoryHandle cannot be written down. It is not a path and has
// no string form, so "remember the folder" has to mean "keep the object", and
// IndexedDB is the only store that takes one — it is structured-cloneable but
// not JSON, so localStorage and the Preferences capability are both out.
//
// IndexedDB predates promises and reports through onsuccess/onerror events, so
// this is callback-shaped for a second reason on top of the one in
// shell/folder.go: there is no promise here to await even if awaiting were free.
package web

import (
	"errors"
	"syscall/js"
)

const (
	folderDBName  = "gophics"
	folderDBStore = "folders"
)

// withFolderStore opens the database and hands fn an object store in a
// transaction of the given mode.
//
// The database is opened per call rather than cached. These operations happen
// when a user picks or reopens a folder — twice a session, not twice a frame —
// and a cached handle would have to deal with the connection being closed out
// from under it by a version change in another tab.
func withFolderStore(mode string, fn func(store js.Value, fail func(error)), onErr func(error)) {
	idb := js.Global().Get("indexedDB")
	if !idb.Truthy() {
		onErr(errors.New("web: indexedDB unavailable"))
		return
	}
	req := idb.Call("open", folderDBName, 1)

	var upgrade, success, failure js.Func
	release := func() { upgrade.Release(); success.Release(); failure.Release() }

	upgrade = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		db := req.Get("result")
		if !db.Get("objectStoreNames").Call("contains", folderDBStore).Bool() {
			db.Call("createObjectStore", folderDBStore)
		}
		return nil
	})
	failure = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		release()
		onErr(errors.New("web: indexedDB open failed"))
		return nil
	})
	success = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		release()
		db := req.Get("result")
		tx := db.Call("transaction", folderDBStore, mode)
		fn(tx.Call("objectStore", folderDBStore), onErr)
		return nil
	})

	req.Set("onupgradeneeded", upgrade)
	req.Set("onsuccess", success)
	req.Set("onerror", failure)
}

// idbPut stores handle under key.
func idbPut(key string, handle js.Value, done func(error)) {
	withFolderStore("readwrite", func(store js.Value, fail func(error)) {
		onRequest(store.Call("put", handle, key), func(_ js.Value, err error) { done(err) })
	}, done)
}

// idbGet loads the handle stored under key. A missing key yields an undefined
// value and no error — IndexedDB reports "not there" as a successful read of
// undefined, and so does this.
func idbGet(key string, done func(js.Value, error)) {
	withFolderStore("readonly", func(store js.Value, fail func(error)) {
		onRequest(store.Call("get", key), done)
	}, func(err error) { done(js.Value{}, err) })
}
