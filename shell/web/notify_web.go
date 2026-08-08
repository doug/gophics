//go:build js && wasm

// Web implementation of the shell notification capability (shell/notify.go)
// using the Notification API.

package web

import (
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
	notif.New(msg.Title, opts)
}
