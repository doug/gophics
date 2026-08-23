package mobile

import "github.com/doug/gophics/shell"

// Haptic makes the Bridge a shell.HapticWindow: the UI queues feedback events,
// and the host drains them each frame with TakeHaptic and plays them on the OS
// generator (iOS UIFeedbackGenerator, Android performHapticFeedback/Vibrator).
// Following the same poll/drain pattern as OpenURL keeps Go free of any native
// call — the host owns the platform API.
func (b *Bridge) Haptic() shell.Haptic { return bridgeHaptic{b} }

type bridgeHaptic struct{ b *Bridge }

func (h bridgeHaptic) Play(k shell.HapticKind) {
	// Coalesce a burst of identical events in one frame (e.g. several toggles)
	// so the host gets one buzz, not a machine-gun.
	if n := len(h.b.haptics); n > 0 && h.b.haptics[n-1] == k {
		return
	}
	h.b.haptics = append(h.b.haptics, k)
}

// TakeHaptic returns and clears the oldest pending haptic event as its
// HapticKind value (see shell.HapticKind), or -1 when none are pending. The
// host calls it each frame and maps the value to its native generator.
//
// A sentinel rather than the comma-ok a Go reader expects, because this method
// crosses the gomobile boundary and a second result is allowed there only when
// it is an error. Every app's bind package used to flatten it to -1 by hand;
// doing it once here is what let those packages go away.
func (b *Bridge) TakeHaptic() int {
	if len(b.haptics) == 0 {
		return -1
	}
	k := b.haptics[0]
	b.haptics = b.haptics[1:]
	return int(k)
}
