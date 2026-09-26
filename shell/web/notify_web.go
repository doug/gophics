//go:build js && wasm

// Web implementation of the shell notification capability (shell/notify.go)
// using the Notification API.

package web

import (
	"log"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Notifier returns a notifier only when the browser exposes the Notification API.
func (w *window) Notifier() shell.Notifier {
	if js.Global().Get("Notification").IsUndefined() {
		return nil
	}
	return &webNotifier{}
}

type webNotifier struct{}

func (n *webNotifier) Authorize(cb func(shell.Permission)) {
	if cb == nil {
		return // the nil rule in shell/shell.go: nothing to deliver to
	}
	promise := js.Global().Get("Notification").Call("requestPermission")
	go func() {
		res, err := await(promise)
		p := shell.PermissionDenied
		if err == nil && res.String() == "granted" {
			p = shell.PermissionGranted
		}
		cb(p)
	}()
}

func (n *webNotifier) Notify(msg shell.Notification) {
	notif := js.Global().Get("Notification")
	if notif.Get("permission").String() != "granted" {
		return
	}
	opts := map[string]any{}
	if msg.Body != "" {
		opts["body"] = msg.Body
	}
	if msg.Tag != "" {
		opts["tag"] = msg.Tag
	}
	// Chrome for Android refuses `new Notification` from a page (TypeError:
	// Illegal constructor); there a notification has to be shown through a
	// service worker registration. When the page has one, go that way and
	// keep the constructor as the fallback; otherwise construct directly.
	if sw := js.Global().Get("navigator").Get("serviceWorker"); sw.Truthy() && sw.Get("controller").Truthy() {
		onSettled(sw.Get("ready"), func(reg js.Value, err error) {
			if err != nil || !reg.Truthy() {
				construct(notif, msg.Title, opts)
				return
			}
			onSettled(reg.Call("showNotification", msg.Title, opts), func(_ js.Value, err error) {
				if err != nil {
					construct(notif, msg.Title, opts)
				}
			})
		})
		return
	}
	construct(notif, msg.Title, opts)
}

// construct runs `new Notification`, which throws where page notifications
// are not allowed; a throw is a Go panic, and Notify is a no-op there rather
// than the end of the app.
func construct(notif js.Value, title string, opts map[string]any) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("gophics/web: notification not shown: %v", r)
		}
	}()
	notif.New(title, opts)
}
