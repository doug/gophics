//go:build darwin && !ios && !js

package desktop

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// The Mac provider reads the live system setting, through the objc bridge's
// double return — a path nothing else exercised. Cross-check against the
// same value read a different way, so a misread register cannot agree with
// itself.
func TestMacDoubleClickIntervalIsTheSystemsSetting(t *testing.T) {
	got := (&window{}).GestureTuning().DoubleTap
	out, err := exec.Command("swift", "-e", "import AppKit; print(NSEvent.doubleClickInterval)").Output()
	if err != nil {
		t.Skipf("no swift to cross-check with: %v", err)
	}
	want, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
	if err != nil {
		t.Skip("could not parse the cross-check")
	}
	if got != want {
		t.Errorf("GestureTuning().DoubleTap = %v, NSEvent.doubleClickInterval = %v", got, want)
	}
	if got <= 0 || got > 5 {
		t.Errorf("double-click interval %v is not a plausible number of seconds", got)
	}
}

// A native Mac app edits text with Cmd and Option. The provider used to rely
// on the iOS defaults for this and they did not carry it, so the field was
// false on the one desktop where it matters.
func TestMacProviderSelectsMacKeys(t *testing.T) {
	if !(&window{}).GestureTuning().MacKeys {
		t.Error("GestureTuning().MacKeys is false on macOS: Option+arrow would not " +
			"move by word and Cmd+arrow would not reach the line ends")
	}
}
