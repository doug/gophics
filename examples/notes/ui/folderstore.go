package ui

import (
	"errors"
	"strings"

	"github.com/doug/gophics/shell"
)

// prefFolderToken is where the open folder's token is remembered between
// sessions. The token itself is opaque and platform-shaped — a path on desktop,
// a handle key in the browser — so this only ever stores and returns it.
const prefFolderToken = "notes.folder"

// folderStore persists notes in a shell.Folder — a directory the user picked.
//
// This file used to be 142 lines of syscall/js driving the File System Access
// API, including a helper that blocked a goroutine on a JS promise and a
// comment naming the one goroutine that must never call it. That is the
// FolderPicker capability now, and what is left here is the part that is
// actually about notes: .md files, and what a Note's identity is.
//
// It has no build tag. The capability is nil where the platform cannot offer a
// folder, which is a runtime answer, so an app asks rather than compiling two
// versions of itself.
type folderStore struct {
	f     shell.Folder
	onErr func(error)
}

func newFolderStore(f shell.Folder, onErr func(error)) *folderStore {
	return &folderStore{f: f, onErr: onErr}
}

func (s *folderStore) Label() string { return s.f.Name() }

// Write saves the note and reports it as written before the bytes have landed.
//
// The vault is the in-memory model and the file is the write-through, so the
// editor cannot wait for a round trip on every keystroke pause — and on web
// there is no way to wait that does not block the frame. A failure therefore
// cannot come back through the return value; it arrives later through onErr,
// which is strictly more than the code this replaces did. That one awaited the
// real error and handed it to a caller that wrote `_ =`.
func (s *folderStore) Write(name, body string) (Note, error) {
	file := name + ".md"
	s.f.Write(file, []byte(body), s.report)
	return Note{Path: file, Name: name, Body: body}, nil
}

func (s *folderStore) Remove(n Note) error {
	s.f.Remove(n.Path, s.report)
	return nil
}

func (s *folderStore) report(err error) {
	if err != nil && s.onErr != nil {
		s.onErr(err)
	}
}

// openFolder asks the user for a folder and loads it into the vault.
//
// The picker must be reached from a tap, which is why this hangs off the
// button's handler rather than running at startup: a browser opens a directory
// chooser only during a user gesture. The capability spends that gesture
// synchronously inside Open, so nothing here has to be careful about it.
func openFolder(picker shell.FolderPicker, prefs shell.Preferences, s *workspaceState) {
	if picker == nil {
		s.SetState(func() {
			s.storeErr = "This browser can't open a folder — try Chrome or Edge."
		})
		return
	}
	picker.Open(func(f shell.Folder, err error) {
		switch {
		case err != nil:
			s.SetState(func() { s.storeErr = "Could not open that folder." })
		case f == nil:
			// The user dismissed the picker, which is not an error and should
			// not leave a message behind.
		default:
			loadFolder(s, f)
			remember(prefs, f)
		}
	})
}

// restoreFolder reopens the folder from the last session, if there is one.
//
// Called at mount, and again from the reopen button. Both matter: the first
// covers the case where the browser still has the grant, and the second is the
// only way to get it back when it has lapsed, because re-asking requires a user
// gesture and mounting is not one.
func restoreFolder(picker shell.FolderPicker, prefs shell.Preferences, s *workspaceState) {
	if picker == nil || prefs == nil {
		return
	}
	token, ok := prefs.Get(prefFolderToken)
	if !ok || token == "" {
		return
	}
	picker.Restore(token, func(f shell.Folder, err error) {
		switch {
		case errors.Is(err, shell.ErrFolderPermission):
			// Still there, still ours, just not granted right now.
			s.SetState(func() { s.reopen = true })
		case err != nil:
			s.SetState(func() { s.storeErr = "Could not reopen the last folder." })
		case f == nil:
			// Moved, deleted, or an unplugged drive. Forget it rather than
			// offering to reopen something that is not there.
			_ = prefs.Delete(prefFolderToken)
			s.SetState(func() { s.reopen = false })
		default:
			s.SetState(func() { s.reopen = false })
			loadFolder(s, f)
		}
	})
}

// remember stores the folder's token so the next session can reopen it.
// Failing to remember is not worth telling the user about: the folder is open
// and working, and the cost is being asked again next time.
func remember(prefs shell.Preferences, f shell.Folder) {
	if prefs == nil || f == nil {
		return
	}
	if token := f.Token(); token != "" {
		_ = prefs.Set(prefFolderToken, token)
	}
}

// loadFolder reads every .md file in f and adopts it as the vault.
//
// The reads are sequential rather than fanned out: each is a round trip to the
// browser's file system, and issuing hundreds at once is how a folder of notes
// becomes a stalled tab. A file that fails to read is skipped rather than
// failing the whole vault — one unreadable note should not cost the user the
// other forty.
func loadFolder(s *workspaceState, f shell.Folder) {
	f.List(shell.FolderListOptions{Accept: []string{".md"}}, func(entries []shell.FolderEntry, err error) {
		if err != nil {
			s.SetState(func() { s.storeErr = "Could not read that folder." })
			return
		}
		notes := make([]Note, 0, len(entries))
		var read func(int)
		read = func(i int) {
			if i == len(entries) {
				store := newFolderStore(f, func(error) {
					s.SetState(func() { s.storeErr = "Could not save to that folder." })
				})
				s.SetState(func() {
					s.storeErr = ""
					s.W().Vault.adopt(store, notes)
				})
				return
			}
			name := entries[i].Name
			f.Read(name, func(b []byte, err error) {
				if err == nil {
					notes = append(notes, Note{Path: name, Name: noteName(name), Body: string(b)})
				}
				read(i + 1)
			})
		}
		read(0)
	})
}

// noteName is a file name without its .md extension — the display name and the
// [[wikilink]] target. TrimSuffix on the lowered name would return the lowered
// name, so the length is what gets trimmed.
func noteName(file string) string {
	if strings.HasSuffix(strings.ToLower(file), ".md") {
		return file[:len(file)-len(".md")]
	}
	return file
}
