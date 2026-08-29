package mobile

import (
	"errors"

	"github.com/doug/gophics/shell"
)

// Share and local notifications, over the same host-drain shape the media
// capabilities use: Go hands the host a request id, the host does the native
// work and calls back with that id.
//
// Each host interface is registered on its own (SetShareHost, SetNotifyHost)
// rather than folded into one big PlatformHost. A host implements a
// gomobile-generated protocol, so widening an existing interface breaks every
// host that already conforms to it; separate interfaces let a host adopt one
// capability without being forced to stub the rest, and an app sees nil for
// whatever the host did not wire.

// ShareHost is the native share sheet, implemented by the host.
type ShareHost interface {
	// Share presents the platform share sheet.
	// Answer with DeliverShareResult(reqID, "") on completion or dismissal, or a
	// message on failure.
	Share(reqID int, title, text, url, fileName string, fileData []byte)
}

// SetShareHost registers the share backend, enabling ctx.Share().
func (b *Bridge) SetShareHost(h ShareHost) { b.shareHost = h; b.capabilitiesChanged() }

// Share makes the Bridge a shell.ShareWindow.
func (b *Bridge) Share() shell.Share {
	if b.shareHost == nil {
		return nil
	}
	return mobileShare{b}
}

type mobileShare struct{ b *Bridge }

func (s mobileShare) Share(item shell.ShareItem, done func(error)) {
	b := s.b
	id := b.newReq()
	if done != nil {
		if b.shareCb == nil {
			b.shareCb = map[int]func(error){}
		}
		b.shareCb[id] = done
	}
	b.shareHost.Share(id, item.Title, item.Text, item.URL, item.FileName, item.FileData)
}

// DeliverShareResult completes a Share request: "" for success or a dismissal,
// otherwise the message to report.
//
// Dismissal is success because most platforms cannot tell the two apart — iOS
// reports completed=false for both "cancelled" and "the extension failed", so
// an app that treated dismissal as an error would show a failure every time
// somebody changed their mind.
func (b *Bridge) DeliverShareResult(reqID int, errMsg string) {
	cb := b.shareCb[reqID]
	if cb == nil {
		return
	}
	delete(b.shareCb, reqID)
	if errMsg == "" {
		cb(nil)
		return
	}
	cb(errors.New(errMsg))
}

// NotifyHost posts local notifications, implemented by the host.
type NotifyHost interface {
	// AuthorizeNotify requests permission to post.
	// Answer with DeliverNotifyPermission(reqID, granted).
	AuthorizeNotify(reqID int)
	// Notify posts a notification. A non-empty tag replaces any previous
	// notification with the same tag rather than stacking a new one.
	Notify(title, body, tag string)
}

// SetNotifyHost registers the notification backend, enabling ctx.Notifier().
func (b *Bridge) SetNotifyHost(h NotifyHost) { b.notifyHost = h; b.capabilitiesChanged() }

// Notifier makes the Bridge a shell.NotifyWindow.
func (b *Bridge) Notifier() shell.Notifier {
	if b.notifyHost == nil {
		return nil
	}
	return mobileNotifier{b}
}

type mobileNotifier struct{ b *Bridge }

func (n mobileNotifier) Authorize(cb func(shell.Permission)) {
	b := n.b
	id := b.newReq()
	if cb != nil {
		if b.notifyCb == nil {
			b.notifyCb = map[int]func(shell.Permission){}
		}
		b.notifyCb[id] = cb
	}
	b.notifyHost.AuthorizeNotify(id)
}

func (n mobileNotifier) Notify(msg shell.Notification) {
	n.b.notifyHost.Notify(msg.Title, msg.Body, msg.Tag)
}

// DeliverNotifyPermission answers an AuthorizeNotify request.
func (b *Bridge) DeliverNotifyPermission(reqID int, granted bool) {
	cb := b.notifyCb[reqID]
	if cb == nil {
		return
	}
	delete(b.notifyCb, reqID)
	if granted {
		cb(shell.PermissionGranted)
		return
	}
	cb(shell.PermissionDenied)
}

// newReq allocates a request id for the capabilities in this file. It is
// separate from the media bridge's counter because the two answer through
// different host interfaces and never see each other's ids.
func (b *Bridge) newReq() int {
	b.reqNext++
	return b.reqNext
}
