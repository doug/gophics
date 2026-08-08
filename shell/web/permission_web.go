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
		md := js.Global().Get("navigator").Get("mediaDevices")
		if md.IsUndefined() {
			cb(shell.PermissionDenied)
			return
		}
		constraints := map[string]any{}
		if k == shell.PermCamera {
			constraints["video"] = true
		} else {
			constraints["audio"] = true
		}
		go func() {
			stream, err := await(md.Call("getUserMedia", constraints))
			if err != nil {
				cb(shell.PermissionDenied)
				return
			}
			// We only wanted the grant — stop the tracks immediately.
			tracks := stream.Call("getTracks")
			for i := 0; i < tracks.Length(); i++ {
				tracks.Index(i).Call("stop")
			}
			cb(shell.PermissionGranted)
		}()

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
