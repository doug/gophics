//go:build darwin && !ios && !js

// macOS implementation of the shell battery capability (shell/battery.go),
// reading IOKit's power-source API through the pure-Go FFI bridge. No CGo.
//
// Only three symbols are called directly — IOPSCopyPowerSourcesInfo,
// IOPSCopyPowerSourcesList and IOPSGetPowerSourceDescription. Everything they
// return is toll-free bridged: the CFArray *is* an NSArray and the
// CFDictionary *is* an NSDictionary, the same objects under two names. So the
// values come out through internal/objc rather than through a second hand-rolled
// binding for CFArrayGetValueAtIndex, CFDictionaryGetValue, CFNumberGetValue and
// CFStringCompare. That is the difference between four calls and a dozen, and
// the dozen are the fiddly ones — CFNumberGetValue alone needs a type constant
// and an out-parameter.
//
// CFRelease is the exception: the Copy functions return owned references, and
// only genuinely bridged classes are safe to -release. The blob is opaque, so it
// gets the real CFRelease.

package desktop

import (
	"sync"
	"time"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"

	"github.com/doug/gophics/internal/objc"
	"github.com/doug/gophics/shell"
)

var (
	ioKitOnce sync.Once
	ioKitOK   bool

	symPSInfo, symPSList, symPSDesc, symCFRelease unsafe.Pointer

	// The description keys, created once and retained. objc.String returns an
	// autoreleased NSString, and there is no autorelease pool on the goroutine
	// that polls the battery, so building them per read would leak steadily.
	keyCurrent, keyMax, keyState objc.ID
)

// loadIOKit resolves the symbols and the constant keys. Failure is not an
// error to the caller: it means no battery capability, which is the same
// answer a desktop Mac gives.
func loadIOKit() bool {
	ioKitOnce.Do(func() {
		cf, err := ffi.LoadLibrary("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		if err != nil {
			return
		}
		io, err := ffi.LoadLibrary("/System/Library/Frameworks/IOKit.framework/IOKit")
		if err != nil {
			return
		}
		if symCFRelease, err = ffi.GetSymbol(cf, "CFRelease"); err != nil {
			return
		}
		if symPSInfo, err = ffi.GetSymbol(io, "IOPSCopyPowerSourcesInfo"); err != nil {
			return
		}
		if symPSList, err = ffi.GetSymbol(io, "IOPSCopyPowerSourcesList"); err != nil {
			return
		}
		if symPSDesc, err = ffi.GetSymbol(io, "IOPSGetPowerSourceDescription"); err != nil {
			return
		}
		// Retained so they outlive any autorelease pool that may be draining
		// on whichever thread first touches the battery.
		keyCurrent = objc.String("Current Capacity").Send("retain")
		keyMax = objc.String("Max Capacity").Send("retain")
		keyState = objc.String("Power Source State").Send("retain")
		ioKitOK = keyCurrent.Valid() && keyMax.Valid() && keyState.Valid()
	})
	return ioKitOK
}

// callPtr calls fn with argc pointer-sized arguments, returning a pointer.
// A call interface is prepared per call, as internal/objc does: battery reads
// happen at most once a second, so clarity beats caching a CIF.
func callPtr(fn unsafe.Pointer, args ...uintptr) uintptr {
	argTypes := make([]*types.TypeDescriptor, len(args))
	ptrs := make([]unsafe.Pointer, len(args))
	for i := range args {
		argTypes[i] = types.PointerTypeDescriptor
		ptrs[i] = unsafe.Pointer(&args[i])
	}
	cif := &types.CallInterface{}
	if ffi.PrepareCallInterface(cif, types.DefaultCall, types.PointerTypeDescriptor, argTypes) != nil {
		return 0
	}
	var out uintptr
	if _, err := ffi.CallFunction(cif, fn, unsafe.Pointer(&out), ptrs); err != nil {
		return 0
	}
	return out
}

