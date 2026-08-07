// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build darwin

package audio

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
)

// CoreAudio AudioQueue output driver (zero-cgo, via goffi). It is the mirror of
// the AudioQueue input path: AudioQueueNewOutput registers a callback that, each
// time the queue needs data, refills a buffer from the source (the Mixer) and
// re-enqueues it. The callback runs on one of the queue's own C threads;
// goffi's callback bridge supports being entered from a C-created thread.
//
// This replaces the NullDriver that the upstream !windows build uses, so audio
// actually plays on macOS.

const (
	kAudioFormatLinearPCM    = 0x6C70636D // 'lpcm'
	kAudioFormatFlagIsFloat  = 1 << 0
	kAudioFormatFlagIsPacked = 1 << 3
)

// asbd mirrors AudioStreamBasicDescription (40 bytes).
type asbd struct {
	SampleRate       float64
	FormatID         uint32
	FormatFlags      uint32
	BytesPerPacket   uint32
	FramesPerPacket  uint32
	BytesPerFrame    uint32
	ChannelsPerFrame uint32
	BitsPerChannel   uint32
	Reserved         uint32
}

var (
	caOnce sync.Once
	caErr  error

	aqNewOutput, aqAllocBuf, aqEnqueue, aqStart, aqStop, aqDispose *proc
)

func loadCoreAudio() error {
	caOnce.Do(func() {
		h, err := ffi.LoadLibrary("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox")
		if err != nil {
			caErr = fmt.Errorf("audio: load AudioToolbox: %w", err)
			return
		}
		mk := func(name string, nargs int) *proc {
			fn, gerr := ffi.GetSymbol(h, name)
			if gerr != nil {
				caErr = fmt.Errorf("audio: resolve %s: %w", name, gerr)
				return nil
			}
			return newProc(fn, nargs)
		}
		aqNewOutput = mk("AudioQueueNewOutput", 7)
		aqAllocBuf = mk("AudioQueueAllocateBuffer", 3)
		aqEnqueue = mk("AudioQueueEnqueueBuffer", 4)
		aqStart = mk("AudioQueueStart", 2)
		aqStop = mk("AudioQueueStop", 2)
		aqDispose = mk("AudioQueueDispose", 2)
	})
	return caErr
}

// coreAudioDriver implements Driver using a CoreAudio output AudioQueue.
type coreAudioDriver struct {
	sampleRate int
	channels   int
	bufBytes   int

	src   ReadFloat32er
	queue uintptr
	cb    uintptr // keep the callback trampoline alive

	// FFI out-parameters live on the (heap-allocated) driver struct so their
	// addresses stay stable: passing &stackLocal as a uintptr through the call
	// wrapper can go stale if the goroutine's stack moves before C writes to it,
	// which leaves the handle unwritten (a zero queue after a noErr status).
	format  asbd
	scratch uintptr

	mu      sync.Mutex
	started bool
}

func defaultDriver() Driver { return &coreAudioDriver{} }

func (d *coreAudioDriver) Open(sampleRate, channels, bufferSizeMs int) error {
	if err := loadCoreAudio(); err != nil {
		return err
	}
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	if channels < 1 {
		channels = 2
	}
	if bufferSizeMs <= 0 {
		bufferSizeMs = 50
	}
	d.sampleRate, d.channels = sampleRate, channels
	frames := sampleRate * bufferSizeMs / 1000
	d.bufBytes = frames * 4 * channels
	return nil
}

func (d *coreAudioDriver) SetSource(src ReadFloat32er) { d.src = src }

