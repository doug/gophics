//go:build !js && !(darwin && !ios)

package desktop

import "github.com/doug/gophics/shell"

// CameraPreview reports that live camera preview is unavailable here.
//
// shell.LiveMediaWindow pairs the preview with the microphone, but they are
// independent capabilities and a platform may have one without the other —
// which is the case on Linux and Windows today, where the microphone is
// implemented and the camera is not. Linux wants V4L2 and Windows Media
// Foundation; nil is the contract's way of saying so, and an app hides the
// affordance rather than failing.
func (w *window) CameraPreview() shell.CameraPreview { return nil }
