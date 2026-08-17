//go:build windows

// The four interface vtables.
//
// Each is built once and shared by every element: a vtable is per-type, not
// per-object, so a tree of ten thousand nodes costs the same handful of
// syscall.NewCallback thunks as a tree of one. That matters because the
// process-wide supply of callbacks is finite.
//
// Every method recovers its element from `this` by subtracting its own slot's
// offset, and returns an HRESULT. A method that cannot answer returns an error
// code rather than guessing: UIA copes with a missing property, but a wrong one
// is read aloud.

package platform

import (
	"sync"
	"syscall"
	"unsafe"
)

var (
	vtSimpleOnce, vtFragmentOnce, vtRootOnce, vtPatternOnce sync.Once
	vtSimpleArr                                             [7]uintptr
	vtFragmentArr                                           [9]uintptr
	vtRootArr                                               [5]uintptr
	vtPatternArr                                            [5]uintptr
)

// vtSimple builds IRawElementProviderSimple: IUnknown, then get_ProviderOptions,
// GetPatternProvider, GetPropertyValue, get_HostRawElementProvider.
func vtSimple() uintptr {
	vtSimpleOnce.Do(func() {
		u := unknownSlots(offSimple)
		vtSimpleArr[0], vtSimpleArr[1], vtSimpleArr[2] = u[0], u[1], u[2]

		vtSimpleArr[3] = syscall.NewCallback(func(this, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			*(*int32)(unsafe.Pointer(pRetVal)) = providerOptionsServerSide
			return sOK
		})

		// GetPatternProvider(patternId, **IUnknown)
		vtSimpleArr[4] = syscall.NewCallback(func(this uintptr, patternID int32, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			out := (*uintptr)(unsafe.Pointer(pRetVal))
			*out = 0
			e := elemFrom(this, offSimple)
			if e == nil {
				return eElementNotAvailable
			}
			n, ok := e.node()
			if !ok {
				return sOK // no node: no patterns, but not an error
			}
			switch patternID {
			case patternInvoke:
				if supportsInvoke(n) {
					e.addRef()
					*out = e.base() + offPattern
				}
			case patternToggle:
				if supportsToggle(n) {
					e.addRef()
					*out = e.base() + offPattern
				}
			}
			return sOK
		})

		// GetPropertyValue(propertyId, *VARIANT)
		vtSimpleArr[5] = syscall.NewCallback(func(this uintptr, propID int32, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			v := (*variant)(unsafe.Pointer(pRetVal))
			v.setEmpty()
			e := elemFrom(this, offSimple)
			if e == nil {
				return eElementNotAvailable
			}
			e.property(propID, v)
			return sOK
		})

		// get_HostRawElementProvider(**IRawElementProviderSimple)
		//
		// Only the window element has a host. Returning the HWND's own provider
		// is what merges our tree into the desktop's — without it the elements
		// exist but belong to no window, and a screen reader never finds them.
		vtSimpleArr[6] = syscall.NewCallback(func(this, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			out := (*uintptr)(unsafe.Pointer(pRetVal))
			*out = 0
			e := elemFrom(this, offSimple)
			if e == nil || e.id != rootElemID {
				return sOK
			}
			procUiaHostProviderFromHwnd.Call(uintptr(e.prov.hwnd), uintptr(unsafe.Pointer(out)))
			return sOK
		})
	})
	return uintptr(unsafe.Pointer(&vtSimpleArr[0]))
}

