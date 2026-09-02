//go:build (linux || freebsd || openbsd || netbsd || dragonfly) && !android && !js

// Linux/BSD folder chooser (shell/folder.go). Like the file chooser, this
// drives whichever standard dialog program the session provides rather than
// binding GTK or Qt, which would mean CGo.
package desktop

import "github.com/doug/gophics/shell"

// FolderPicker publishes the capability only when the session has a chooser
// that can select a directory, so callers can use nil to decide whether to
// offer the button at all.
func (w *window) FolderPicker() shell.FolderPicker {
	if i, _ := chooser(); i < 0 || choosers[i].dir == nil {
		return nil
	}
	return unixFolderPicker{}
}

type unixFolderPicker struct{}

// Open runs the chooser on its own goroutine: it is a separate process and
// blocks until the user answers, which must not be the frame loop.
func (unixFolderPicker) Open(done func(shell.Folder, error)) {
	if done == nil {
		return
	}
	go func() {
		i, bin := chooser()
		if i < 0 || choosers[i].dir == nil {
			done(nil, ErrNoFileChooser)
			return
		}
		out, err := runChooser(bin, choosers[i].dir())
		if err != nil {
			done(nil, err)
			return
		}
		// runChooser reports a dismissed dialog as empty output and no error.
		if out == "" {
			done(nil, nil)
			return
		}
		done(osFolder{path: out}, nil)
	}()
}

// zenityDirArgs selects a directory. --directory only constrains what can be
// picked; --file-selection is still the mode.
func zenityDirArgs() []string { return []string{"--file-selection", "--directory"} }

// Restore reopens a remembered path; see restoreFolder in folder.go.
func (unixFolderPicker) Restore(token string, done func(shell.Folder, error)) {
	restoreFolder(token, done)
}
