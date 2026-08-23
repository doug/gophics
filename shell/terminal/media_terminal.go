//go:build darwin || linux

package terminal

import (
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/internal/devmedia"
)

// Audio on the terminal.
//
// A terminal app is a process on the same machine as the speakers and the
// microphone, so there is nothing about a TTY that makes these unavailable —
// they were simply never wired up, and an app that recorded fine in a window
// went silent when run in a terminal for no reason it could discover. The
// adapters are the ones the desktop shell uses, in shell/internal/devmedia.
//
// The camera is deliberately absent. shell.MediaWindow pairs Camera with
// Audio, so Camera() has to exist here, but a terminal cannot display a
// preview and a still it cannot show is not worth opening a device and a
// permission prompt for. It returns nil, which is the same answer an app gets
// on a machine with no camera, and apps already handle that.
var (
	_ shell.MediaWindow     = (*window)(nil)
	_ shell.LiveMediaWindow = (*window)(nil)
)

// Audio returns the record/playback capability.
func (w *window) Audio() shell.Audio { return devmedia.Audio() }

// Microphone returns live input monitoring.
func (w *window) Microphone() shell.Microphone { return devmedia.Microphone() }

// Camera returns nil: see the note above.
func (w *window) Camera() shell.Camera { return nil }

// CameraPreview returns nil: see the note above.
func (w *window) CameraPreview() shell.CameraPreview { return nil }
