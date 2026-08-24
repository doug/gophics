//go:build !js

package desktop

import (
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/internal/devmedia"
)

// The window's media capabilities. The adapters themselves live in
// shell/internal/devmedia, because they concern devices rather than windows
// and the terminal shell needs the same ones.
//
// These are the compile-time checks that the window still opts into each one;
// dropping a method silently un-publishes it from the widget tree, which no
// test would otherwise catch.
var (
	_ shell.CameraWindow        = (*window)(nil)
	_ shell.CameraPreviewWindow = (*window)(nil)
	_ shell.MicrophoneWindow    = (*window)(nil)
	_ shell.SpeakersWindow      = (*window)(nil)
)

// Speakers returns audio output.
func (w *window) Speakers() shell.Speakers { return devmedia.Speakers() }

// Microphone returns audio input.
func (w *window) Microphone() shell.Microphone { return devmedia.Microphone() }

// Camera returns still capture, or nil where no backend exists yet.
func (w *window) Camera() shell.Camera { return devmedia.Camera() }

// CameraPreview returns live camera preview, or nil where no backend exists yet.
func (w *window) CameraPreview() shell.CameraPreview { return devmedia.CameraPreview() }
