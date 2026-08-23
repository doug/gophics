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
// These are the compile-time checks that the window still opts into both
// capability groups; dropping a method silently un-publishes it from the
// widget tree, which no test would otherwise catch.
var (
	_ shell.MediaWindow     = (*window)(nil)
	_ shell.LiveMediaWindow = (*window)(nil)
)

// Audio returns the record/playback capability.
func (w *window) Audio() shell.Audio { return devmedia.Audio() }

// Microphone returns live input monitoring.
func (w *window) Microphone() shell.Microphone { return devmedia.Microphone() }
