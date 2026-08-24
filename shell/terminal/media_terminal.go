//go:build darwin || linux

package terminal

import (
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/internal/devmedia"
)

// The microphone and the speakers on the terminal.
//
// A terminal app is a process on the same machine as those devices, so there
// is nothing about a TTY that makes them unavailable — they were simply never
// wired up, and an app that recorded fine in a window went silent when run in
// a terminal for no reason it could discover. The adapters are the ones the
// desktop shell uses, in shell/internal/devmedia.
//
// No camera, and now that is said by omission rather than by a method. While
// the capabilities were paired, declaring the speakers meant also declaring a
// still camera a terminal cannot display and returning nil from it; one
// interface per device means a shell simply implements the devices it has.
var (
	_ shell.MicrophoneWindow = (*window)(nil)
	_ shell.SpeakersWindow   = (*window)(nil)
)

// Microphone returns audio input.
func (w *window) Microphone() shell.Microphone { return devmedia.Microphone() }

// Speakers returns audio output.
func (w *window) Speakers() shell.Speakers { return devmedia.Speakers() }