// vtFragment builds IRawElementProviderFragment: IUnknown, Navigate,
// GetRuntimeId, get_BoundingRectangle, GetEmbeddedFragmentRoots, SetFocus,
// get_FragmentRoot.
func vtFragment() uintptr {
	vtFragmentOnce.Do(func() {
		u := unknownSlots(offFragment)
		vtFragmentArr[0], vtFragmentArr[1], vtFragmentArr[2] = u[0], u[1], u[2]

		// Navigate(direction, **IRawElementProviderFragment)
		vtFragmentArr[3] = syscall.NewCallback(func(this uintptr, dir int32, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			out := (*uintptr)(unsafe.Pointer(pRetVal))
			*out = 0
			e := elemFrom(this, offFragment)
			if e == nil {
				return eElementNotAvailable
			}
			if target := e.navigate(dir); target != nil {
				*out = target.base() + offFragment
			}
			return sOK
		})

		// GetRuntimeId(**SAFEARRAY)
		//
		// The ID must be stable for an element's lifetime and unique in the
		// process: UIA uses it to tell whether two provider pointers are the
		// same element, and a screen reader uses that to know whether focus
		// actually moved.
		vtFragmentArr[4] = syscall.NewCallback(func(this, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			out := (*uintptr)(unsafe.Pointer(pRetVal))
			*out = 0
			e := elemFrom(this, offFragment)
			if e == nil {
				return eElementNotAvailable
			}
			// UiaAppendRuntimeId (3) tells UIA to prefix the host's own ID.
			sa := safeArrayOfInt32([]int32{3, int32(e.id)})
			if sa == 0 {
				return eFail
			}
			*out = sa
			return sOK
		})

		// get_BoundingRectangle(*UiaRect) — screen coordinates, not client.
		vtFragmentArr[5] = syscall.NewCallback(func(this, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			r := (*uiaRect)(unsafe.Pointer(pRetVal))
			*r = uiaRect{}
			e := elemFrom(this, offFragment)
			if e == nil {
				return eElementNotAvailable
			}
			// The window element has no gophics node. It must still report a
			// real rectangle: UIA clips descendants to the fragment root, so a
			// zero-sized root silently empties the whole tree.
			if e.id == rootElemID {
				l, t, w, h := e.prov.windowRect()
				*r = uiaRect{left: l, top: t, width: w, height: h}
				uiaLogf("bounds root=(%v,%v %vx%v)", l, t, w, h)
				return sOK
			}
			n, ok := e.node()
			if !ok {
				return sOK
			}
			x, y := e.prov.clientToScreen(n.X, n.Y)
			*r = uiaRect{left: float64(x), top: float64(y), width: float64(n.W), height: float64(n.H)}
			uiaLogf("bounds id=%d node=(%d,%d %dx%d) screen=(%d,%d)", e.id, n.X, n.Y, n.W, n.H, x, y)
			return sOK
		})

		// GetEmbeddedFragmentRoots(**SAFEARRAY) — none; we are one fragment.
		vtFragmentArr[6] = syscall.NewCallback(func(this, pRetVal uintptr) uintptr {
			if pRetVal != 0 {
				*(*uintptr)(unsafe.Pointer(pRetVal)) = 0
			}
			return sOK
		})

		// SetFocus() — focus belongs to the widget tree, and nothing routes an
		// external request into it yet. Doing nothing beats claiming success.
		vtFragmentArr[7] = syscall.NewCallback(func(this uintptr) uintptr { return sOK })

		// get_FragmentRoot(**IRawElementProviderFragmentRoot)
		vtFragmentArr[8] = syscall.NewCallback(func(this, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			out := (*uintptr)(unsafe.Pointer(pRetVal))
			*out = 0
			e := elemFrom(this, offFragment)
			if e == nil {
				return eElementNotAvailable
			}
			root := e.prov.rootElem()
			*out = root.base() + offRoot
			return sOK
		})
	})
	return uintptr(unsafe.Pointer(&vtFragmentArr[0]))
}

