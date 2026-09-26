//go:build !js

package desktop

import (
	"errors"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strconv"
)

// writeFileAtomic writes data to path via a temporary file in the same
// directory, syncs it, and renames it into place.
//
// This is the one file-writing routine in the desktop shell, and it used to be
// four. The save panels on three platforms and the folder capability each grew
// their own: one fsynced and the others did not, two fell back to a direct
// write and two did not, and the Windows one deleted the destination before
// renaming — so a crash in that gap lost the user's file entirely, which is
// the exact failure an atomic write exists to prevent. (The delete was a
// workaround for a limitation Go no longer has: os.Rename on Windows replaces
// an existing file via MoveFileEx.) Four answers to "what does a save
// guarantee" is three too many, so now there is one, and it is the strictest:
//
//   - Same-directory temp file, because a rename is only atomic within a
//     filesystem; through the system temp dir it degrades to a copy, which is
//     the truncation window this exists to close.
//   - Sync before rename, because the rename can otherwise be durable while
//     the bytes are not, leaving a file that exists and is empty after a power
//     loss. The cost is unmeasurable at save-file rates.
//   - Fall back to a direct write when the directory will not take a temp file
//     (a FUSE mount, say): a file that cannot be saved at all is worse than
//     one saved non-atomically.
//   - The result has the mode a plain save would: an existing destination
//     keeps its own, a new file gets 0644 through the umask. The temp file
//     used to come from os.CreateTemp, which is 0600 by design, so every
//     document the save panels and Folder.Write produced was user-private —
//     a file exported for sharing on a multi-user machine silently lost
//     group and other read.
func writeFileAtomic(path string, data []byte) error {
	dir, base := filepath.Split(path)
	var keep os.FileMode
	if st, err := os.Stat(path); err == nil && st.Mode().IsRegular() {
		keep = st.Mode().Perm()
	}
	tmp, err := createTemp(dir, "."+base+".tmp")
	if err != nil {
		return os.WriteFile(path, data, 0o644)
	}
	name := tmp.Name()
	fail := func(err error) error { tmp.Close(); os.Remove(name); return err }

	if keep != 0 {
		// Chmod applies the bits as given, without the umask — the file
		// already had them.
		if err := tmp.Chmod(keep); err != nil {
			return fail(err)
		}
	}
	if _, err := tmp.Write(data); err != nil {
		return fail(err)
	}
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

// createTemp is os.CreateTemp with a 0644 create mode instead of 0600, so
// the file that will be renamed into place has the permissions a new
// document should, umask-adjusted by the kernel as for any create. An empty
// dir means the working directory, not the system temp dir — the rename has
// to stay on one filesystem.
func createTemp(dir, prefix string) (*os.File, error) {
	for range 10000 {
		name := filepath.Join(dir, prefix+strconv.FormatUint(uint64(rand.Uint32()), 10))
		f, err := os.OpenFile(name, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		return f, err
	}
	return nil, &fs.PathError{Op: "createtemp", Path: filepath.Join(dir, prefix+"*"), Err: fs.ErrExist}
}
