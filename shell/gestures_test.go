package shell

import "testing"

// MacKeys is the one field in GestureTuning that is a convention rather than a
// measurement, and the platform defaults are where it is decided. Neither
// native Apple shell set it — only the web shell did, from the user agent — so
// a Mac app and an iPad with a keyboard edited text with Ctrl and Home/End
// while the same app in Safari used Cmd and Option.
func TestPlatformDefaultsDecideMacKeys(t *testing.T) {
	if !IOSGestureTuning().MacKeys {
		t.Error("IOSGestureTuning().MacKeys is false; a hardware keyboard on an " +
			"iPad is a Mac keyboard, and the macOS shell starts from these values")
	}
	if AndroidGestureTuning().MacKeys {
		t.Error("AndroidGestureTuning().MacKeys is true; a keyboard on Android is a PC keyboard")
	}
}

// Resolved fills the measured fields and must leave the convention alone in
// both directions — it has no default to fill, and a false is an answer.
func TestResolvedPreservesMacKeys(t *testing.T) {
	if !(GestureTuning{MacKeys: true}).Resolved().MacKeys {
		t.Error("Resolved dropped MacKeys=true")
	}
	if (GestureTuning{}).Resolved().MacKeys {
		t.Error("Resolved invented MacKeys=true")
	}
	if got := (GestureTuning{}).Resolved(); got.TouchSlop != 10 || got.LongPress != 0.5 || got.DoubleTap != 0.3 {
		t.Errorf("Resolved zero value = %+v, want the documented defaults", got)
	}
}
