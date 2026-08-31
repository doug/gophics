//go:build !js && !linux && !darwin && !windows

package desktop

import "github.com/doug/gophics/shell"

// Fallback for platforms with no controller backend — the BSDs, in practice.
// macOS uses GameController, Linux evdev and Windows XInput; see the
// per-platform files beside this one.
//
// Returning nil is the honest answer, and it is the same one Battery gives on
// this build. The capability used to be published with a Poll that always
// returned nothing, on the reasoning that a game could then poll every frame
// without a nil check. That convenience costs the caller the only thing it
// needs to know: "no controller is connected" and "this build cannot see
// controllers" become the same answer, so a game shows a pairing prompt that
// can never be satisfied. Mobile carried the identical stub and was made nil
// for the identical reason.
//
// TODO(platform): FreeBSD/OpenBSD expose controllers through usbhid and uhid,
// which is a different model again from evdev. Implementing that is what makes
// this non-nil.
func (w *window) Gamepads() shell.Gamepads { return nil }