// vtRoot builds IRawElementProviderFragmentRoot: IUnknown,
// ElementProviderFromPoint, GetFocus.
func vtRoot() uintptr {
	vtRootOnce.Do(func() {
		u := unknownSlots(offRoot)
		vtRootArr[0], vtRootArr[1], vtRootArr[2] = u[0], u[1], u[2]

		// ElementProviderFromPoint(x, y: double, **IRawElementProviderFragment)
		//
		// Declared without its arguments, and answering E_NOTIMPL, because Go
		// cannot express this signature: syscall.NewCallback rejects float
		// parameters outright ("compileCallback: float arguments not
		// supported"), and reinterpreting the bits is not an option either —
		// on x64 and ARM64 alike, doubles arrive in floating-point registers
		// that a Go callback never sees, so the integer registers hold
		// unrelated values.
		//
		// Omitting the parameters is safe: the caller passes them in registers
		// and cleans up its own stack, so a callee that ignores them is
		// well-formed. E_NOTIMPL then tells UIA to stop asking.
		//
		// What is lost is point hit-testing — "what is under the mouse", which
		// Narrator uses for touch and mouse exploration. Keyboard and focus
		// navigation, which is how a screen reader is mostly driven, go through
		// Navigate and GetFocus and are unaffected. The provider's own hitTest
		// is kept and tested, ready for a route that can pass the coordinates.
		vtRootArr[3] = syscall.NewCallback(func(this uintptr) uintptr {
			return eNotImpl
		})

		// GetFocus(**IRawElementProviderFragment)
		vtRootArr[4] = syscall.NewCallback(func(this, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			out := (*uintptr)(unsafe.Pointer(pRetVal))
			*out = 0
			e := elemFrom(this, offRoot)
			if e == nil {
				return eElementNotAvailable
			}
			if f := e.prov.focused(); f != nil {
				*out = f.base() + offFragment
			}
			return sOK
		})
	})
	return uintptr(unsafe.Pointer(&vtRootArr[0]))
}

// vtPattern builds a vtable serving both IInvokeProvider and IToggleProvider.
//
// They can share one table because their layouts do not collide: Invoke has one
// method after IUnknown, Toggle has Toggle plus get_ToggleState. A node offers
// at most one of the two — supportsInvoke excludes checkboxes — so a caller
// that queried for Invoke only ever calls slot 3, and one that queried for
// Toggle calls slots 3 and 4. Slot 3 therefore does whichever the node
// supports, which for a checkbox is a toggle and for a button an invoke.
func vtPattern() uintptr {
	vtPatternOnce.Do(func() {
		u := unknownSlots(offPattern)
		vtPatternArr[0], vtPatternArr[1], vtPatternArr[2] = u[0], u[1], u[2]

		// Invoke() / Toggle()
		vtPatternArr[3] = syscall.NewCallback(func(this uintptr) uintptr {
			e := elemFrom(this, offPattern)
			if e == nil {
				return eElementNotAvailable
			}
			n, ok := e.node()
			if !ok {
				return eElementNotAvailable
			}
			if !supportsInvoke(n) && !supportsToggle(n) {
				return eFail
			}
			e.prov.mu.RLock()
			activate := e.prov.activate
			e.prov.mu.RUnlock()
			if activate == nil {
				return eFail
			}
			// Runs on the UIA thread; the caller's activate marshals it, the
			// same contract every bridge gets (see a11y.go).
			activate(n.ID)
			return sOK
		})

		// get_ToggleState(*ToggleState)
		vtPatternArr[4] = syscall.NewCallback(func(this, pRetVal uintptr) uintptr {
			if pRetVal == 0 {
				return eInvalidArg
			}
			e := elemFrom(this, offPattern)
			if e == nil {
				return eElementNotAvailable
			}
			n, ok := e.node()
			if !ok {
				return eElementNotAvailable
			}
			*(*int32)(unsafe.Pointer(pRetVal)) = toggleState(n)
			return sOK
		})
	})
	return uintptr(unsafe.Pointer(&vtPatternArr[0]))
}

// safeArrayOfFloat64 builds a SAFEARRAY of VT_R8, used for BoundingRectangle.
func safeArrayOfFloat64(vals []float64) uintptr {
	sa, _, _ := procSafeArrayCreateVector.Call(uintptr(vtR8), 0, uintptr(len(vals)))
	if sa == 0 {
		return 0
	}
	for i := range vals {
		idx := int32(i)
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&vals[i])))
	}
	return sa
}

// safeArrayOfInt32 builds a SAFEARRAY of VT_I4, which is what GetRuntimeId
// must return.
func safeArrayOfInt32(vals []int32) uintptr {
	sa, _, _ := procSafeArrayCreateVector.Call(uintptr(vtI4), 0, uintptr(len(vals)))
	if sa == 0 {
		return 0
	}
	for i := range vals {
		idx := int32(i)
		procSafeArrayPutElement.Call(sa, uintptr(unsafe.Pointer(&idx)), uintptr(unsafe.Pointer(&vals[i])))
	}
	return sa
}