// cfRelease drops an owned CoreFoundation reference.
func cfRelease(ref uintptr) {
	if ref == 0 {
		return
	}
	cif := &types.CallInterface{}
	at := []*types.TypeDescriptor{types.PointerTypeDescriptor}
	if ffi.PrepareCallInterface(cif, types.DefaultCall, types.VoidTypeDescriptor, at) != nil {
		return
	}
	_, _ = ffi.CallFunction(cif, symCFRelease, nil, []unsafe.Pointer{unsafe.Pointer(&ref)})
}

// reading is one sample of the power source.
type reading struct {
	level    float32
	charging bool
	present  bool
}

// readPowerSource samples IOKit. It walks every power source and takes the
// first with a usable capacity: a Mac may also list a UPS or a Bluetooth
// peripheral, and those are not what ctx.Battery() means.
func readPowerSource() reading {
	if !loadIOKit() {
		return reading{}
	}
	blob := callPtr(symPSInfo)
	if blob == 0 {
		return reading{}
	}
	defer cfRelease(blob)

	list := callPtr(symPSList, blob)
	if list == 0 {
		return reading{}
	}
	defer cfRelease(list)

	for _, ps := range objc.Array(objc.ID(list)) {
		// Get, not Copy: the description is not owned, so it is not released.
		desc := objc.ID(callPtr(symPSDesc, blob, uintptr(ps)))
		if r, ok := readDescription(desc); ok {
			return r
		}
	}
	return reading{}
}

// readDescription pulls the level and charging state out of one power-source
// description dictionary, reporting false if it does not describe a battery
// with a usable capacity — a UPS or a Bluetooth peripheral can appear in the
// same list, and those are not what ctx.Battery() means.
//
// Split from readPowerSource so it can be tested: the machines that run this
// suite may have no battery at all (a Mac Studio does not), which would leave
// the key lookups and the number conversion — the parts that fail quietly and
// wrongly — never executed.
func readDescription(desc objc.ID) (reading, bool) {
	if !desc.Valid() {
		return reading{}, false
	}
	cur := desc.Send("objectForKey:", objc.Obj(keyCurrent))
	max := desc.Send("objectForKey:", objc.Obj(keyMax))
	if !cur.Valid() || !max.Valid() {
		return reading{}, false
	}
	m := max.SendInt("integerValue")
	if m <= 0 {
		return reading{}, false
	}
	c := max64(0, min64(cur.SendInt("integerValue"), m))
	// "AC Power" means mains, matching the other platforms: a caller asking
	// this wants to know whether to worry about running out, and a full
	// battery on mains is not a worry.
	state := objc.GoString(desc.Send("objectForKey:", objc.Obj(keyState)))
	return reading{level: float32(c) / float32(m), charging: state == "AC Power", present: true}, true
}

func min64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

// Battery makes the desktop window a shell.BatteryWindow, or returns nil on a
// Mac with no internal battery — a Studio or a Mini — so callers hide the
// affordance rather than showing a fabricated full charge.
func (w *window) Battery() shell.Battery {
	if !readPowerSource().present {
		return nil
	}
	return &darwinBattery{}
}

// sampleTTL bounds how often IOKit is asked. Level() may be called from Build,
// so it can run every frame; a power-source copy per frame would be wasteful
// for a value that moves over minutes.
const sampleTTL = time.Second

type darwinBattery struct {
	batteryWatcher

	// The sample cache has its own lock: it is taken on every Level() call,
	// which can be every frame, and must not contend with the watcher's.
	cacheMu sync.Mutex
	last    reading
	at      time.Time
}

func (b *darwinBattery) sample() reading {
	b.cacheMu.Lock()
	defer b.cacheMu.Unlock()
	if !b.at.IsZero() && time.Since(b.at) < sampleTTL {
		return b.last
	}
	b.last, b.at = readPowerSource(), time.Now()
	return b.last
}

func (b *darwinBattery) Level() float32 { return b.sample().level }

func (b *darwinBattery) Charging() bool { return b.sample().charging }

// OnChange registers f, called when the level or charging state changes.
func (b *darwinBattery) OnChange(f func()) {
	b.watch(f, func() (float32, bool) { return b.Level(), b.Charging() })
}
