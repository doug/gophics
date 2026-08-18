//go:build darwin

package darwin

import (
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// Live-region announcements.
//
// AppKit delivers these through NSAccessibilityPostNotificationWithUserInfo, a
// plain C function rather than an Objective-C method, so it needs an FFI call
// interface instead of the objc_msgSend path the rest of this package uses.
// Its notification name and the two userInfo keys are global NSString
// constants, which are *data* symbols: dlsym yields the address of the pointer,
// so each needs one dereference to reach the NSString itself. Getting that
// wrong yields a plausible non-nil value that is not a string, and AppKit
// silently ignores the post — which is the failure mode to watch for here.

var (
	announceOnce sync.Once
	announceOK   bool

	symPostNotification unsafe.Pointer
	cifPostNotification types.CallInterface

	nameAnnouncementRequested ID
	keyAnnouncement           ID
	keyPriority               ID
)

// NSAccessibilityPriorityLevel.
const (
	priorityMedium = 50
	priorityHigh   = 90
)

// loadAnnounce resolves the C function and the three global NSStrings.
func loadAnnounce() bool {
	announceOnce.Do(func() {
		lib, err := ffi.LoadLibrary("/System/Library/Frameworks/AppKit.framework/AppKit")
		if err != nil {
			return
		}
		symPostNotification, err = ffi.GetSymbol(lib, "NSAccessibilityPostNotificationWithUserInfo")
		if err != nil {
			return
		}
		ptr := []*types.TypeDescriptor{
			types.PointerTypeDescriptor, // element
			types.PointerTypeDescriptor, // notification name
			types.PointerTypeDescriptor, // userInfo
		}
		if err := ffi.PrepareCallInterface(&cifPostNotification, types.DefaultCall,
			types.VoidTypeDescriptor, ptr); err != nil {
			return
		}

		// Data symbols: the address holds a pointer to the NSString.
		global := func(name string) ID {
			s, err := ffi.GetSymbol(lib, name)
			if err != nil || s == nil {
				return 0
			}
			return ID(*(*uintptr)(s))
		}
		nameAnnouncementRequested = global("NSAccessibilityAnnouncementRequestedNotification")
		keyAnnouncement = global("NSAccessibilityAnnouncementKey")
		keyPriority = global("NSAccessibilityPriorityKey")

		announceOK = nameAnnouncementRequested != 0 &&
			keyAnnouncement != 0 && keyPriority != 0
	})
	return announceOK
}

// postAnnouncement speaks message through VoiceOver, against element.
//
// assertive raises the priority so the message interrupts rather than queues —
// the difference between "an error you must hear now" and "five results", and
// the reason the capability carries the flag at all.
func postAnnouncement(element ID, message string, assertive bool) bool {
	if element == 0 || message == "" || !loadAnnounce() {
		return false
	}
	initA11ySelectors()

	priority := int64(priorityMedium)
	if assertive {
		priority = priorityHigh
	}
	num := ID(GetClass("NSNumber")).SendInt(a11ySels.numberWithInteger, priority)
	if num == 0 {
		return false
	}

	// A two-entry NSDictionary: the text, and how loudly to say it.
	// arrayWithObjects:count: rather than the nil-terminated variadic form,
	// which cannot be expressed through a fixed-arity message send.
	objVals := [2]ID{createNSString(message), num}
	keyVals := [2]ID{keyAnnouncement, keyPriority}
	objs := ID(GetClass("NSArray")).SendUintUint(a11ySels.arrayWithObjectsCount,
		uint64(uintptr(unsafe.Pointer(&objVals[0]))), 2)
	keys := ID(GetClass("NSArray")).SendUintUint(a11ySels.arrayWithObjectsCount,
		uint64(uintptr(unsafe.Pointer(&keyVals[0]))), 2)
	if objs == 0 || keys == 0 {
		return false
	}
	info := ID(GetClass("NSDictionary")).SendUintUint(a11ySels.dictWithObjectsForKeys,
		uint64(objs), uint64(keys))
	if info == 0 {
		return false
	}
	runtime.KeepAlive(objVals)
	runtime.KeepAlive(keyVals)

	el, name, ui := uintptr(element), uintptr(nameAnnouncementRequested), uintptr(info)
	args := []unsafe.Pointer{
		unsafe.Pointer(&el),
		unsafe.Pointer(&name),
		unsafe.Pointer(&ui),
	}
	if _, err := ffi.CallFunction(&cifPostNotification, symPostNotification, nil, args); err != nil {
		return false
	}
	return true
}
