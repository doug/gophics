//go:build js && wasm

package web

import (
	"errors"
	"syscall/js"
)

// Small shared pieces of the media capabilities: stream teardown, byte
// bridging, and the getUserMedia request they all start from.

func stopTracks(stream js.Value) {
	tracks := stream.Call("getTracks")
	for i := 0; i < tracks.Length(); i++ {
		tracks.Index(i).Call("stop")
	}
}

func jsToBytes(u8 js.Value) []byte {
	b := make([]byte, u8.Length())
	js.CopyBytesToGo(b, u8)
	return b
}

func bytesToJS(b []byte) js.Value {
	u8 := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(u8, b)
	return u8
}

// requestStream wraps getUserMedia, which is a promise, into the callback shape
// the shell interfaces use.
func requestStream(constraints map[string]any, done func(js.Value, error)) {
	md := mediaDevices()
	if md.IsUndefined() {
		done(js.Undefined(), errors.New("getUserMedia unavailable (an insecure context?)"))
		return
	}
	promise := md.Call("getUserMedia", constraints)
	go func() {
		stream, err := await(promise)
		done(stream, err)
	}()
}

func mediaDevices() js.Value {
	md := js.Global().Get("navigator").Get("mediaDevices")
	if md.IsNull() {
		return js.Undefined()
	}
	return md
}
