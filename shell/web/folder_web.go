//go:build js && wasm

// Web implementation of the folder capability (shell/folder.go) using the File
// System Access API, so an app's documents are real files in a folder the user
// picked — editable outside the app, and still there when it is uninstalled.
//
// Every operation here is a promise, and none of them is awaited. A blocking
// await is the obvious way to write this and is a trap: the goroutine that runs
// Build is the one the JS event loop resumes, so waiting on it for a promise
// the event loop must settle deadlocks the frame. Callbacks instead — which is
// why shell.Folder is callback-shaped even where desktop could answer at once.
package web

import (
	"fmt"
	"sort"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// FolderPicker publishes the capability, but only where the browser has the
// API. Safari and Firefox do not implement showDirectoryPicker; reporting a
// capability that always fails is worse than reporting none, because callers
// use nil to decide whether to offer the button at all.
func (w *window) FolderPicker() shell.FolderPicker {
	if js.Global().Get("showDirectoryPicker").IsUndefined() {
		return nil
	}
	return webFolderPicker{}
}

type webFolderPicker struct{}

// Open presents the directory chooser.
//
// showDirectoryPicker() is called synchronously, before anything that could
// yield, because it spends the user gesture: a browser opens a directory picker
// only during one, and any await beforehand ends it. Everything after is a
// callback, so the frame is never blocked waiting for the user to choose.
func (webFolderPicker) Open(done func(shell.Folder, error)) {
	if done == nil {
		return
	}
	promise := js.Global().Call("showDirectoryPicker") // spends the gesture now
	onSettled(promise, func(handle js.Value, err error) {
		if err != nil {
			// Dismissing the picker rejects with AbortError. That is a choice,
			// not a failure, and the capability reports it as a cancel.
			if isDOMError(err, "AbortError") {
				done(nil, nil)
				return
			}
			done(nil, err)
			return
		}
		// Store the handle before handing the folder over, so the token the
		// caller reads is one Restore can actually resolve. A failed store is
		// not a failed open: the folder works, it just will not come back on
		// its own next launch, and refusing the folder over that would be
		// worse than the app asking again.
		token := newFolderToken()
		idbPut(token, handle, func(err error) {
			if err != nil {
				token = ""
			}
			done(&webFolder{dir: handle, token: token}, nil)
		})
	})
}

// Restore reopens a folder from a token stored in a previous session.
func (webFolderPicker) Restore(token string, done func(shell.Folder, error)) {
	if done == nil {
		return
	}
	if token == "" {
		done(nil, nil)
		return
	}
	idbGet(token, func(handle js.Value, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		if !handle.Truthy() {
			done(nil, nil) // nothing stored under that token any more
			return
		}
		// The handle outlives the session; the permission does not. Chrome
		// hands back a handle in state "prompt" after a restart, and only a
		// user gesture may re-ask — which is why this reports
		// ErrFolderPermission instead of silently failing, so the app can put
		// the retry behind something tappable.
		ensurePermission(handle, func(ok bool, err error) {
			switch {
			case err != nil:
				done(nil, err)
			case !ok:
				done(nil, shell.ErrFolderPermission)
			default:
				done(&webFolder{dir: handle, token: token}, nil)
			}
		})
	})
}

// ensurePermission reports whether the app may read and write handle, asking
// the user if the grant has lapsed.
func ensurePermission(handle js.Value, done func(bool, error)) {
	opts := map[string]any{"mode": "readwrite"}
	if handle.Get("queryPermission").IsUndefined() {
		done(true, nil) // no permissions API here; the handle is all there is
		return
	}
	onSettled(handle.Call("queryPermission", opts), func(v js.Value, err error) {
		if err != nil {
			done(false, err)
			return
		}
		if v.String() == "granted" {
			done(true, nil)
			return
		}
		onSettled(handle.Call("requestPermission", opts), func(v js.Value, err error) {
			if err != nil {
				// Asking outside a user gesture rejects rather than returning
				// "prompt", and that is the same situation for the caller.
				done(false, nil)
				return
			}
			done(v.String() == "granted", nil)
		})
	})
}

// newFolderToken mints a key for one stored handle. Random rather than
// sequential so a token from a previous install cannot collide with a
// different folder in this one.
func newFolderToken() string {
	if c := js.Global().Get("crypto"); c.Truthy() && !c.Get("randomUUID").IsUndefined() {
		return c.Call("randomUUID").String()
	}
	return fmt.Sprintf("folder-%d", js.Global().Get("Date").Call("now").Int())
}

// webFolder is a FileSystemDirectoryHandle.
type webFolder struct {
	dir   js.Value
	token string
}

func (f *webFolder) Name() string { return f.dir.Get("name").String() }

// Token is the IndexedDB key the handle was stored under, not anything the user
// would recognise — shell.Folder.Token documents it as opaque, and here it has
// to be: a directory handle is not expressible as a string, so the string is a
// receipt for one the browser is holding.
func (f *webFolder) Token() string { return f.token }

// List walks the directory's async iterator one entry at a time, because that
// is the only way it is offered: values() yields a promise per step. The walk
// chains rather than recursing on the Go stack, so a folder of ten thousand
// files costs no more stack than one of ten.
//
// Size is left at 0. Reading it means getFile() per entry — a second promise
// each — and shell.FolderEntry documents Size as best-effort for exactly this
// reason: a sidebar listing should not pay a round trip per file to render.
func (f *webFolder) List(opts shell.FolderListOptions, done func([]shell.FolderEntry, error)) {
	if done == nil {
		return
	}
	iter := f.dir.Call("values")
	var out []shell.FolderEntry
	var step func()
	step = func() {
		onSettled(iter.Call("next"), func(next js.Value, err error) {
			if err != nil {
				done(nil, err)
				return
			}
			if next.Get("done").Bool() {
				sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
				done(out, nil)
				return
			}
			h := next.Get("value")
			if h.Get("kind").String() == "file" {
				name := h.Get("name").String()
				if opts.Accepts(name) {
					out = append(out, shell.FolderEntry{Name: name})
				}
			}
			step()
		})
	}
	step()
}

func (f *webFolder) Read(name string, done func([]byte, error)) {
	if done == nil {
		return
	}
	if err := shell.CheckFolderName(name); err != nil {
		done(nil, err)
		return
	}
	onSettled(f.dir.Call("getFileHandle", name), func(fh js.Value, err error) {
		if err != nil {
			done(nil, err)
			return
		}
		onSettled(fh.Call("getFile"), func(file js.Value, err error) {
			if err != nil {
				done(nil, err)
				return
			}
			// arrayBuffer, not text: a vault holds whatever the user put in it,
			// and reading bytes as a string round-trips them through UTF-16 and
			// corrupts anything that is not text.
			onSettled(file.Call("arrayBuffer"), func(buf js.Value, err error) {
				if err != nil {
					done(nil, err)
					return
				}
				u8 := js.Global().Get("Uint8Array").New(buf)
				b := make([]byte, u8.Get("length").Int())
				js.CopyBytesToGo(b, u8)
				done(b, nil)
			})
		})
	})
}

func (f *webFolder) Write(name string, data []byte, done func(error)) {
	report := func(err error) {
		if done != nil {
			done(err)
		}
	}
	if err := shell.CheckFolderName(name); err != nil {
		report(err)
		return
	}
	opts := map[string]any{"create": true}
	onSettled(f.dir.Call("getFileHandle", name, opts), func(fh js.Value, err error) {
		if err != nil {
			report(err)
			return
		}
		onSettled(fh.Call("createWritable"), func(w js.Value, err error) {
			if err != nil {
				report(err)
				return
			}
			buf := js.Global().Get("Uint8Array").New(len(data))
			js.CopyBytesToJS(buf, data)
			onSettled(w.Call("write", buf), func(_ js.Value, err error) {
				if err != nil {
					// The handle is still open; closing it releases the lock on
					// the file, which a later write would otherwise block on.
					w.Call("close")
					report(err)
					return
				}
				// The write lands on close, not on write: skipping this leaves
				// a zero-length file on disk and no error to explain it.
				onSettled(w.Call("close"), func(_ js.Value, err error) { report(err) })
			})
		})
	})
}

func (f *webFolder) Remove(name string, done func(error)) {
	report := func(err error) {
		if done != nil {
			done(err)
		}
	}
	if err := shell.CheckFolderName(name); err != nil {
		report(err)
		return
	}
	onSettled(f.dir.Call("removeEntry", name), func(_ js.Value, err error) {
		if err != nil && isDOMError(err, "NotFoundError") {
			err = nil // removing what is not there is not an error
		}
		report(err)
	})
}

// domError carries a DOMException's name so callers can tell a cancelled picker
// or an absent file from a real failure. The name is the only part of a DOM
// rejection that is stable enough to branch on; the message is for humans.
type domError struct {
	name string
	msg  string
}

func (e *domError) Error() string {
	if e.name == "" {
		return "web: " + e.msg
	}
	return fmt.Sprintf("web: %s: %s", e.name, e.msg)
}

// isDOMError reports whether err is a DOMException with the given name.
func isDOMError(err error, name string) bool {
	d, ok := err.(*domError)
	return ok && d.name == name
}

// onSettled delivers a promise's result to done without blocking the caller.
//
// The alternative — a channel receive — reads better and cannot be used here:
// it parks the calling goroutine until the JS event loop settles the promise,
// and on wasm the event loop is the thing that resumes that goroutine. Called
// from a widget, that is a deadlocked frame.
func onSettled(p js.Value, done func(js.Value, error)) {
	var then, catch js.Func
	release := func() { then.Release(); catch.Release() }

	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer release()
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		done(v, nil)
		return nil
	})
	catch = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer release()
		e := &domError{msg: "promise rejected"}
		if len(args) > 0 && args[0].Truthy() {
			r := args[0]
			if n := r.Get("name"); n.Type() == js.TypeString {
				e.name = n.String()
			}
			if m := r.Get("message"); m.Type() == js.TypeString && m.String() != "" {
				e.msg = m.String()
			} else {
				e.msg = r.Call("toString").String()
			}
		}
		done(js.Value{}, e)
		return nil
	})

	p.Call("then", then).Call("catch", catch)
}
