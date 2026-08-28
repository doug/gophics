package mobile

import (
	"errors"

	"github.com/doug/gophics/shell"
)

// The file chooser and location, both request/response over the host.
//
// Files cross as bytes, never as paths. That is the capability's design (see
// shell/filepicker.go) and it happens to be exactly what mobile needs: Android
// hands back a content:// URI that is not a filesystem path and is only
// readable while the permission grant lasts, and iOS returns a security-scoped
// URL that has to be opened inside a begin/endAccessingSecurityScopedResource
// pair. Both are read by the host, at the moment they are valid, and only the
// bytes reach Go — so PickedFile.Path is left empty on mobile, as documented.

// FileHost is the platform file chooser, implemented by the host.
type FileHost interface {
	// PickFiles presents an open dialog. accept is a comma-separated list of
	// extensions and MIME types ("" for any) — comma-separated because gomobile
	// cannot bind a []string parameter.
	//
	// Answer by calling DeliverPickedFile once per chosen file and then
	// DeliverPickedDone(reqID), or FailPick(reqID, msg). A cancelled dialog is
	// DeliverPickedDone with no files before it, not a failure.
	PickFiles(reqID int, accept string, multiple bool)
	// SaveFile presents a save dialog for data.
	// Answer with DeliverSaveDone(reqID, "") or a message.
	SaveFile(reqID int, name, accept string, data []byte)
}

// SetFileHost registers the file chooser, enabling ctx.FilePicker().
func (b *Bridge) SetFileHost(h FileHost) { b.fileHost = h }

// FilePicker makes the Bridge a shell.FilePickerWindow.
func (b *Bridge) FilePicker() shell.FilePicker {
	if b.fileHost == nil {
		return nil
	}
	return mobileFilePicker{b}
}

type mobileFilePicker struct{ b *Bridge }

func (f mobileFilePicker) Open(opts shell.OpenOptions, done func([]shell.PickedFile, error)) {
	b := f.b
	id := b.newReq()
	if done != nil {
		if b.pickCb == nil {
			b.pickCb = map[int]func([]shell.PickedFile, error){}
		}
		b.pickCb[id] = done
	}
	b.fileHost.PickFiles(id, joinAccept(opts.Accept), opts.Multiple)
}

func (f mobileFilePicker) Save(opts shell.SaveOptions, data []byte, done func(error)) {
	b := f.b
	id := b.newReq()
	if done != nil {
		if b.saveCb == nil {
			b.saveCb = map[int]func(error){}
		}
		b.saveCb[id] = done
	}
	b.fileHost.SaveFile(id, opts.Name, joinAccept(opts.Accept), data)
}

// DeliverPickedFile adds one chosen file to a pending open request. Call it
// once per file, then DeliverPickedDone.
//
// Files arrive one at a time because gomobile cannot carry a slice of structs
// across the boundary — only []byte — so a multi-select is a sequence of calls
// rather than one.
func (b *Bridge) DeliverPickedFile(reqID int, name string, data []byte) {
	if b.pickCb[reqID] == nil {
		return
	}
	if b.picked == nil {
		b.picked = map[int][]shell.PickedFile{}
	}
	// Copy: the host owns the buffer it passed and may reuse it.
	buf := make([]byte, len(data))
	copy(buf, data)
	b.picked[reqID] = append(b.picked[reqID], shell.PickedFile{Name: name, Data: buf})
}

// DeliverPickedDone completes an open request with whatever files were
// delivered, including none for a cancelled dialog.
func (b *Bridge) DeliverPickedDone(reqID int) {
	cb := b.pickCb[reqID]
	if cb == nil {
		return
	}
	files := b.picked[reqID]
	delete(b.pickCb, reqID)
	delete(b.picked, reqID)
	cb(files, nil)
}

// FailPick reports that an open request could not be served.
func (b *Bridge) FailPick(reqID int, msg string) {
	cb := b.pickCb[reqID]
	if cb == nil {
		return
	}
	delete(b.pickCb, reqID)
	delete(b.picked, reqID)
	cb(nil, errors.New(msg))
}

// DeliverSaveDone completes a save request: "" for success or a cancelled
// dialog, otherwise the message to report.
func (b *Bridge) DeliverSaveDone(reqID int, errMsg string) {
	cb := b.saveCb[reqID]
	if cb == nil {
		return
	}
	delete(b.saveCb, reqID)
	if errMsg == "" {
		cb(nil)
		return
	}
	cb(errors.New(errMsg))
}

// joinAccept flattens the Accept filter for the bind boundary.
func joinAccept(accept []string) string {
	out := ""
	for i, a := range accept {
		if i > 0 {
			out += ","
		}
		out += a
	}
	return out
}
