//go:build js && wasm

package fetch

import (
	"context"
	"fmt"
	"syscall/js"
)

// Do performs r with the browser's fetch().
//
// The body is read as an ArrayBuffer rather than text, so this carries images
// and any other binary as faithfully as JSON. Reading it as a string would
// round-trip through UTF-16 and corrupt anything that is not text — which is
// exactly what a caller fetching a PNG would hit.
func Do(ctx context.Context, r Request) (*Response, error) {
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("fetch %s: %w", r.URL, err)
	}

	global := js.Global()
	opts := global.Get("Object").New()
	if r.Method != "" {
		opts.Set("method", r.Method)
	}
	if len(r.Header) > 0 {
		h := global.Get("Object").New()
		for k, v := range r.Header {
			h.Set(k, v)
		}
		opts.Set("headers", h)
	}
	if len(r.Body) > 0 {
		buf := global.Get("Uint8Array").New(len(r.Body))
		js.CopyBytesToJS(buf, r.Body)
		opts.Set("body", buf)
	}

	// fetch() has no timeout and no notion of a Go context, so an
	// AbortController bridges the two: cancelling ctx aborts the request in
	// flight rather than leaving it to complete into a caller that has gone.
	ctrl := global.Get("AbortController").New()
	opts.Set("signal", ctrl.Get("signal"))
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			ctrl.Call("abort")
		case <-stop:
		}
	}()

	type outcome struct {
		resp *Response
		err  error
	}
	done := make(chan outcome, 1)

	var onResp, onBody, onErr js.Func
	// Released once, after the promise chain has settled. The chain is
	// single-shot, so releasing here is safe; releasing any earlier would free
	// a func the browser still holds a reference to.
	release := func() { onResp.Release(); onBody.Release(); onErr.Release() }

	fail := func(msg string) {
		done <- outcome{err: fmt.Errorf("fetch %s: %s", r.URL, msg)}
	}

	onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		// An aborted request reports the cancellation rather than the DOM
		// error, because "context canceled" is the useful half of that.
		if err := ctx.Err(); err != nil {
			done <- outcome{err: fmt.Errorf("fetch %s: %w", r.URL, err)}
			return nil
		}
		msg := "request failed"
		if len(args) > 0 && !args[0].IsUndefined() {
			msg = args[0].Call("toString").String()
		}
		fail(msg)
		return nil
	})

	var status int
	var header map[string]string

	onBody = js.FuncOf(func(_ js.Value, args []js.Value) any {
		buf := global.Get("Uint8Array").New(args[0])
		b := make([]byte, buf.Get("length").Int())
		js.CopyBytesToGo(b, buf)
		done <- outcome{resp: &Response{Status: status, Header: header, Body: b}}
		return nil
	})

	onResp = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resp := args[0]
		status = resp.Get("status").Int()
		header = headersOf(resp.Get("headers"))
		resp.Call("arrayBuffer").Call("then", onBody).Call("catch", onErr)
		return nil
	})

	global.Call("fetch", r.URL, opts).Call("then", onResp).Call("catch", onErr)

	o := <-done
	release()
	return o.resp, o.err
}

// headersOf flattens a fetch Headers object into a map. Repeated names are
// joined by the browser before this sees them, which is what net/http's
// Header.Get returns too, so both platforms answer the same way.
func headersOf(h js.Value) map[string]string {
	if !h.Truthy() {
		return nil
	}
	out := map[string]string{}
	cb := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) >= 2 {
			out[args[1].String()] = args[0].String()
		}
		return nil
	})
	defer cb.Release()
	h.Call("forEach", cb)
	return out
}
