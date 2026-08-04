// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build js && wasm

package audio

import (
	"fmt"
	"syscall/js"
	"unsafe"
)

// WebAudio output. A ScriptProcessorNode's onaudioprocess callback pulls mixed
// samples from the source and writes them into the node's output buffer, which
// is connected to the AudioContext destination. It is the playback mirror of the
// ScriptProcessorNode capture path. (ScriptProcessorNode is deprecated in favor
// of AudioWorklet but is universally supported and simplest for a single-file
// wasm build.)
//
// Browsers start an AudioContext suspended until a user gesture; Start calls
// resume(), but if no gesture has occurred yet, sound begins on the first one.

type webDriver struct {
	sampleRate int
	channels   int

	src     ReadFloat32er
	ctx     js.Value
	proc    js.Value
	onaudio js.Func

	buf   []float32 // interleaved pull buffer
	chBuf []float32 // per-channel scratch
}

func defaultDriver() Driver { return &webDriver{} }

func (d *webDriver) Open(sampleRate, channels, bufferSizeMs int) error {
	if channels < 1 {
		channels = 2
	}
	d.sampleRate, d.channels = sampleRate, channels
	return nil
}

func (d *webDriver) SetSource(src ReadFloat32er) { d.src = src }

func (d *webDriver) Start() error {
	ctxClass := js.Global().Get("AudioContext")
	if !ctxClass.Truthy() {
		ctxClass = js.Global().Get("webkitAudioContext")
	}
	if !ctxClass.Truthy() {
		return fmt.Errorf("audio: no AudioContext available")
	}
	d.ctx = ctxClass.New()
	d.channels = d.ctx.Get("destination").Get("channelCount").Int()
	if d.channels < 1 {
		d.channels = 2
	}
	// bufferSize=4096, 0 input channels, N output channels.
	d.proc = d.ctx.Call("createScriptProcessor", 4096, 0, d.channels)

	d.onaudio = js.FuncOf(func(_ js.Value, a []js.Value) any {
		out := a[0].Get("outputBuffer")
		frames := out.Get("length").Int()
		need := frames * d.channels
		if cap(d.buf) < need {
			d.buf = make([]float32, need)
		}
		b := d.buf[:need]
		n := 0
		if d.src != nil {
			n, _ = d.src.ReadFloat32s(b)
		}
		for i := n; i < need; i++ {
			b[i] = 0
		}
		if cap(d.chBuf) < frames {
			d.chBuf = make([]float32, frames)
		}
		ch := d.chBuf[:frames]
		u8class := js.Global().Get("Uint8Array")
		for c := 0; c < d.channels; c++ {
			for f := 0; f < frames; f++ {
				ch[f] = b[f*d.channels+c] // de-interleave
			}
			chData := out.Call("getChannelData", c) // Float32Array
			view := u8class.New(chData.Get("buffer"), chData.Get("byteOffset"), frames*4)
			js.CopyBytesToJS(view, unsafe.Slice((*byte)(unsafe.Pointer(&ch[0])), frames*4))
		}
		return nil
	})
	d.proc.Set("onaudioprocess", d.onaudio)
	d.proc.Call("connect", d.ctx.Get("destination"))
	d.ctx.Call("resume")
	return nil
}

func (d *webDriver) Stop() error {
	if d.proc.Truthy() {
		d.proc.Call("disconnect")
	}
	if d.ctx.Truthy() {
		d.ctx.Call("suspend")
	}
	return nil
}

func (d *webDriver) Close() error {
	_ = d.Stop()
	if d.proc.Truthy() {
		d.proc.Set("onaudioprocess", js.Null())
	}
	if d.onaudio.Truthy() {
		d.onaudio.Release()
	}
	if d.ctx.Truthy() {
		d.ctx.Call("close")
	}
	return nil
}
