//go:build darwin

package darwin

import "testing"

// The three globals are *data* symbols: dlsym gives the address of the pointer,
// so each needs a dereference to reach the NSString. Getting that wrong yields
// a plausible non-nil value that is not a string, AppKit ignores the post, and
// announcements silently never speak — which is exactly the failure this
// capability was documented as having before.
func TestAnnounceGlobalsResolve(t *testing.T) {
	if !loadAnnounce() {
		t.Fatal("could not resolve NSAccessibilityPostNotificationWithUserInfo and its keys")
	}
	for name, id := range map[string]ID{
		"NSAccessibilityAnnouncementRequestedNotification": nameAnnouncementRequested,
		"NSAccessibilityAnnouncementKey":                   keyAnnouncement,
		"NSAccessibilityPriorityKey":                       keyPriority,
	} {
		if id == 0 {
			t.Errorf("%s resolved to nil", name)
		}
	}
}

// A resolved global must be a real NSString, not just a non-nil address — the
// difference between one dereference and none.
func TestAnnounceGlobalsAreStrings(t *testing.T) {
	if !loadAnnounce() {
		t.Skip("AppKit unavailable")
	}
	if got := NSStringLength(keyAnnouncement); got == 0 {
		t.Error("NSAccessibilityAnnouncementKey has zero length, so it is not an NSString")
	}
	if got := NSStringLength(nameAnnouncementRequested); got == 0 {
		t.Error("the notification name has zero length, so it is not an NSString")
	}
}

// Posting must not crash, and must decline rather than post nonsense when it
// has nothing to say.
func TestPostAnnouncementGuards(t *testing.T) {
	if postAnnouncement(0, "hello", false) {
		t.Error("posted against a nil element")
	}
	view := ID(GetClass("NSView")).Send(RegisterSelector("alloc")).Send(RegisterSelector("init"))
	if view == 0 {
		t.Skip("could not create an NSView")
	}
	if postAnnouncement(view, "", false) {
		t.Error("posted an empty announcement")
	}
	// A real post against a detached view: VoiceOver may ignore it, but the
	// call itself must succeed, which is what exercises the FFI path.
	if !postAnnouncement(view, "five results", false) {
		t.Error("a well-formed announcement failed to post")
	}
	if !postAnnouncement(view, "upload failed", true) {
		t.Error("an assertive announcement failed to post")
	}
}
