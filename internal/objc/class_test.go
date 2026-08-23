//go:build darwin && !ios

package objc

import (
	"sync/atomic"
	"testing"
)

// The point of defining a class is that the Objective-C runtime will call back
// into Go. Everything else here is setup; this is the behaviour.
//
// Without it, capabilities the system calls into — camera frames, notification
// observers, window delegates — cannot be implemented at all, because there is
// no object to hand the framework.
func TestDefinedMethodCallsBackIntoGo(t *testing.T) {
	if err := Init(); err != nil {
		t.Skipf("Objective-C runtime unavailable: %v", err)
	}

	var got atomic.Int64
	cls, err := DefineClass("GophicsTestCallback", "NSObject")
	if err != nil {
		t.Fatalf("DefineClass: %v", err)
	}
	// v@:q — void, self, _cmd, one 64-bit integer.
	err = cls.AddMethod("takeNumber:", "v@:q", func(self, cmd, n uintptr) uintptr {
		got.Store(int64(n))
		return 0
	})
	if err != nil {
		t.Fatalf("AddMethod: %v", err)
	}
	if _, err := cls.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}

	obj := cls.New()
	if !obj.Valid() {
		t.Fatal("New returned nil; the class registered but cannot be instantiated")
	}
	obj.SendVoid("takeNumber:", Int(4242))

	if got.Load() != 4242 {
		t.Errorf("the Objective-C runtime did not reach the Go implementation: "+
			"got %d, want 4242", got.Load())
	}
}

// A class the runtime already knows must be refused rather than silently
// producing a nil class that swallows every message sent to it.
func TestDefineClassRejectsADuplicateName(t *testing.T) {
	if err := Init(); err != nil {
		t.Skipf("Objective-C runtime unavailable: %v", err)
	}
	const name = "GophicsTestDuplicate"
	c, err := DefineClass(name, "NSObject")
	if err != nil {
		t.Fatalf("first DefineClass: %v", err)
	}
	if _, err := c.Register(); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if _, err := DefineClass(name, "NSObject"); err == nil {
		t.Error("defining the same class name twice was allowed")
	}
}

// A superclass that does not exist is a caller error worth naming, usually a
// framework that was never loaded.
func TestDefineClassRejectsAMissingSuperclass(t *testing.T) {
	if err := Init(); err != nil {
		t.Skipf("Objective-C runtime unavailable: %v", err)
	}
	if _, err := DefineClass("GophicsTestNoSuper", "NoSuchSuperclassZZZ"); err == nil {
		t.Error("a missing superclass was accepted")
	}
}

// Methods cannot be added after registration; saying so beats the runtime
// quietly ignoring the call.
func TestAddMethodAfterRegisterIsRefused(t *testing.T) {
	if err := Init(); err != nil {
		t.Skipf("Objective-C runtime unavailable: %v", err)
	}
	c, err := DefineClass("GophicsTestSealed", "NSObject")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Register(); err != nil {
		t.Fatal(err)
	}
	if err := c.AddMethod("late:", "v@:q", func(a, b, x uintptr) uintptr { return 0 }); err == nil {
		t.Error("a method was added after registration")
	}
}
