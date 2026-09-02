//go:build !js

package desktop

import (
	"os"
	"path/filepath"
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
func writeFileAtomic(path string, data []byte) error {
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
