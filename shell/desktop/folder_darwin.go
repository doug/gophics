//go:build darwin && !ios && !js

// macOS folder chooser (shell/folder.go): NSOpenPanel in directory mode,
// through the pure-Go Objective-C bridge. No CGo.
package desktop

import (
	"errors"

	"github.com/doug/gophics/internal/objc"
	"github.com/doug/gophics/shell"
)

// FolderPicker publishes the capability; the app runner wires it to the widget
// tree.
func (w *window) FolderPicker() shell.FolderPicker { return macFolderPicker{w: w} }

// macFolderPicker holds the window so it can route panel work to the main
// thread, for the same reason macPicker does: an NSOpenPanel is an NSWindow,
// and AppKit aborts the process rather than returning an error when one is
// constructed off the main thread. See mainthread.go.
type macFolderPicker struct{ w *window }

func (p macFolderPicker) Open(done func(shell.Folder, error)) {
	if done == nil {
		return
	}
	p.w.runOnMain(func() { p.open(done) })
}

func (p macFolderPicker) open(done func(shell.Folder, error)) {
	panel, err := newPanel("NSOpenPanel", "openPanel")
	if err != nil {
		done(nil, err)
		return
	}
	// The whole difference from the file panel: directories only.
	panel.SendVoid("setCanChooseFiles:", objc.Bool(false))
	panel.SendVoid("setCanChooseDirectories:", objc.Bool(true))
	panel.SendVoid("setAllowsMultipleSelection:", objc.Bool(false))

	if !runPanel(panel) {
		done(nil, nil) // cancelled
		return
	}
	urls := objc.Array(panel.Send("URLs"))
	if len(urls) == 0 {
		done(nil, nil)
		return
	}
	path := objc.GoString(urls[0].Send("path"))
	if path == "" {
		done(nil, errors.New("desktop: open panel returned no path"))
		return
	}
	done(osFolder{path: path}, nil)
}
