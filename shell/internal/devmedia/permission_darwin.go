//go:build darwin && !ios && !js

package devmedia

import (
	"sync"

	"github.com/doug/gophics/internal/objc"
	"github.com/doug/gophics/shell"
)

// Microphone permission on macOS, which can be asked ahead of the attempt.
//
// This existed for the camera and not for the microphone, and the comment
// explaining why said no desktop platform has a queryable permission API. That
// is false here: it is the same AVCaptureDevice call, with the media type
// changed from video to audio. The cost of the gap was that a user who had
// denied microphone access in System Settings was told "granted", opened a
// device that returned silence, and got no way to tell that from a quiet room.

// AVAuthorizationStatus, from AVCaptureDevice.h. Declared again here rather
// than shared with internal/camera, which loads three frameworks and an FFI
// surface this needs none of.
const (
	avAuthRestricted = 1
	avAuthDenied     = 2
	avAuthAuthorized = 3
)

var (
	avOnce sync.Once
	avErr  error
)

func micPermission() shell.Permission {
	avOnce.Do(func() {
		if avErr = objc.Init(); avErr != nil {
			return
		}
		avErr = objc.LoadFramework("AVFoundation")
	})
	// If the framework will not load, the honest answer is the one this
	// returned before it could ask at all: try, and let Listen find out.
	if avErr != nil {
		return shell.PermissionGranted
	}
	dev := objc.Class("AVCaptureDevice")
	if !dev.Valid() {
		return shell.PermissionGranted
	}
	// "soun" is AVMediaTypeAudio; the camera path passes "vide". Pass anything
	// else and AVFoundation raises an Objective-C exception rather than
	// returning a status, which crosses back into Go as a process abort.
	return permissionFor(dev.SendInt("authorizationStatusForMediaType:", objc.Obj(objc.String("soun"))))
}

// permissionFor maps an AVAuthorizationStatus onto the shell's answer.
//
// Split from the query so it can be tested. The query cannot be: on a machine
// where access is granted, a function that asks and one that returns Granted
// unconditionally are indistinguishable — which is exactly the bug this file
// fixes, and it went unnoticed for that reason.
func permissionFor(status int64) shell.Permission {
	switch status {
	case avAuthAuthorized:
		return shell.PermissionGranted
	case avAuthDenied, avAuthRestricted:
		return shell.PermissionDenied
	default:
		// Not determined, or a value this build does not know: asking happens
		// when the device is opened.
		return shell.PermissionPrompt
	}
}
