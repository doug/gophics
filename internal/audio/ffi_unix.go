// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build (darwin || linux) && !js

package audio

import (
	"runtime"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
	"github.com/go-webgpu/goffi/types"
)

// proc is a minimal libffi call wrapper shared by the darwin (CoreAudio) and
// linux (PulseAudio) output drivers: every argument and the return value is
// passed pointer-sized, which matches the C ABI for the calls we make.
type proc struct {
	fn  unsafe.Pointer
	cif types.CallInterface
}

func newProc(fn unsafe.Pointer, nargs int) *proc {
	p := &proc{fn: fn}
	at := make([]*types.TypeDescriptor, nargs)
	for i := range at {
		at[i] = types.PointerTypeDescriptor
	}
	if err := ffi.PrepareCallInterface(&p.cif, types.DefaultCall, types.PointerTypeDescriptor, at); err != nil {
		panic(err)
	}
	return p
}

func (p *proc) call(args ...uintptr) uintptr {
	var ret uintptr
	av := make([]unsafe.Pointer, len(args))
	for i := range args {
		av[i] = unsafe.Pointer(&args[i])
	}
	if _, err := ffi.CallFunction(&p.cif, p.fn, unsafe.Pointer(&ret), av); err != nil {
		panic(err)
	}
	runtime.KeepAlive(args)
	return ret
}