// fill pulls one buffer's worth of samples from the source into the AudioQueue
// buffer (zero-padding any shortfall so a full buffer is always emitted) and
// sets its byte size.
func (d *coreAudioDriver) fill(buf uintptr) {
	// buf is a CoreAudio-owned AudioQueueBuffer* — native memory the Go GC never
	// moves — so the uintptr+offset field reads below are safe FFI, not the
	// moving-object hazard go vet's "possible misuse of unsafe.Pointer" guards
	// against. (//nolint documents intent; it does not affect `go vet`.)
	capacity := *(*uint32)(unsafe.Pointer(buf))     //nolint:govet // native AudioQueueBuffer field @ 0
	dataPtr := *(*uintptr)(unsafe.Pointer(buf + 8)) //nolint:govet // native AudioQueueBuffer field @ 8
	n := int(capacity) / 4
	if dataPtr == 0 || n <= 0 {
		return
	}
	out := unsafe.Slice((*float32)(unsafe.Pointer(dataPtr)), n) //nolint:govet // native audio data buffer
	got := 0
	if d.src != nil {
		got, _ = d.src.ReadFloat32s(out)
	}
	for i := got; i < n; i++ {
		out[i] = 0 // silence for the remainder
	}
	*(*uint32)(unsafe.Pointer(buf + 16)) = capacity //nolint:govet // native AudioQueueBuffer field @ 16 (full buffer)
}

// callback is the AudioQueueOutputCallback, invoked on a CoreAudio thread when a
// buffer has finished playing and can be refilled.
func (d *coreAudioDriver) callback(userData, aq, buf uintptr) uintptr {
	d.fill(buf)
	aqEnqueue.call(aq, buf, 0, 0)
	return 0
}

func (d *coreAudioDriver) Start() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.started {
		return nil
	}
	d.cb = ffi.NewCallback(d.callback)

	d.format = asbd{
		SampleRate:       float64(d.sampleRate),
		FormatID:         kAudioFormatLinearPCM,
		FormatFlags:      kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked,
		BytesPerPacket:   uint32(4 * d.channels),
		FramesPerPacket:  1,
		BytesPerFrame:    uint32(4 * d.channels),
		ChannelsPerFrame: uint32(d.channels),
		BitsPerChannel:   32,
	}

	// AudioQueueNewOutput(&format, cb, userData=0, runLoop=NULL, mode=NULL, flags=0, &queue).
	// &d.format and &d.queue are heap-stable (see the struct comment).
	d.queue = 0
	st := aqNewOutput.call(
		uintptr(unsafe.Pointer(&d.format)), d.cb, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&d.queue)),
	)
	if int32(st) != 0 || d.queue == 0 {
		return fmt.Errorf("audio: AudioQueueNewOutput failed (status %d)", int32(st))
	}
	queue := d.queue

	// Allocate, prime, and enqueue a few buffers so playback has data to start.
	for i := 0; i < 3; i++ {
		d.scratch = 0
		if st := aqAllocBuf.call(queue, uintptr(d.bufBytes), uintptr(unsafe.Pointer(&d.scratch))); int32(st) != 0 || d.scratch == 0 {
			aqDispose.call(queue, 1)
			d.queue = 0
			return fmt.Errorf("audio: AudioQueueAllocateBuffer failed (status %d)", int32(st))
		}
		buf := d.scratch
		d.fill(buf)
		if st := aqEnqueue.call(queue, buf, 0, 0); int32(st) != 0 {
			aqDispose.call(queue, 1)
			d.queue = 0
			return fmt.Errorf("audio: AudioQueueEnqueueBuffer failed (status %d)", int32(st))
		}
	}
	if st := aqStart.call(queue, 0); int32(st) != 0 {
		aqDispose.call(queue, 1)
		d.queue = 0
		return fmt.Errorf("audio: AudioQueueStart failed (status %d)", int32(st))
	}
	d.started = true
	return nil
}

func (d *coreAudioDriver) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.queue != 0 {
		aqStop.call(d.queue, 1) // inImmediate = true
	}
	d.started = false
	return nil
}

func (d *coreAudioDriver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.queue != 0 {
		aqStop.call(d.queue, 1)
		aqDispose.call(d.queue, 1)
		d.queue = 0
	}
	d.started = false
	return nil
}
