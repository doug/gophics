package shell

import (
	"fmt"
	"strings"
)

// Folder-picking capability: a directory the user chooses once and the app then
// reads and writes repeatedly. A Window exposes it by implementing
// FolderPickerWindow; callers reach it through ctx.FolderPicker(), which is nil
// where the platform can't provide one.
//
// This is deliberately separate from FilePicker, and the difference is the
// handle rather than the dialog. FilePicker carries bytes, not paths, because
// the web File API and mobile content URIs expose no stable filesystem path —
// which makes it right for "import this file" and unable to express "keep
// editing the files in that folder". Open hands over a snapshot with nothing to
// write back through, and Save on web is a download. A local-first app whose
// documents are real files on the user's disk needs a handle it can come back
// to, so that is what Folder is.
//
// Backed by the File System Access API on web, and by the native directory
// chooser plus ordinary file I/O on desktop. Mobile has no implementation yet,
// so ctx.FolderPicker() is nil there and callers hide the affordance.
//
// Every method is callback-shaped, including the ones a desktop implementation
// could answer immediately. That is not ceremony: on web each of these is a
// promise, and the honest options were a callback or a blocking wait. A
// synchronous List() would be answerable on desktop and would have to block the
// caller on web — where blocking the goroutine that runs Build deadlocks the
// frame, because the promise it waits on can only settle from the event loop it
// is holding. An API that works on the platform you develop on and hangs on the
// one you ship to is worse than one that is uniformly a little more verbose.
//
// All callbacks arrive on the UI goroutine; the generated PostedFolderPicker
// wrapper guarantees it, so implementations may call back from wherever the
// platform hands them the result.

// FolderPickerWindow is implemented by a Window that can present a directory
// chooser. The app runner type-asserts it and publishes FolderPicker() to the
// widget tree — the same shape as FilePickerWindow/MediaWindow.
type FolderPickerWindow interface {
	FolderPicker() FolderPicker
}

// FolderPicker presents the platform directory chooser.
type FolderPicker interface {
	// Open presents the chooser and reports the folder the user picked. On
	// cancel it reports a nil Folder and a nil error — dismissing a dialog is
	// not a failure.
	//
	// Must be called from a user gesture (a tap). Browsers only open a
	// directory picker during one, and the gesture is spent synchronously, so
	// an implementation must invoke the picker before doing anything that
	// yields.
	Open(done func(Folder, error))
}

// FolderEntry is one file in a folder. Directories are not reported: a
// capability that promised recursion would have to promise it on web too, where
// each level is another round of promises.
type FolderEntry struct {
	Name string // the file's name within the folder, e.g. "todo.md"
	Size int64  // in bytes; 0 where the platform does not report it cheaply
}

// FolderListOptions configures a listing.
type FolderListOptions struct {
	// Accept limits the listing by extension, e.g. {".md"}. Empty lists every
	// file. Unlike OpenOptions.Accept this is applied by gophics rather than
	// passed to a dialog, so it is exact rather than a hint, and it takes
	// extensions only — there is no file chooser here to interpret a MIME type.
	Accept []string
}

// Accepts reports whether name passes the filter; an empty Accept passes
// everything. Implementations use this rather than matching themselves, so that
// the same folder lists the same files on every platform — a backend rolling
// its own suffix check is how "note.MD" becomes visible on one platform and
// missing on another.
//
// Matching is case-insensitive: a vault written on a case-insensitive
// filesystem holds "NOTE.MD" as readily as "note.md", and the user who typed
// neither should see both.
func (o FolderListOptions) Accepts(name string) bool {
	if len(o.Accept) == 0 {
		return true
	}
	lower := strings.ToLower(name)
	for _, a := range o.Accept {
		if a == "" {
			continue
		}
		if !strings.HasPrefix(a, ".") {
			a = "." + a
		}
		if strings.HasSuffix(lower, strings.ToLower(a)) {
			return true
		}
	}
	return false
}

// Folder is a directory the user picked, which the app can read and write until
// it is dropped. Names are file names within the folder, never paths: a name
// containing a separator or ".." is rejected, so a folder grants access to
// itself and not to the disk around it.
//
// A Folder does not survive a restart. Reopening one without asking again is
// possible in principle — a filesystem path on desktop, an IndexedDB-persisted
// handle on web — but the two have different permission stories, and guessing
// at one would be worse than making the app ask.
type Folder interface {
	// Name is the folder's display name, for showing the user which folder is
	// open. It is not a path and is not unique.
	Name() string
	// List reports the files directly in the folder, sorted by name.
	List(opts FolderListOptions, done func([]FolderEntry, error))
	// Read returns one file's contents.
	Read(name string, done func([]byte, error))
	// Write creates or replaces one file.
	Write(name string, data []byte, done func(error))
	// Remove deletes one file. Removing a file that is not there is not an
	// error, matching Preferences.Delete and SecureStorage.Delete.
	Remove(name string, done func(error))
}

// CheckFolderName validates a name for use with Folder's methods, and is what
// makes Folder's "a folder grants access to itself and not to the disk around
// it" true rather than aspirational. Implementations call it before touching
// anything; it is exported because they live in other packages.
//
// A desktop backend joins the name onto a real directory path, so "../.ssh/id"
// there is arbitrary filesystem access with the user's own permissions. The web
// backend happens to be safe already — getFileHandle rejects a name containing
// a separator itself — but a guarantee that holds only because one platform's
// API is strict is not a guarantee.
//
// Backslash and colon are rejected everywhere, not only on Windows: a vault
// written on a Mac gets opened on Windows, and a name that is a path there
// should not have been creatable here.
func CheckFolderName(name string) error {
	switch {
	case name == "":
		return errFolderName{name, "is empty"}
	case name == "." || name == "..":
		return errFolderName{name, "is a directory reference"}
	case strings.ContainsAny(name, `/\`):
		return errFolderName{name, "contains a path separator"}
	case strings.ContainsRune(name, ':'):
		return errFolderName{name, "contains a colon"}
	case strings.ContainsRune(name, 0):
		return errFolderName{name, "contains a NUL"}
	}
	return nil
}

// errFolderName reports a rejected name, quoting it: the caller usually built
// it from user input and needs to see which one was refused.
type errFolderName struct{ name, why string }

func (e errFolderName) Error() string {
	return fmt.Sprintf("shell: folder file name %q %s", e.name, e.why)
}
