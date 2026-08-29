package mobile

import (
	"errors"

	"github.com/doug/gophics/shell"
)

// Wake lock, photo-library writes and biometric authentication.
//
// Wake lock is the odd one: the lease bookkeeping is pure Go, and the host is
// told only when the count crosses zero. Doing it the other way — a host method
// per lease — would make every host reimplement reference counting, and getting
// it wrong there means a screen that never sleeps.

// DeviceHost is the platform side of the capabilities in this file.
type DeviceHost interface {
	// SetKeepAwake turns the screen-sleep block on or off. It is called only on
	// a change, never repeatedly with the same value: iOS sets
	// UIApplication.isIdleTimerDisabled, Android adds or clears
	// FLAG_KEEP_SCREEN_ON.
	SetKeepAwake(on bool)

	// AuthorizePhotos requests add-only photo-library access.
	// Answer with DeliverPhotosPermission(reqID, granted).
	AuthorizePhotos(reqID int)
	// SavePhoto writes encoded image bytes (PNG or JPEG) to the library, into
	// album when non-empty.
	// Answer with DeliverPhotoSaved(reqID, "") or a message.
	SavePhoto(reqID int, data []byte, album string)

	// BiometricKind reports what the device can ask for right now: 0 none,
	// 1 fingerprint, 2 face, 3 other (shell.BiometricKind). Synchronous, because
	// both platforms answer it from a local query.
	BiometricKind() int
	// Authenticate presents the platform prompt.
	// Answer with DeliverAuth(reqID, ok, errMsg).
	Authenticate(reqID int, reason string, allowFallback bool)
}

// SetDeviceHost registers the backend for WakeLock, Photos and Biometric.
func (b *Bridge) SetDeviceHost(h DeviceHost) { b.deviceHost = h; b.capabilitiesChanged() }

// ---- Wake lock ----

// WakeLock makes the Bridge a shell.WakeLockWindow.
func (b *Bridge) WakeLock() shell.WakeLock {
	if b.deviceHost == nil {
		return nil
	}
	return mobileWake{b}
}

type mobileWake struct{ b *Bridge }

func (w mobileWake) Held() bool { return w.b.wakeCount > 0 }

func (w mobileWake) Acquire(reason string) func() {
	b := w.b
	b.wakeCount++
	if b.wakeCount == 1 {
		b.deviceHost.SetKeepAwake(true)
	}
	released := false
	return func() {
		// Idempotent: releasing twice must not decrement twice, or two widgets
		// each holding one lease can leave the count negative and the screen
		// asleep while the other still needs it.
		if released {
			return
		}
		released = true
		b.wakeCount--
		if b.wakeCount == 0 {
			b.deviceHost.SetKeepAwake(false)
		}
	}
}

// ---- Photos ----

// Photos makes the Bridge a shell.PhotosWindow.
func (b *Bridge) Photos() shell.Photos {
	if b.deviceHost == nil {
		return nil
	}
	return mobilePhotos{b}
}

type mobilePhotos struct{ b *Bridge }

func (p mobilePhotos) Authorize(cb func(shell.Permission)) {
	b := p.b
	id := b.newReq()
	if cb != nil {
		if b.photoPermCb == nil {
			b.photoPermCb = map[int]func(shell.Permission){}
		}
		b.photoPermCb[id] = cb
	}
	b.deviceHost.AuthorizePhotos(id)
}

func (p mobilePhotos) Save(data []byte, album string, done func(error)) {
	b := p.b
	id := b.newReq()
	if done != nil {
		if b.photoSaveCb == nil {
			b.photoSaveCb = map[int]func(error){}
		}
		b.photoSaveCb[id] = done
	}
	b.deviceHost.SavePhoto(id, data, album)
}

// DeliverPhotosPermission answers an AuthorizePhotos request.
func (b *Bridge) DeliverPhotosPermission(reqID int, granted bool) {
	cb := b.photoPermCb[reqID]
	if cb == nil {
		return
	}
	delete(b.photoPermCb, reqID)
	if granted {
		cb(shell.PermissionGranted)
		return
	}
	cb(shell.PermissionDenied)
}

// DeliverPhotoSaved completes a Save: "" for success, otherwise the message.
func (b *Bridge) DeliverPhotoSaved(reqID int, errMsg string) {
	cb := b.photoSaveCb[reqID]
	if cb == nil {
		return
	}
	delete(b.photoSaveCb, reqID)
	if errMsg == "" {
		cb(nil)
		return
	}
	cb(errors.New(errMsg))
}

// ---- Biometric ----

// Biometric makes the Bridge a shell.BiometricWindow.
func (b *Bridge) Biometric() shell.Biometric {
	if b.deviceHost == nil {
		return nil
	}
	return mobileBiometric{b}
}

type mobileBiometric struct{ b *Bridge }

func (m mobileBiometric) Available() shell.BiometricKind {
	k := m.b.deviceHost.BiometricKind()
	if k < 0 || k > int(shell.BiometricOther) {
		return shell.BiometricNone
	}
	return shell.BiometricKind(k)
}

func (m mobileBiometric) Authenticate(reason string, allowFallback bool, done func(bool, error)) {
	b := m.b
	id := b.newReq()
	if done != nil {
		if b.authCb == nil {
			b.authCb = map[int]func(bool, error){}
		}
		b.authCb[id] = done
	}
	b.deviceHost.Authenticate(id, reason, allowFallback)
}

// DeliverAuth completes an Authenticate request. errMsg is "" when ok, and
// carries the reason otherwise — including cancellation, which is a negative
// outcome rather than an error the app should treat differently.
func (b *Bridge) DeliverAuth(reqID int, ok bool, errMsg string) {
	cb := b.authCb[reqID]
	if cb == nil {
		return
	}
	delete(b.authCb, reqID)
	if ok {
		cb(true, nil)
		return
	}
	if errMsg == "" {
		errMsg = "authentication failed"
	}
	cb(false, errors.New(errMsg))
}
