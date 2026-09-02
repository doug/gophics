//go:build js && wasm

// Web implementation of the shell file-picker capability (shell/filepicker.go).
// Open uses a hidden <input type=file> (the same idiom as the camera capture
// path); Save triggers a browser download from a Blob object URL. Reuses the
// package's await/jsToBytes helpers from media_web.go.

package web

import (
	"strings"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// FilePicker satisfies shell.FilePickerWindow for the web shell.
func (w *window) FilePicker() shell.FilePicker { return &webFilePicker{doc: w.doc} }

type webFilePicker struct{ doc js.Value }

// Open pops a file dialog. Browsers fire no event when the dialog is cancelled,
// so on cancel `done` simply isn't called (callers must treat that as "no
// change") — matching how the camera capture path behaves.
func (p *webFilePicker) Open(opts shell.OpenOptions, done func([]shell.PickedFile, error)) {
	if done == nil {
		return // result-only: a picker nobody receives is a no-op
	}
	input := p.doc.Call("createElement", "input")
	input.Set("type", "file")
	if len(opts.Accept) > 0 {
		input.Set("accept", strings.Join(opts.Accept, ","))
	}
	if opts.Multiple {
		input.Set("multiple", true)
	}

	var onChange js.Func
	onChange = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		files := input.Get("files")
		n := files.Length()
		if n == 0 {
			onChange.Release()
			done(nil, nil)
			return nil
		}
		go func() {
			defer onChange.Release()
			out := make([]shell.PickedFile, 0, n)
			for i := 0; i < n; i++ {
				f := files.Index(i)
				buf, err := await(f.Call("arrayBuffer"))
				if err != nil {
					done(nil, err)
					return
				}
				data := jsToBytes(js.Global().Get("Uint8Array").New(buf))
				out = append(out, shell.PickedFile{Name: f.Get("name").String(), Data: data})
			}
			done(out, nil)
		}()
		return nil
	})
	input.Call("addEventListener", "change", onChange)
	input.Call("click") // relies on the calling user gesture
}

// Save writes data to a file the user downloads, via a temporary object-URL
// anchor. The browser owns the destination, so there is no cancel signal.
func (p *webFilePicker) Save(opts shell.SaveOptions, data []byte, done func(error)) {
	u8 := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(u8, data)
	blob := js.Global().Get("Blob").New([]any{u8})
	url := js.Global().Get("URL").Call("createObjectURL", blob)

	name := opts.Name
	if name == "" {
		name = "download"
	}
	a := p.doc.Call("createElement", "a")
	a.Set("href", url)
	a.Set("download", name)
	a.Call("click")
	js.Global().Get("URL").Call("revokeObjectURL", url)
	if done != nil {
		done(nil)
	}
}
