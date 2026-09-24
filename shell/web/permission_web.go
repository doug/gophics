//go:build js && wasm

// Web implementation of the shell unified permission capability
// (shell/permission.go). The web has no single permission API, so each kind
// routes to its own request path (Notification.requestPermission, getUserMedia,
// geolocation.getCurrentPosition); Status reads what can be read synchronously.

package web

import (
	"syscall/js"

	"github.com/doug/gophics/shell"
)

func (w *window) Permissions() shell.Permissions { return &webPermissions{} }

type webPermissions struct{}

func (p *webPermissions) Status(k shell.PermissionKind) shell.Permission {
	// Only notifications expose a synchronous status; the rest are only knowable
	// via the async Permissions API, so report Prompt (undecided) and let Request
	// drive the real flow.
	if k == shell.PermNotifications {
		n := js.Global().Get("Notification")
		if !n.IsUndefined() {
			switch n.Get("permission").String() {
			case "granted":
				return shell.PermissionGranted
			case "denied":
				return shell.PermissionDenied
			}
		}
	}
	return shell.PermissionPrompt
}

func (p *webPermissions) Request(k shell.PermissionKind, cb func(shell.Permission)) {
	switch k {
	case shell.PermNotifications:
		n := js.Global().Get("Notification")
		if n.IsUndefined() {
			cb(shell.PermissionDenied)
			return
		}
		go func() {
			res, err := await(n.Call("requestPermission"))
			cb(grant(err == nil && res.String() == "granted"))
		}()

	case shell.PermCamera, shell.PermMicrophone:
		// The same request the camera preview's and microphone's own
		// Authorize make (mediautil_web.go), which also covers a null
		// mediaDevices — not only an undefined one.
		constraints := map[string]any{"video": true}
		if k == shell.PermMicrophone {
			constraints = map[string]any{"audio": true}
		}
		requestStream(constraints, func(stream js.Value, err error) {
			if err != nil {
				cb(shell.PermissionDenied)
				return
			}
			stopTracks(stream) // only the grant was wanted
			cb(shell.PermissionGranted)
		})

	case shell.PermLocation:
		geo := js.Global().Get("navigator").Get("geolocation")
		if geo.IsUndefined() {
			cb(shell.PermissionDenied)
			return
		}
		var ok, fail js.Func
		ok = js.FuncOf(func(js.Value, []js.Value) any { ok.Release(); fail.Release(); cb(shell.PermissionGranted); return nil })
		fail = js.FuncOf(func(js.Value, []js.Value) any { ok.Release(); fail.Release(); cb(shell.PermissionDenied); return nil })
		geo.Call("getCurrentPosition", ok, fail)

	default:
		cb(shell.PermissionPrompt)
	}
}

func grant(ok bool) shell.Permission {
	if ok {
		return shell.PermissionGranted
	}
	return shell.PermissionDenied
}
