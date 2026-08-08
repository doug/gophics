//go:build js && wasm

// Web implementation of the shell share capability (shell/share.go) using the
// Web Share API (navigator.share). Reuses the package await helper.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Share returns a share capability only when the browser exposes navigator.share
// (a secure context on a supporting browser); otherwise nil, so ctx.Share() is
// nil and callers hide their share affordance.
func (w *window) Share() shell.Share {
	if js.Global().Get("navigator").Get("share").IsUndefined() {
		return nil
	}
	return &webShare{}
}

type webShare struct{}

func (s *webShare) Share(item shell.ShareItem, done func(error)) {
	nav := js.Global().Get("navigator")
	data := map[string]any{}
	if item.Title != "" {
		data["title"] = item.Title
	}
	if item.Text != "" {
		data["text"] = item.Text
	}
	if item.URL != "" {
		data["url"] = item.URL
	}
	if len(item.FileData) > 0 && item.FileName != "" {
		u8 := js.Global().Get("Uint8Array").New(len(item.FileData))
		js.CopyBytesToJS(u8, item.FileData)
		file := js.Global().Get("File").New([]any{u8}, item.FileName)
		data["files"] = []any{file}
		// Some browsers reject files in a share; drop them if canShare says no.
		if cs := nav.Get("canShare"); !cs.IsUndefined() && !nav.Call("canShare", data).Bool() {
			delete(data, "files")
		}
	}

	promise := nav.Call("share", data)
	go func() {
		// A user cancel rejects with AbortError; treat any rejection as a
		// non-fatal dismissal (the interface promises no reliable "shared" signal).
		_, _ = await(promise)
		if done != nil {
			done(nil)
		}
	}()
}
