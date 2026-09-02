//go:build js && wasm

// Async plumbing for the web shell: the three ways a JS result comes back, in
// one place, with the rule for choosing between them stated once.
//
// The rule is about goroutines. On wasm the JS event loop and the Go scheduler
// share one thread; a goroutine that blocks hands control back to the event
// loop, which is what makes await safe — and the event-loop goroutine itself
// must never block on a promise, because the settle it is waiting for can only
// be delivered by the loop it is holding. Capability entry points are called
// from that goroutine (an event handler, a Build), so:
//
//   - await: for sequential logic that already runs on a spawned goroutine
//     (`go func() { ... }`). Reads top to bottom; the right tool for loops
//     like "for each picked file, read it".
//   - onSettled: for promise results delivered by callback, safe from any
//     goroutine including the event loop. The right tool at entry points that
//     must not block — a user-gesture path that has already spent its gesture.
//   - onRequest: onSettled for IndexedDB, which predates promises and reports
//     through onsuccess/onerror events instead.
package web

import (
	"errors"
	"syscall/js"
)

// await blocks the calling goroutine until p settles, returning its value or a
// rejection error. It must run on a spawned goroutine, never the event-loop
// one — see the package comment on this file.
func await(p js.Value) (js.Value, error) {
	type result struct {
		v   js.Value
		err error
	}
	ch := make(chan result, 1)
	onSettled(p, func(v js.Value, err error) { ch <- result{v, err} })
	r := <-ch
	return r.v, r.err
}

// onSettled delivers p's outcome to done without blocking the caller. done is
// invoked from the event-loop goroutine; capability callbacks that reach app
// code are re-marshaled by the generated Posted wrappers, so implementations
// need not care.
func onSettled(p js.Value, done func(js.Value, error)) {
	var then, catch js.Func
	release := func() { then.Release(); catch.Release() }

	then = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer release()
		var v js.Value
		if len(args) > 0 {
			v = args[0]
		}
		done(v, nil)
		return nil
	})
	catch = js.FuncOf(func(_ js.Value, args []js.Value) any {
		defer release()
		e := &domError{msg: "promise rejected"}
		if len(args) > 0 && args[0].Truthy() {
			r := args[0]
			if n := r.Get("name"); n.Type() == js.TypeString {
				e.name = n.String()
			}
			if m := r.Get("message"); m.Type() == js.TypeString && m.String() != "" {
				e.msg = m.String()
			} else {
				e.msg = r.Call("toString").String()
			}
		}
		done(js.Value{}, e)
		return nil
	})

	p.Call("then", then).Call("catch", catch)
}

// onRequest delivers one IDBRequest's outcome, releasing its handlers once.
func onRequest(req js.Value, done func(js.Value, error)) {
	var success, failure js.Func
	release := func() { success.Release(); failure.Release() }

	success = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		release()
		done(req.Get("result"), nil)
		return nil
	})
	failure = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		release()
		done(js.Value{}, errors.New("web: indexedDB request failed"))
		return nil
	})
	req.Set("onsuccess", success)
	req.Set("onerror", failure)
}

// domError carries a DOMException's name so callers can tell a cancelled
// picker or an absent file from a real failure. The name is the only part of a
// DOM rejection stable enough to branch on; the message is for humans.
type domError struct {
	name string
	msg  string
}

func (e *domError) Error() string {
	if e.name == "" {
		return "web: " + e.msg
	}
	return "web: " + e.name + ": " + e.msg
}

// isDOMError reports whether err is a DOMException with the given name.
func isDOMError(err error, name string) bool {
	d, ok := err.(*domError)
	return ok && d.name == name
}
