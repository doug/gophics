//go:build darwin && !ios

package objc

import (
	"fmt"
	"runtime"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// Defining Objective-C classes from Go, so a Go function can serve as a
// delegate method.
//
// Sending messages is enough for the capabilities that only ask the system for
// something — a file panel, a share sheet. It is not enough for the ones the
// system calls back into. AVFoundation delivers camera frames by calling
// -captureOutput:didOutputSampleBuffer:fromConnection: on an object you hand
// it, so capturing video means being able to supply an object that responds to
// that selector, which means defining a class at runtime.
//
// The pieces are objc_allocateClassPair to make one, class_addMethod to give it
// a method whose implementation is a goffi callback, and objc_registerClassPair
// to publish it. Nothing here is camera-specific; a notification observer or a
// window delegate needs exactly the same three calls.

var (
	classInitOnce sync.Once
	classInitErr  error

	symAllocateClassPair unsafe.Pointer
	symAddMethod         unsafe.Pointer
	symRegisterClassPair unsafe.Pointer

	cifAllocateClassPair types.CallInterface
	cifAddMethod         types.CallInterface
	cifRegisterClassPair types.CallInterface

	// definedClasses keeps the trampolines and encodings alive for the process
	// lifetime. A registered class is never unregistered — the runtime holds
	// pointers into it — so anything it references must outlive every caller.
	definedClasses sync.Map // string → *ClassDef
)

func classInit() error {
	classInitOnce.Do(func() { classInitErr = loadClassSyms() })
	return classInitErr
}

func loadClassSyms() error {
	if err := Init(); err != nil {
		return err
	}
	lib, err := ffi.LoadLibrary("/usr/lib/libobjc.A.dylib")
	if err != nil {
		return fmt.Errorf("objc: load libobjc: %w", err)
	}
	for _, s := range []struct {
		name string
		dst  *unsafe.Pointer
	}{
		{"objc_allocateClassPair", &symAllocateClassPair},
		{"class_addMethod", &symAddMethod},
		{"objc_registerClassPair", &symRegisterClassPair},
	} {
		p, err := ffi.GetSymbol(lib, s.name)
		if err != nil {
			return fmt.Errorf("objc: %s: %w", s.name, err)
		}
		*s.dst = p
	}

	ptr := types.PointerTypeDescriptor
	// Class objc_allocateClassPair(Class super, const char *name, size_t extra)
	if err := ffi.PrepareCallInterface(&cifAllocateClassPair, types.DefaultCall, ptr,
		[]*types.TypeDescriptor{ptr, ptr, types.UInt64TypeDescriptor}); err != nil {
		return fmt.Errorf("objc: prepare objc_allocateClassPair: %w", err)
	}
	// BOOL class_addMethod(Class cls, SEL name, IMP imp, const char *types)
	if err := ffi.PrepareCallInterface(&cifAddMethod, types.DefaultCall, types.UInt8TypeDescriptor,
		[]*types.TypeDescriptor{ptr, ptr, ptr, ptr}); err != nil {
		return fmt.Errorf("objc: prepare class_addMethod: %w", err)
	}
	// void objc_registerClassPair(Class cls)
	if err := ffi.PrepareCallInterface(&cifRegisterClassPair, types.DefaultCall,
		types.VoidTypeDescriptor, []*types.TypeDescriptor{ptr}); err != nil {
		return fmt.Errorf("objc: prepare objc_registerClassPair: %w", err)
	}
	return nil
}

// ClassDef is a runtime-defined Objective-C class.
type ClassDef struct {
	name string
	id   ID

	registered bool
	// keepAlive holds the C strings and trampolines the runtime keeps
	// pointing at. Losing them to the collector would leave the runtime
	// dereferencing freed memory the next time it dispatched a message.
	keepAlive []any
}

// DefineClass creates a subclass of super named name.
//
// The name must be unique in the process: Objective-C class names are global,
// and allocating one that already exists returns nil rather than erroring, so
// that case is reported here instead of surfacing later as a nil receiver
// silently swallowing every message.
//
// Call AddMethod for each method, then Register. A class cannot be modified
// after registration, which is why this is a two-step API rather than one call.
func DefineClass(name, super string) (*ClassDef, error) {
	if err := classInit(); err != nil {
		return nil, err
	}
	if _, exists := definedClasses.Load(name); exists {
		return nil, fmt.Errorf("objc: class %q is already defined in this process", name)
	}
	superCls := Class(super)
	if !superCls.Valid() {
		return nil, fmt.Errorf("objc: superclass %q not found — is its framework loaded?", super)
	}

	cname := append([]byte(name), 0)
	sup := uintptr(superCls)
	np := uintptr(unsafe.Pointer(&cname[0]))
	extra := uint64(0)

	var out ID
	args := []unsafe.Pointer{
		unsafe.Pointer(&sup), unsafe.Pointer(&np), unsafe.Pointer(&extra),
	}
	if _, err := ffi.CallFunction(&cifAllocateClassPair, symAllocateClassPair,
		unsafe.Pointer(&out), args); err != nil {
		return nil, fmt.Errorf("objc: objc_allocateClassPair(%s): %w", name, err)
	}
	runtime.KeepAlive(cname)
	if out == 0 {
		return nil, fmt.Errorf("objc: could not allocate class %q (name taken?)", name)
	}
	return &ClassDef{name: name, id: out, keepAlive: []any{cname}}, nil
}

// AddMethod gives the class a method implemented by a Go function.
//
// enc is the Objective-C type encoding, the runtime's description of the
// signature: "v@:@@@" is a void method taking self, _cmd and three objects,
// which is the shape of most delegate callbacks. Getting it wrong is not
// checked by the runtime and shows up as garbage arguments, so the encoding
// and fn's signature have to be kept in step by hand.
//
// fn must take uintptr for every argument including self and _cmd, matching
// goffi's callback contract.
func (c *ClassDef) AddMethod(sel, enc string, fn any) error {
	if c.registered {
		return fmt.Errorf("objc: %s is registered; methods cannot be added after that", c.name)
	}
	if err := classInit(); err != nil {
		return err
	}

	imp := ffi.NewCallback(fn)
	if imp == 0 {
		return fmt.Errorf("objc: could not make a trampoline for %s.%s", c.name, sel)
	}
	cenc := append([]byte(enc), 0)

	cls := uintptr(c.id)
	s := uintptr(Sel(sel))
	ip := imp
	ep := uintptr(unsafe.Pointer(&cenc[0]))

	var ok uint8
	args := []unsafe.Pointer{
		unsafe.Pointer(&cls), unsafe.Pointer(&s), unsafe.Pointer(&ip), unsafe.Pointer(&ep),
	}
	if _, err := ffi.CallFunction(&cifAddMethod, symAddMethod, unsafe.Pointer(&ok), args); err != nil {
		return fmt.Errorf("objc: class_addMethod(%s.%s): %w", c.name, sel, err)
	}
	if ok == 0 {
		return fmt.Errorf("objc: class_addMethod(%s.%s) refused — already defined?", c.name, sel)
	}
	// The runtime keeps the encoding string and the trampoline; both must
	// outlive every dispatch, which is forever.
	c.keepAlive = append(c.keepAlive, cenc, fn)
	return nil
}

// Register publishes the class and returns it. After this it can be
// instantiated and messaged, and no further methods may be added.
func (c *ClassDef) Register() (ID, error) {
	if c.registered {
		return c.id, nil
	}
	if err := classInit(); err != nil {
		return 0, err
	}
	cls := uintptr(c.id)
	args := []unsafe.Pointer{unsafe.Pointer(&cls)}
	if _, err := ffi.CallFunction(&cifRegisterClassPair, symRegisterClassPair, nil, args); err != nil {
		return 0, fmt.Errorf("objc: objc_registerClassPair(%s): %w", c.name, err)
	}
	c.registered = true
	definedClasses.Store(c.name, c)
	return c.id, nil
}

// ID returns the class object.
func (c *ClassDef) ID() ID { return c.id }

// New allocates and initialises an instance of the class.
func (c *ClassDef) New() ID {
	if !c.registered {
		return 0
	}
	return c.id.Send("alloc").Send("init")
}
