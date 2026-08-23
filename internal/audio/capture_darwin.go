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

// CoreAudio AudioQueue *input* driver (zero-cgo, via goffi) — the mirror of the
// AudioQueue output path in driver_darwin.go, and it reuses that file's library
// handle, asbd layout and proc wrapper.
//
// AudioQueueNewInput registers a callback that fires each time a buffer has
// been filled by the hardware; the callback hands the samples to the sink and
// re-enqueues the buffer. The callback runs on one of the queue's own C
// threads, which goffi's callback bridge supports being entered from.
//
// Microphone access on macOS is gated by TCC. A bundled .app needs
// NSMicrophoneUsageDescription in its Info.plist; a bare binary run from a
// terminal inherits the terminal's own grant, so the first run may prompt for
// the terminal rather than for this program. Either way a denial surfaces as
// silence — CoreAudio starts the queue and delivers zeros — which is why the
// shell layer reports level rather than assuming input is working.
var (
	aqNewInput, aqAllocBufIn, aqEnqueueIn, aqStartIn, aqStopIn, aqDisposeIn *proc
	caInOnce                                                                sync.Once
	caInErr                                                                 error
)

func loadCoreAudioInput() error {
	caInOnce.Do(func() {
		h, err := ffi.LoadLibrary("/System/Library/Frameworks/AudioToolbox.framework/AudioToolbox")
		if err != nil {
			caInErr = fmt.Errorf("audio: load AudioToolbox: %w", err)
			return
		}
		mk := func(name string, nargs int) *proc {
			fn, gerr := ffi.GetSymbol(h, name)
			if gerr != nil {
				caInErr = fmt.Errorf("audio: resolve %s: %w", name, gerr)
				return nil
			}
			return newProc(fn, nargs)
		}
		aqNewInput = mk("AudioQueueNewInput", 7)
		aqAllocBufIn = mk("AudioQueueAllocateBuffer", 3)
		aqEnqueueIn = mk("AudioQueueEnqueueBuffer", 4)
		aqStartIn = mk("AudioQueueStart", 2)
		aqStopIn = mk("AudioQueueStop", 2)
		aqDisposeIn = mk("AudioQueueDispose", 2)
	})
	return caInErr
}

// captureBuffers is how many buffers the queue cycles through. Three is the
// usual choice: one being filled by the hardware, one in the callback, one
// spare, so a slow callback does not starve the queue.
const captureBuffers = 3

type coreAudioCapture struct {
	rate     int
	bufBytes int
	sink     func([]float32)

	// FFI out-parameters live on the heap-allocated struct so their addresses
	// stay stable; a &stackLocal passed as uintptr can go stale if the
	// goroutine's stack moves before C writes to it. Same reasoning as the
	// output driver.
	format  asbd
	queue   uintptr
	scratch uintptr
	cb      uintptr // keep the callback trampoline alive

	mu      sync.Mutex
	started bool
	closed  bool
}

func defaultCapture() Capture { return &coreAudioCapture{} }

func (c *coreAudioCapture) Open(sampleRate int) (int, error) {
	if err := loadCoreAudioInput(); err != nil {
		return 0, err
	}
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	c.rate = sampleRate
	// ~46 ms per buffer at 44.1 kHz. Matched to the analysis window rather than
	// minimised: latency below one window buys nothing here, and larger buffers
	// mean fewer callbacks and less chance of a dropout.
	c.bufBytes = 2048 * 4
	return c.rate, nil
}

// callback is the AudioQueueInputCallback: (userData, aq, buffer, startTime,
// numPacketDescs, packetDescs).
func (c *coreAudioCapture) callback(userData, aq, buf, startTime, numDescs, descs uintptr) uintptr {
	// buf is a CoreAudio-owned AudioQueueBuffer* — native memory the Go GC never
	// moves — so the uintptr+offset reads are safe FFI, not the moving-object
	// hazard go vet's "possible misuse of unsafe.Pointer" warns about.
	byteSize := *(*uint32)(unsafe.Pointer(buf + 16)) //nolint:govet // native AudioQueueBuffer field @ 16
	dataPtr := *(*uintptr)(unsafe.Pointer(buf + 8))  //nolint:govet // native AudioQueueBuffer field @ 8
	if dataPtr != 0 && byteSize > 0 {
		n := int(byteSize) / 4
		in := unsafe.Slice((*float32)(unsafe.Pointer(dataPtr)), n) //nolint:govet // native audio data buffer
		if s := c.sink; s != nil {
			s(in)
		}
	}
	// Re-enqueue so the hardware can fill it again. Skipping this on the last
	// buffer would silently stop capture rather than erroring.
	aqEnqueueIn.call(aq, buf, 0, 0)
	return 0
}

func (c *coreAudioCapture) Start(sink func([]float32)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.started {
		return nil
	}
	if c.rate == 0 {
		return fmt.Errorf("audio: capture started before Open")
	}
	c.sink = sink
	c.cb = ffi.NewCallback(c.callback)

	c.format = asbd{
		SampleRate:       float64(c.rate),
		FormatID:         kAudioFormatLinearPCM,
		FormatFlags:      kAudioFormatFlagIsFloat | kAudioFormatFlagIsPacked,
		BytesPerPacket:   4,
		FramesPerPacket:  1,
		BytesPerFrame:    4,
		ChannelsPerFrame: 1, // mono: the analyzer is mono and the OS downmixes
		BitsPerChannel:   32,
	}

	// AudioQueueNewInput(&format, cb, userData=0, runLoop=NULL, mode=NULL, flags=0, &queue).
	// A NULL run loop asks CoreAudio for its own callback thread, which is what
	// we want: a UI that stops pumping its run loop must not stop capture.
	c.queue = 0
	st := aqNewInput.call(
		uintptr(unsafe.Pointer(&c.format)), c.cb, 0, 0, 0, 0,
		uintptr(unsafe.Pointer(&c.queue)),
	)
	if int32(st) != 0 || c.queue == 0 {
		return fmt.Errorf("audio: AudioQueueNewInput failed (status %d)", int32(st))
	}
	queue := c.queue

	for i := 0; i < captureBuffers; i++ {
		c.scratch = 0
		if st := aqAllocBufIn.call(queue, uintptr(c.bufBytes), uintptr(unsafe.Pointer(&c.scratch))); int32(st) != 0 || c.scratch == 0 {
			aqDisposeIn.call(queue, 1)
			c.queue = 0
			return fmt.Errorf("audio: AudioQueueAllocateBuffer failed (status %d)", int32(st))
		}
		if st := aqEnqueueIn.call(queue, c.scratch, 0, 0); int32(st) != 0 {
			aqDisposeIn.call(queue, 1)
			c.queue = 0
			return fmt.Errorf("audio: AudioQueueEnqueueBuffer failed (status %d)", int32(st))
		}
	}
	if st := aqStartIn.call(queue, 0); int32(st) != 0 {
		aqDisposeIn.call(queue, 1)
		c.queue = 0
		return fmt.Errorf("audio: AudioQueueStart failed (status %d)", int32(st))
	}
	c.started = true
	return nil
}

func (c *coreAudioCapture) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	if c.queue != 0 {
		aqStopIn.call(c.queue, 1) // inImmediate
		aqDisposeIn.call(c.queue, 1)
		c.queue = 0
	}
	c.started = false
	// Drop the sink so a callback already in flight cannot deliver into a
	// monitor its owner believes is finished.
	c.sink = nil
	return nil
}
