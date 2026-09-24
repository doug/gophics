package mobile

import (
	"runtime"
	"testing"
)

// The Bridge answers by GOOS. An iPad with a hardware keyboard is a Mac
// keyboard; an Android device with one is a PC keyboard — the iOS tuning used
// to omit the field, so an iPad got Ctrl and Home/End editing.
func TestGestureTuningMacKeysFollowsThePlatform(t *testing.T) {
	got := NewBridge(nil).GestureTuning().MacKeys
	want := runtime.GOOS != "android"
	if got != want {
		t.Errorf("GestureTuning().MacKeys = %v on %s, want %v", got, runtime.GOOS, want)
	}
}
