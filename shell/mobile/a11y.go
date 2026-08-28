package mobile

import (
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/shell"
)

// Accessibility makes the Bridge a shell.AccessibilityWindow.
//
// Mobile already had the *pull* half of accessibility and still does: iOS
// UIAccessibility and Android's AccessibilityNodeProvider query the app when
// their focus moves, and the A11y* accessors in mobile.go answer from the
// handler's A11yProvider. That is the right shape for both platforms and this
// does not disturb it.
//
// What was missing is Announce — live-region speech, which nothing pulls
// because it is an event rather than a state. A form that reports "3 errors" or
// a list that reports "12 results" had no way to say so on a phone.
func (b *Bridge) Accessibility() shell.Accessibility { return mobileA11y{b} }

type mobileA11y struct{ b *Bridge }

// Announce queues a message for the host to speak. The host drains it with
// TakeAnnouncement and posts it (iOS UIAccessibility.post(.announcement),
// Android View.announceForAccessibility).
func (a mobileA11y) Announce(message string, assertive bool) {
	if message == "" {
		return
	}
	a.b.announce = append(a.b.announce, announcement{msg: message, assertive: assertive})
}

// SetTree is deliberately a no-op: the host pulls this same tree through
// A11yRefresh and the accessors beside it, on the AT's schedule rather than
// ours. Accepting the push as well would keep a second copy that nothing reads.
//
// The capability is still published rather than withheld, because Announce is
// real; a caller that needs to know whether the tree is reaching the AT should
// ask the platform, not the shell.
func (mobileA11y) SetTree([]app.A11yNode, func(id int)) {}

type announcement struct {
	msg       string
	assertive bool
}

// TakeAnnouncement returns and clears the oldest pending announcement, or ""
// when there is none.
//
// AnnouncementAssertive describes the message this call returned, so the two
// are read in that order: take the text, then ask how to say it. gomobile
// allows a second result only when it is an error, which is why the priority
// cannot simply ride along with the string.
func (b *Bridge) TakeAnnouncement() string {
	if len(b.announce) == 0 {
		return ""
	}
	a := b.announce[0]
	b.announce = b.announce[1:]
	b.announceLast = a.assertive
	return a.msg
}

// AnnouncementAssertive reports whether the message most recently returned by
// TakeAnnouncement should interrupt current speech.
func (b *Bridge) AnnouncementAssertive() bool { return b.announceLast }
