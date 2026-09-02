//go:build !js

// Desktop implementation of the folder capability (shell/folder.go): a
// directory the user picked, read and written with ordinary file I/O.
//
// The per-platform half is only the chooser — folder_darwin.go, folder_unix.go,
// folder_windows.go — because once a directory has been chosen there is nothing
// platform-specific about reading it. That split is why this file has no build
// tag beyond !js.
package desktop

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/doug/gophics/shell"
)

// osFolder is a directory on the local filesystem.
type osFolder struct{ path string }

// Name is the folder's display name, not its path: shell.Folder documents it
// for showing the user which folder is open, and a deep path is worse at that
// than its last element.
func (f osFolder) Name() string { return filepath.Base(f.path) }

// Token is the path. Desktop has no permission to lose and no handle to keep
// alive, so the path is the whole of what a later Restore needs — and unlike
// the web token it stays meaningful if a human ever reads the preference file.
func (f osFolder) Token() string { return f.path }

// restoreFolder reopens a remembered path.
//
// A folder the user has since deleted, renamed, or unmounted reports nothing
// rather than an error: shell.FolderPicker.Restore treats a stale token as "the
// folder is gone", which is the ordinary case here — an external drive that is
// not plugged in this morning is not a failure to report.
func restoreFolder(token string, done func(shell.Folder, error)) {
	if done == nil {
		return
	}
	if token == "" {
		done(nil, nil)
		return
	}
	go func() {
		info, err := os.Stat(token)
		if err != nil || !info.IsDir() {
			done(nil, nil)
			return
		}
		done(osFolder{path: token}, nil)
	}()
}

// Every method runs its I/O on a goroutine and reports through the callback.
// Disk work is fast but not bounded — a vault on a network mount can stall for
// seconds — and the caller is the UI goroutine. The generated PostedFolder
// wrapper marshals the callback back to it.

func (f osFolder) List(opts shell.FolderListOptions, done func([]shell.FolderEntry, error)) {
	if done == nil {
		return
	}
	go func() {
		ents, err := os.ReadDir(f.path)
		if err != nil {
			done(nil, err)
			return
		}
		// os.ReadDir sorts by filename, which is the order shell.Folder.List
		// promises, so there is nothing to sort here.
		out := make([]shell.FolderEntry, 0, len(ents))
		for _, e := range ents {
			if e.IsDir() || !opts.Accepts(e.Name()) {
				continue
			}
			var size int64
			if info, err := e.Info(); err == nil {
				size = info.Size()
			}
			out = append(out, shell.FolderEntry{Name: e.Name(), Size: size})
		}
		done(out, nil)
	}()
}

func (f osFolder) Read(name string, done func([]byte, error)) {
	if done == nil {
		return
	}
	if err := shell.CheckFolderName(name); err != nil {
		done(nil, err)
		return
	}
	go func() {
		b, err := os.ReadFile(filepath.Join(f.path, name))
		if err != nil {
			done(nil, err)
			return
		}
		done(b, nil)
	}()
}

// Write replaces the file atomically — to a temporary file in the same
// directory, then rename.
//
// os.WriteFile truncates first, so a crash or a full disk midway through leaves
// the note empty and the previous contents gone. That is the one failure a
// notes vault must not have. It also matches the web backend, where
// createWritable() writes to a swap file and commits on close, so an app sees
// the same guarantee on both platforms rather than a stronger one where it
// happened to be developed.
func (f osFolder) Write(name string, data []byte, done func(error)) {
	report := func(err error) {
		if done != nil {
			done(err)
		}
	}
	if err := shell.CheckFolderName(name); err != nil {
		report(err)
		return
	}
	go report(writeFolderFile(filepath.Join(f.path, name), data))
}

// Remove deletes one file. Removing what is not there is not an error, as
// shell.Folder documents.
func (f osFolder) Remove(name string, done func(error)) {
	report := func(err error) {
		if done != nil {
			done(err)
		}
	}
	if err := shell.CheckFolderName(name); err != nil {
		report(err)
		return
	}
	go func() {
		err := os.Remove(filepath.Join(f.path, name))
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		report(err)
	}()
}

// writeFolderFile writes data to path via a temporary file in the same
// directory, then renames.
//
// Same directory, not the system temp dir, because a rename is only atomic
// within a filesystem; across one it degrades to a copy, which is the
// truncation this exists to avoid. The temporary file is removed on any failure
// so a crashed write does not litter the user's folder.
//
// Not the picker's writeAtomic, which is deliberately different: that one is
// for a one-shot Save and does not fsync, while a vault is written on every
// keystroke pause and has to survive a power loss. Both share the fallback for
// a directory that will not take a temporary file — a FUSE mount, say — because
// a note that cannot be saved at all is worse than one saved non-atomically.
func writeFolderFile(path string, data []byte) error {
	dir, base := filepath.Split(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp")
	if err != nil {
		return os.WriteFile(path, data, 0o644)
	}
	name := tmp.Name()
	fail := func(err error) error { tmp.Close(); os.Remove(name); return err }

	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
	// Sync before rename: the rename can otherwise be durable while the bytes
	// are not, leaving a file that exists and is empty after a power loss.
	if err := tmp.Sync(); err != nil {
		return fail(err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, path); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}
