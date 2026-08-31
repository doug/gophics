//go:build js

package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"syscall/js"
)

// On wasm this calls fetch() directly rather than going through net/http.
//
// Go's js/wasm net/http already calls fetch() under the hood, so this is not
// about which network API runs — it is about what the linker can drop.
// Transport.RoundTrip branches on jsFetchMissing, a runtime value, so the
// socket path stays reachable and drags crypto/tls, x509, and http2 in with
// it. Measured against a build that only differs in this: 2.61MB gzipped
// versus 0.55MB. The browser terminates TLS on this path; Go's stack never
// runs. widget/netimage_fetch_js.go makes the same trade for the same reason.
type liveAPI struct{}

func newLiveAPI() *liveAPI { return &liveAPI{} }

func (a *liveAPI) get(ctx context.Context, url string, v any) error {
	// fetch() has no timeout of its own, and ctx cancellation has to reach it
	// somehow: an AbortController bridges both, so a cancelled context and an
	// elapsed apiTimeout abort the request the same way.
	ctrl := js.Global().Get("AbortController").New()
	opts := js.Global().Get("Object").New()
	opts.Set("signal", ctrl.Get("signal"))

	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			ctrl.Call("abort")
		case <-stop:
		}
	}()

	type result struct {
		body string
		err  error
	}
	done := make(chan result, 1)

	var onOK, onText, onErr js.Func
	release := func() { onOK.Release(); onText.Release(); onErr.Release() }

	onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "fetch failed"
		if len(args) > 0 && !args[0].IsUndefined() {
			msg = args[0].Call("toString").String()
		}
		done <- result{err: fmt.Errorf("%s: %s", url, msg)}
		return nil
	})
	onText = js.FuncOf(func(_ js.Value, args []js.Value) any {
		done <- result{body: args[0].String()}
		return nil
	})
	onOK = js.FuncOf(func(_ js.Value, args []js.Value) any {
		resp := args[0]
		if !resp.Get("ok").Bool() {
			done <- result{err: fmt.Errorf("%s: HTTP %d", url, resp.Get("status").Int())}
			return nil
		}
		resp.Call("text").Call("then", onText).Call("catch", onErr)
		return nil
	})

	js.Global().Call("fetch", url, opts).Call("then", onOK).Call("catch", onErr)

	// The callbacks must outlive this call only until one of them fires; the
	// promise chain is single-shot, so releasing here is safe and releasing
	// any earlier would free a func the browser still holds.
	r := <-done
	release()
	if r.err != nil {
		return r.err
	}
	return json.Unmarshal([]byte(r.body), v)
}
