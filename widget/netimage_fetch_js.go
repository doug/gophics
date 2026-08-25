//go:build js

package widget

import (
	"bytes"
	"fmt"
	"image"
	"syscall/js"
)

// do fetches url via the browser's fetch() API directly instead of net/http.
// net/http unconditionally references crypto/tls (Transport, ClientConfig,
// ...) even though Go's wasm net/http implementation itself calls fetch()
// under the hood, so importing it here would drag the entire TLS/x509 stack
// — nistec, ecdsa, mlkem, rsa, ~1MB — into every binary that uses widget,
// including ones that never construct a NetworkImage. The browser's own TLS
// stack already terminates the connection; Go's never runs on this path.
func (l *imgLoader) do(url string) loadResult {
	// fetch() has no timeout of its own — a request to a host that accepts the
	// connection and then says nothing hangs forever, holding one of the
	// imgFetchConcurrency slots with it. An AbortController driven by
	// setTimeout supplies what http.Client.Timeout supplies on native, from
	// the same imgFetchTimeout constant rather than a duration written twice.
	ctrl := js.Global().Get("AbortController").New()
	opts := js.Global().Get("Object").New()
	opts.Set("signal", ctrl.Get("signal"))

	timedOut := false
	abort := js.FuncOf(func(js.Value, []js.Value) any {
		timedOut = true
		ctrl.Call("abort")
		return nil
	})
	timer := js.Global().Call("setTimeout", abort, imgFetchTimeout.Milliseconds())
	defer func() {
		js.Global().Call("clearTimeout", timer)
		abort.Release()
	}()

	// The abort covers the body read too, not just the headers: a response
	// that stalls halfway through its bytes is the same hung slot.
	fail := func(err error) loadResult {
		if timedOut {
			return loadResult{err: fmt.Errorf("netimage: %s: timed out after %s", url, imgFetchTimeout)}
		}
		return loadResult{err: fmt.Errorf("netimage: %s: %w", url, err)}
	}

	resp, err := awaitPromise(js.Global().Call("fetch", url, opts))
	if err != nil {
		return fail(err)
	}
	if !resp.Get("ok").Bool() {
		return loadResult{err: fmt.Errorf("netimage: %s: %s", url, resp.Get("statusText").String())}
	}

	buf, err := awaitPromise(resp.Call("arrayBuffer"))
	if err != nil {
		return fail(err)
	}
	array := js.Global().Get("Uint8Array").New(buf)
	data := make([]byte, array.Get("length").Int())
	js.CopyBytesToGo(data, array)

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return loadResult{err: fmt.Errorf("netimage: decode %s: %w", url, err)}
	}
	return loadResult{img: img}
}

// awaitPromise blocks the calling goroutine until a JS Promise settles.
// Go's wasm goroutines are cooperatively scheduled, so this yields via a
// channel rather than spinning; the caller must not be on the main
// goroutine or the JS event loop that resolves the promise never runs.
func awaitPromise(promise js.Value) (js.Value, error) {
	type result struct {
		value    js.Value
		rejected bool
	}
	ch := make(chan result, 1)

	var thenFn, catchFn js.Func
	thenFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{value: arg0(args)}
		return nil
	})
	catchFn = js.FuncOf(func(_ js.Value, args []js.Value) any {
		ch <- result{value: arg0(args), rejected: true}
		return nil
	})
	promise.Call("then", thenFn).Call("catch", catchFn)

	r := <-ch
	thenFn.Release()
	catchFn.Release()

	if r.rejected {
		msg := r.value.Get("message")
		if msg.Truthy() {
			return js.Undefined(), fmt.Errorf("%s", msg.String())
		}
		return js.Undefined(), fmt.Errorf("%s", r.value.Call("toString").String())
	}
	return r.value, nil
}

func arg0(args []js.Value) js.Value {
	if len(args) > 0 {
		return args[0]
	}
	return js.Undefined()
}
