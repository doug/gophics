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
	"fmt"
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
	// Read through globalProp and open under recover: where IndexedDB is
	// denied (an opaque origin, storage blocked in a sandboxed frame) the
	// property getter and open() both throw SecurityError instead of
	// reporting through onerror.
	idb := globalProp("indexedDB")
	if !idb.Truthy() {
		onErr(errors.New("web: indexedDB unavailable"))
		return
	}
	req, err := idbOpen(idb)
	if err != nil {
		onErr(err)
		return
	}

	var upgrade, success, failure, blocked js.Func
	release := func() { upgrade.Release(); success.Release(); failure.Release(); blocked.Release() }
	// One outcome reaches the caller. onblocked can precede onsuccess — the
	// request goes ahead once the blocking connection closes — so a blocked
	// request is reported at once and the late success is then dropped.
	settled := false
	settle := func() bool {
		if settled {
			return false
		}
		settled = true
		return true
	}

	upgrade = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		db := req.Get("result")
		if !db.Get("objectStoreNames").Call("contains", folderDBStore).Bool() {
			db.Call("createObjectStore", folderDBStore)
		}
		return nil
	})
	blocked = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		if settle() {
			onErr(errors.New("web: indexedDB open blocked by another connection"))
		}
		return nil
	})
	failure = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		release()
		if settle() {
			onErr(errors.New("web: indexedDB open failed"))
		}
		return nil
	})
	success = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		release()
		db := req.Get("result")
		if !settle() {
			db.Call("close")
			return nil
		}
		tx := db.Call("transaction", folderDBStore, mode)
		// The connection is opened per call, so it is closed per call: once
		// the transaction ends (complete or abort — an error aborts it) the
		// database is let go, instead of one connection lingering per
		// Open/Restore and blocking a version change in another tab.
		var closer js.Func
		closer = js.FuncOf(func(_ js.Value, _ []js.Value) any {
			closer.Release()
			db.Call("close")
			return nil
		})
		tx.Call("addEventListener", "complete", closer)
		tx.Call("addEventListener", "abort", closer)
		fn(tx.Call("objectStore", folderDBStore), onErr)
		return nil
	})

	req.Set("onupgradeneeded", upgrade)
	req.Set("onsuccess", success)
	req.Set("onerror", failure)
	req.Set("onblocked", blocked)
}

// idbOpen issues indexedDB.open, turning a synchronous throw into an error.
func idbOpen(idb js.Value) (req js.Value, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("web: indexedDB open: %v", r)
		}
	}()
	return idb.Call("open", folderDBName, 1), nil
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
