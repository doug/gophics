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
