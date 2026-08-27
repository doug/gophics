//go:build darwin || linux

package terminal

import (
	"testing"

	"github.com/doug/gophics/shell"
)

// A terminal window really offers the microphone and the speakers.
//
// It is a process on the same machine as those devices, so nothing about a TTY
// makes them unavailable — they were simply never wired, and an app that
// recorded fine in a window went silent in a terminal for no reason it could
// discover. That is the kind of gap a compile-time assertion does not close:
// the interface assertions in this file hold if the methods exist, whatever
// they return, so a method returning nil satisfies them and the capability is
// still missing at run time.
//
// This is deliberately not a recording test. It does not need a microphone to
// be attached, and it must pass on a build machine with no audio hardware —
// what it checks is that the wiring exists and hands back something, which is
// the part that was absent.
func TestATerminalOffersItsAudioDevices(t *testing.T) {
	var w window

	var mw shell.MicrophoneWindow = &w
	if mw.Microphone() == nil {
		t.Error("a terminal reports no microphone; an app running here would be silent")
	}
	var sw shell.SpeakersWindow = &w
	if sw.Speakers() == nil {
		t.Error("a terminal reports no speakers")
	}
}

// And it does not claim a camera, which it has no way to display.
func TestATerminalDoesNotClaimACamera(t *testing.T) {
	var w any = &window{}
	if _, ok := w.(shell.CameraWindow); ok {
		t.Error("a terminal declares a camera it cannot show; one interface per " +
			"device exists so a shell implements only what it has")
	}
}
