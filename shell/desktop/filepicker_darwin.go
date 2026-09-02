//go:build darwin && !ios && !js

// macOS implementation of the file-picking capability (shell/filepicker.go),
// driving the real AppKit panels — NSOpenPanel and NSSavePanel — through the
// pure-Go Objective-C bridge in internal/objc. No CGo.
package desktop

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/doug/gophics/internal/objc"
	"github.com/doug/gophics/shell"
)

// FilePicker publishes the capability; the app runner wires it to the widget tree.
func (w *window) FilePicker() shell.FilePicker { return macPicker{w: w} }

// macPicker holds the window so it can route panel work to the main thread.
type macPicker struct{ w *window }

// The panels are main-thread-bound: an NSOpenPanel/NSSavePanel *is* an NSWindow,
// and AppKit refuses to even construct one off the main thread, raising
// NSInternalInconsistencyException ("NSWindow should only be instantiated on the
// main thread!") — an uncatchable process abort from Go, not an error we could
// return. gophics drives Build and tickers from gogpu's render thread, so the
// caller often is not the main thread; every panel therefore runs inside
// w.runOnMain (inline from an input handler, queued to the next OnUpdate
// otherwise). See mainthread.go.
//
// The callback is invoked from the main thread; the generated PostedFilePicker
// wrapper marshals it back to the UI goroutine, so app code sees the documented
// contract either way.

// Open presents NSOpenPanel and reads the chosen files.
func (p macPicker) Open(opts shell.OpenOptions, done func([]shell.PickedFile, error)) {
	if done == nil {
		return
	}
	p.w.runOnMain(func() { p.open(opts, done) })
}

func (p macPicker) open(opts shell.OpenOptions, done func([]shell.PickedFile, error)) {
	panel, err := newPanel("NSOpenPanel", "openPanel")
	if err != nil {
		done(nil, err)
		return
	}
	panel.SendVoid("setCanChooseFiles:", objc.Bool(true))
	panel.SendVoid("setCanChooseDirectories:", objc.Bool(false))
	panel.SendVoid("setAllowsMultipleSelection:", objc.Bool(opts.Multiple))
	setAllowedTypes(panel, opts.Accept)

	if !runPanel(panel) {
		done([]shell.PickedFile{}, nil) // cancelled
		return
	}

	urls := objc.Array(panel.Send("URLs"))
	files := make([]shell.PickedFile, 0, len(urls))
	for _, u := range urls {
		path := objc.GoString(u.Send("path"))
		if path == "" {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			done(nil, err)
			return
		}
		files = append(files, shell.PickedFile{
			Name: filepath.Base(path),
			Data: data,
			Path: path,
		})
	}
	done(files, nil)
}

// Save presents NSSavePanel and writes data to the chosen path.
func (p macPicker) Save(opts shell.SaveOptions, data []byte, done func(error)) {
	p.w.runOnMain(func() { p.save(opts, data, done) })
}

func (p macPicker) save(opts shell.SaveOptions, data []byte, done func(error)) {
	report := func(err error) {
		if done != nil {
			done(err)
		}
	}
	panel, err := newPanel("NSSavePanel", "savePanel")
	if err != nil {
		report(err)
		return
	}
	if opts.Name != "" {
		panel.SendVoid("setNameFieldStringValue:", objc.Obj(objc.String(opts.Name)))
	}
	setAllowedTypes(panel, opts.Accept)

	if !runPanel(panel) {
		report(nil) // cancelled
		return
	}
	path := objc.GoString(panel.Send("URL").Send("path"))
	if path == "" {
		report(errors.New("desktop: save panel returned no path"))
		return
	}
	report(writeFileAtomic(path, data))
}

// newPanel loads AppKit and constructs the named panel class.
func newPanel(class, ctor string) (objc.ID, error) {
	if err := objc.LoadFramework("AppKit"); err != nil {
		return 0, err
	}
	cls := objc.Class(class)
	if !cls.Valid() {
		return 0, errors.New("desktop: " + class + " unavailable")
	}
	p := cls.Send(ctor)
	if !p.Valid() {
		return 0, errors.New("desktop: could not create " + class)
	}
	return p, nil
}

// runPanel shows the panel modally and reports whether the user confirmed.
// Callers arrive via runOnMain, so -runModal is on the main thread and its
// response code is available directly.
func runPanel(panel objc.ID) bool {
	const nsModalResponseOK = 1
	return panel.SendInt("runModal") == nsModalResponseOK
}

// setAllowedTypes applies an Accept hint as the panel's file-type filter.
//
// -setAllowedFileTypes: is soft-deprecated (macOS 12 prefers allowedContentTypes
// with UTType objects) but remains functional and is far cheaper to bridge; the
// filter is documented as a hint, so degrading to "no filter" is acceptable.
func setAllowedTypes(panel objc.ID, accept []string) {
	exts := extensions(accept)
	if len(exts) == 0 {
		return
	}
	objs := make([]objc.ID, 0, len(exts))
	for _, e := range exts {
		objs = append(objs, objc.String(e))
	}
	panel.SendVoid("setAllowedFileTypes:", objc.Obj(objc.NewArray(objs...)))
}

// extensions keeps the bare extensions from an Accept list, dropping MIME types
// (which AppKit's extension filter can't use) and any leading dot.
func extensions(accept []string) []string {
	out := make([]string, 0, len(accept))
	for _, a := range accept {
		a = strings.TrimSpace(a)
		if a == "" || strings.Contains(a, "/") {
			continue // a MIME type, not an extension
		}
		out = append(out, strings.TrimPrefix(a, "."))
	}
	return out
}
