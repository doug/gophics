//go:build darwin && !ios

// Package objc is a minimal Objective-C runtime bridge for the shell layer's
// macOS capability implementations (file dialogs, share sheets, notifications).
//
// It is deliberately small: dlopen the Objective-C runtime, look up classes and
// selectors, and send messages through objc_msgSend — all via the pure-Go
// goffi FFI, so the tree stays **zero CGo**. The per-capability native body then
// reads like the Objective-C it mirrors:
//
//	panel := objc.Class("NSOpenPanel").Send("openPanel")
//	panel.SendVoid("setCanChooseFiles:", objc.Bool(true))
//	urls := panel.Send("URLs")
//
// Threading: AppKit requires the main thread. Send does not marshal — callers
// that may run off the main thread should use OnMain (which routes through
// -performSelectorOnMainThread:withObject:waitUntilDone:).
package objc

import (
	"errors"
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// ID is an Objective-C object pointer (id). The zero value is nil.
type ID uintptr

// SEL is a registered Objective-C selector.
type SEL uintptr

// Valid reports whether the object is non-nil.
func (o ID) Valid() bool { return o != 0 }

var (
	initOnce sync.Once
	initErr  error

	symMsgSend     unsafe.Pointer
	symGetClass    unsafe.Pointer
	symSelRegister unsafe.Pointer

	cifGetClass    types.CallInterface
	cifSelRegister types.CallInterface

	selCache sync.Map // string → SEL

	// frameworks keeps dlopen handles alive; loading a framework is what makes
	// its classes visible to objc_getClass.
	frameworks sync.Map // string → struct{}
)

// Init loads the Objective-C runtime. It is called automatically by Class and
// Send; call it directly to surface a load failure early.
func Init() error {
	initOnce.Do(func() { initErr = load() })
	return initErr
}

func load() error {
	lib, err := ffi.LoadLibrary("/usr/lib/libobjc.A.dylib")
	if err != nil {
		return fmt.Errorf("objc: load libobjc: %w", err)
	}
	if symMsgSend, err = ffi.GetSymbol(lib, "objc_msgSend"); err != nil {
		return fmt.Errorf("objc: objc_msgSend: %w", err)
	}
	if symGetClass, err = ffi.GetSymbol(lib, "objc_getClass"); err != nil {
		return fmt.Errorf("objc: objc_getClass: %w", err)
	}
	if symSelRegister, err = ffi.GetSymbol(lib, "sel_registerName"); err != nil {
		return fmt.Errorf("objc: sel_registerName: %w", err)
	}
	// Foundation, always. NSObject lives in libobjc, but everything this
	// package hands callers — String, NewArray, the NSNumber unwrapping —
	// is Foundation, and objc_getClass only sees classes from images that are
	// loaded.
	//
	// Until Go 1.27 something in the runtime pulled CoreFoundation in, so this
	// worked by accident: Class("NSString") resolved in any process. It stopped,
	// and the failure is quiet — Class returns nil and every Send on it is a
	// no-op, so a battery readout simply reported nothing rather than erroring.
	// Loading it here is a dlopen of an already-resident system image.
	if err := loadFramework("Foundation"); err != nil {
		return fmt.Errorf("objc: load Foundation: %w", err)
	}

	ptr := []*types.TypeDescriptor{types.PointerTypeDescriptor}
	if err := ffi.PrepareCallInterface(&cifGetClass, types.DefaultCall, types.PointerTypeDescriptor, ptr); err != nil {
		return fmt.Errorf("objc: prepare objc_getClass: %w", err)
	}
	if err := ffi.PrepareCallInterface(&cifSelRegister, types.DefaultCall, types.PointerTypeDescriptor, ptr); err != nil {
		return fmt.Errorf("objc: prepare sel_registerName: %w", err)
	}
	return nil
}

// LoadFramework dlopens a system framework by name (e.g. "AppKit"), making its
// classes resolvable. Repeat calls for the same name are no-ops.
func LoadFramework(name string) error {
	if err := Init(); err != nil {
		return err
	}
	return loadFramework(name)
}

// loadFramework is LoadFramework without the Init, so load can call it while
// initOnce is still running — going through the exported one there would
// deadlock on the very Once that is executing.
func loadFramework(name string) error {
	if _, done := frameworks.Load(name); done {
		return nil
	}
	path := fmt.Sprintf("/System/Library/Frameworks/%s.framework/%s", name, name)
	if _, err := ffi.LoadLibrary(path); err != nil {
		return fmt.Errorf("objc: load %s: %w", name, err)
	}
	frameworks.Store(name, struct{}{})
	return nil
}

// Class returns the class object for name, or 0 if the class is not registered
// (its framework may need LoadFramework first).
func Class(name string) ID {
	if Init() != nil {
		return 0
	}
	cname := append([]byte(name), 0)
	arg := uintptr(unsafe.Pointer(&cname[0]))
	var out ID
	args := [1]unsafe.Pointer{unsafe.Pointer(&arg)}
	_, _ = ffi.CallFunction(&cifGetClass, symGetClass, unsafe.Pointer(&out), args[:])
	runtime.KeepAlive(cname)
	return out
}

// Sel registers (and caches) a selector by name.
func Sel(name string) SEL {
	if cached, ok := selCache.Load(name); ok {
		return cached.(SEL)
	}
	if Init() != nil {
		return 0
	}
	cname := append([]byte(name), 0)
	arg := uintptr(unsafe.Pointer(&cname[0]))
	var out SEL
	args := [1]unsafe.Pointer{unsafe.Pointer(&arg)}
	_, _ = ffi.CallFunction(&cifSelRegister, symSelRegister, unsafe.Pointer(&out), args[:])
	runtime.KeepAlive(cname)
	selCache.Store(name, out)
	return out
}

// Arg is one message argument, carrying its FFI type and a pointer to the value.
type Arg struct {
	typ  *types.TypeDescriptor
	ptr  unsafe.Pointer
	keep any
}

// Obj passes an object pointer.
func Obj(o ID) Arg {
	v := uintptr(o)
	return Arg{types.PointerTypeDescriptor, unsafe.Pointer(&v), &v}
}

// Bool passes a BOOL.
func Bool(b bool) Arg {
	var v uint8
	if b {
		v = 1
	}
	return Arg{types.UInt8TypeDescriptor, unsafe.Pointer(&v), &v}
}

// Int passes an NSInteger.
func Int(i int64) Arg {
	v := i
	return Arg{types.SInt64TypeDescriptor, unsafe.Pointer(&v), &v}
}

// UInt passes an NSUInteger.
func UInt(u uint64) Arg {
	v := u
	return Arg{types.UInt64TypeDescriptor, unsafe.Pointer(&v), &v}
}

// ptrArg passes a raw C pointer (e.g. a NUL-terminated byte buffer).
func ptrArg(p unsafe.Pointer, keep any) Arg {
	v := uintptr(p)
	return Arg{types.PointerTypeDescriptor, unsafe.Pointer(&v), keep}
}

// send is the one call path: objc_msgSend(obj, sel, args...) with an explicit
// return type. A fresh call interface is prepared per call — file dialogs and
// share sheets are user-initiated, so the cost is irrelevant next to the clarity.
func send(obj ID, sel SEL, ret *types.TypeDescriptor, out unsafe.Pointer, args ...Arg) error {
	if obj == 0 || sel == 0 {
		return errors.New("objc: nil receiver or selector")
	}
	if err := Init(); err != nil {
		return err
	}
	argTypes := make([]*types.TypeDescriptor, 2+len(args))
	argTypes[0] = types.PointerTypeDescriptor // self
	argTypes[1] = types.PointerTypeDescriptor // _cmd
	for i, a := range args {
		argTypes[2+i] = a.typ
	}
	cif := &types.CallInterface{}
	if err := ffi.PrepareCallInterface(cif, types.DefaultCall, ret, argTypes); err != nil {
		return err
	}
	self, cmd := uintptr(obj), uintptr(sel)
	ptrs := make([]unsafe.Pointer, 2+len(args))
	ptrs[0] = unsafe.Pointer(&self)
	ptrs[1] = unsafe.Pointer(&cmd)
	for i, a := range args {
		ptrs[2+i] = a.ptr
	}
	_, err := ffi.CallFunction(cif, symMsgSend, out, ptrs)
	runtime.KeepAlive(args)
	return err
}

// Send sends a message returning an object.
func (o ID) Send(sel string, args ...Arg) ID {
	var out ID
	_ = send(o, Sel(sel), types.PointerTypeDescriptor, unsafe.Pointer(&out), args...)
	return out
}

// SendVoid sends a message with no return value.
func (o ID) SendVoid(sel string, args ...Arg) {
	_ = send(o, Sel(sel), types.VoidTypeDescriptor, nil, args...)
}

// SendInt sends a message returning an NSInteger.
func (o ID) SendInt(sel string, args ...Arg) int64 {
	var out int64
	_ = send(o, Sel(sel), types.SInt64TypeDescriptor, unsafe.Pointer(&out), args...)
	return out
}

// SendUInt sends a message returning an NSUInteger (e.g. -count).
func (o ID) SendUInt(sel string, args ...Arg) uint64 {
	var out uint64
	_ = send(o, Sel(sel), types.UInt64TypeDescriptor, unsafe.Pointer(&out), args...)
	return out
}

// SendFloat sends a message returning a C float, such as
// -[GCControllerButtonInput value].
//
// The out buffer is deliberately wider than a float32: libffi writes small
// return values through an ffi_arg-sized slot, so handing it a bare 4-byte
// destination invites a write past it on ABIs that pad.
// SendDouble sends a message whose return is a double (an NSTimeInterval, a
// CGFloat on 64-bit). SendFloat reads the register as a float32 and would
// misread one.
func (o ID) SendDouble(sel string, args ...Arg) float64 {
	var out float64
	_ = send(o, Sel(sel), types.DoubleTypeDescriptor, unsafe.Pointer(&out), args...)
	return out
}

func (o ID) SendFloat(sel string, args ...Arg) float32 {
	var out struct {
		v float32
		_ [4]byte
	}
	_ = send(o, Sel(sel), types.FloatTypeDescriptor, unsafe.Pointer(&out), args...)
	return out.v
}

// SendBool sends a message returning a BOOL.
func (o ID) SendBool(sel string, args ...Arg) bool {
	var out uint8
	_ = send(o, Sel(sel), types.UInt8TypeDescriptor, unsafe.Pointer(&out), args...)
	return out != 0
}

// OnMain performs a zero-argument selector on the main thread, waiting for it to
// finish. AppKit requires the main thread; when the caller already is the main
// thread the message is delivered inline.
func (o ID) OnMain(sel string) {
	o.SendVoid("performSelectorOnMainThread:withObject:waitUntilDone:",
		SelArg(Sel(sel)), Obj(0), Bool(true))
}

// SelArg passes a SEL as an argument (for the performSelector: family).
func SelArg(s SEL) Arg {
	v := uintptr(s)
	return Arg{types.PointerTypeDescriptor, unsafe.Pointer(&v), &v}
}

// String creates an autoreleased NSString from a Go string.
func String(s string) ID {
	cls := Class("NSString")
	if !cls.Valid() {
		return 0
	}
	b := append([]byte(s), 0)
	return cls.Send("stringWithUTF8String:", ptrArg(unsafe.Pointer(&b[0]), b))
}

// GoString reads an NSString as a Go string (via -UTF8String).
func GoString(s ID) string {
	if !s.Valid() {
		return ""
	}
	var p unsafe.Pointer
	if err := send(s, Sel("UTF8String"), types.PointerTypeDescriptor, unsafe.Pointer(&p)); err != nil || p == nil {
		return ""
	}
	return cstring(p)
}

// cstring copies a NUL-terminated C string into a Go string. The pointer targets
// memory owned by the Objective-C runtime (an NSString's UTF-8 buffer), never the
// Go heap, so it is stable for the duration of the copy; unsafe.Add walks it
// without ever round-tripping through uintptr.
func cstring(p unsafe.Pointer) string {
	if p == nil {
		return ""
	}
	var n int
	for *(*byte)(unsafe.Add(p, n)) != 0 {
		n++
	}
	if n == 0 {
		return ""
	}
	return string(unsafe.Slice((*byte)(p), n))
}

// Array reads an NSArray of objects into a Go slice.
func Array(a ID) []ID {
	if !a.Valid() {
		return nil
	}
	n := a.SendUInt("count")
	out := make([]ID, 0, n)
	for i := range n {
		out = append(out, a.Send("objectAtIndex:", UInt(i)))
	}
	return out
}

// NewArray builds an autoreleased NSArray from objects.
func NewArray(objs ...ID) ID {
	cls := Class("NSMutableArray")
	if !cls.Valid() {
		return 0
	}
	arr := cls.Send("array")
	for _, o := range objs {
		arr.SendVoid("addObject:", Obj(o))
	}
	return arr
}
