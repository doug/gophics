//go:build !js && !((darwin && !ios) || (linux && !android) || windows)

package devmedia

import "github.com/doug/gophics/shell"

// CameraPreview reports that live camera preview is unavailable here.
//
// This is the fallback for the BSDs: macOS, Linux and Windows all have a
// camera backend in internal/camera (AVFoundation, V4L2, Media Foundation),
// and the microphone is implemented on all of them too. The preview and the
// microphone are independent capabilities, so a platform may have one without
// the other; nil is the contract's way of saying so, and an app hides the
// affordance rather than failing.
func CameraPreview() shell.CameraPreview { return nil }

// Camera returns nil: still capture follows the preview, and there is no
// preview here.
func Camera() shell.Camera { return nil }
