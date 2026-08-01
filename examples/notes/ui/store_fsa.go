//go:build js && wasm

package ui

import (
	"errors"
	"strings"
	"syscall/js"
)

// fsaStore persists notes as real .md files in a user-picked folder via the
// browser's File System Access API — true local-first parity with desktop: the
// files live on the user's disk and are editable outside the app. Supported in
// Chromium browsers; Safari/Firefox coverage is partial.
type fsaStore struct{ dir js.Value } // a FileSystemDirectoryHandle

func (s *fsaStore) List() ([]Note, error) {
	var notes []Note
	iter := s.dir.Call("values") // async iterator of FileSystemHandles
	for {
		next, err := await(iter.Call("next"))
		if err != nil {
			return notes, err
		}
		if next.Get("done").Bool() {
			break
		}
		h := next.Get("value")
		if h.Get("kind").String() != "file" {
			continue
		}
		name := h.Get("name").String()
		if !strings.HasSuffix(strings.ToLower(name), ".md") {
			continue
		}
		file, err := await(h.Call("getFile"))
		if err != nil {
			continue
		}
		text, err := await(file.Call("text"))
		if err != nil {
			continue
		}
		base := name[:len(name)-len(".md")]
		notes = append(notes, Note{Path: base, Name: base, Body: text.String()})
	}
	return notes, nil
}

func (s *fsaStore) Write(name, body string) (Note, error) {
	fh, err := await(s.dir.Call("getFileHandle", name+".md", map[string]any{"create": true}))
	if err != nil {
		return Note{}, err
	}
	w, err := await(fh.Call("createWritable"))
	if err != nil {
		return Note{}, err
	}
	if _, err := await(w.Call("write", body)); err != nil {
		return Note{}, err
	}
	if _, err := await(w.Call("close")); err != nil {
		return Note{}, err
	}
	return Note{Path: name, Name: name, Body: body}, nil
}

func (s *fsaStore) Remove(n Note) error {
	_, err := await(s.dir.Call("removeEntry", n.Name+".md"))
	return err
}

func (s *fsaStore) Label() string { return s.dir.Get("name").String() }

// defaultStore is nil on web: the File System Access API requires a user
// gesture, so the vault starts empty and the user opens a folder from the UI.
func defaultStore() store { return nil }

// openFolder runs the directory picker and loads the chosen folder into the
// vault. It is called from the sidebar button's click — a valid user gesture —
// so showDirectoryPicker() is invoked synchronously here (the gesture is
// consumed now); the awaiting runs on a goroutine so it doesn't block the frame.
func openFolder(s *workspaceState) {
	if js.Global().Get("showDirectoryPicker").IsUndefined() {
		s.SetState(func() {
			s.storeErr = "This browser can't open a folder — try Chrome or Edge."
		})
		return
	}
	promise := js.Global().Call("showDirectoryPicker") // consumes the gesture now
	go func() {
		handle, err := await(promise)
		if err != nil {
			return // the user dismissed the picker
		}
		st := &fsaStore{dir: handle}
		notes, err := st.List()
		if err != nil {
			s.SetState(func() { s.storeErr = "Could not read that folder." })
			return
		}
		s.SetState(func() {
			s.storeErr = ""
			s.W().Vault.adopt(st, notes)
		})
	}()
}

// await blocks the calling goroutine until the JS promise settles, returning its
// value or a rejection error. This is safe on wasm's single thread: a blocked
// goroutine hands control back to the JS event loop, which settles the promise
// and wakes us via the then/catch callbacks. It must NOT run on the event-loop
// goroutine itself (openFolder awaits inside a spawned goroutine).
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	var then, catch js.Func
	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- result{v: v}
		return nil
	})
	catch = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "promise rejected"
		if len(args) > 0 && args[0].Truthy() {
			msg = args[0].Call("toString").String()
		}
		ch <- result{err: errors.New(msg)}
		return nil
	})
	p.Call("then", then).Call("catch", catch)
	r := <-ch
	then.Release()
	catch.Release()
	return r.v, r.err
}
