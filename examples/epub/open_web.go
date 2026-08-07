//go:build js && wasm

package main

import (
	"errors"
	"syscall/js"
)

// openEPUB pops a browser file picker for an .epub and calls cb with the file's
// bytes. It must run inside a user gesture (the button tap) — browsers only open
// a file dialog then. cb is not called on cancel. The parser (parseEPUB in
// epub.go) already takes raw zip bytes, so this is the whole web integration.
func openEPUB(cb func([]byte)) {
	input := js.Global().Get("document").Call("createElement", "input")
	input.Set("type", "file")
	input.Set("accept", ".epub,application/epub+zip")

	var onChange js.Func
	onChange = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		files := input.Get("files")
		if files.Length() == 0 {
			onChange.Release()
			return nil
		}
		file := files.Index(0)
		go func() {
			defer onChange.Release()
			buf, err := await(file.Call("arrayBuffer"))
			if err != nil {
				return
			}
			u8 := js.Global().Get("Uint8Array").New(buf)
			data := make([]byte, u8.Length())
			js.CopyBytesToGo(data, u8)
			cb(data)
		}()
		return nil
	})
	input.Call("addEventListener", "change", onChange)
	input.Call("click")
}

// await blocks (on its own goroutine) until a JS promise settles, returning its
// resolved value or an error on rejection.
func await(promise js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	then := js.FuncOf(func(_ js.Value, args []js.Value) any {
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		ch <- result{v: v}
		return nil
	})
	catch := js.FuncOf(func(_ js.Value, _ []js.Value) any {
		ch <- result{err: errors.New("epub: reading the file failed")}
		return nil
	})
	defer then.Release()
	defer catch.Release()
	promise.Call("then", then).Call("catch", catch)
	r := <-ch
	return r.v, r.err
}
