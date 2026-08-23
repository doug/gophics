// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

//go:build linux && !android && !js

package audio

import (
	"fmt"
	"sync"
	"unsafe"

	"github.com/go-webgpu/goffi/ffi"
)

// PulseAudio blocking capture via libpulse-simple (pa_simple), through goffi
// (zero-cgo) — the mirror of the playback path in driver_linux.go, which this
// file's constants and paSampleSpec come from.
//
// pa_simple has no callback, so capture runs its own pump goroutine that blocks
// in pa_simple_read and hands each block to the sink. That goroutine *is* the
// audio thread as far as the Capture contract is concerned.

const paStreamRecord = 2 // PA_STREAM_RECORD

var (
	paInOnce sync.Once
	paInErr  error

	paSimpleNewIn, paSimpleRead, paSimpleFreeIn *proc
)

func loadPulseInput() error {
	paInOnce.Do(func() {
		var h unsafe.Pointer
		var err error
		for _, n := range []string{"libpulse-simple.so.0", "libpulse-simple.so"} {
			if h, err = ffi.LoadLibrary(n); err == nil {
				break
			}
		}
		if h == nil {
			paInErr = fmt.Errorf("audio: load libpulse-simple (install libpulse): %w", err)
			return
		}
		mk := func(name string, nargs int) *proc {
			fn, gerr := ffi.GetSymbol(h, name)
			if gerr != nil {
				paInErr = fmt.Errorf("audio: resolve %s: %w", name, gerr)
				return nil
			}
			return newProc(fn, nargs)
		}
		paSimpleNewIn = mk("pa_simple_new", 9)
		paSimpleRead = mk("pa_simple_read", 4)
		paSimpleFreeIn = mk("pa_simple_free", 1)
	})
	return paInErr
}

type pulseCapture struct {
	rate   int
	frames int
	sink   func([]float32)

	// Heap-stable FFI storage; see the CoreAudio driver's note on why a
	// stack address passed as uintptr can go stale.
	spec paSampleSpec
	perr int32

	mu   sync.Mutex
	s    uintptr // pa_simple*
	stop chan struct{}
	done chan struct{}
}

func defaultCapture() Capture { return &pulseCapture{} }

func (c *pulseCapture) Open(sampleRate int) (int, error) {
	if err := loadPulseInput(); err != nil {
		return 0, err
	}
	if sampleRate <= 0 {
		sampleRate = 44100
	}
	c.rate = sampleRate
	// One analysis window per read: pa_simple_read blocks until exactly this
	// many bytes are available, so the size sets the callback cadence.
	c.frames = 2048
	// PulseAudio resamples to whatever is asked for, so unlike CoreAudio and
	// WASAPI the requested rate is always the granted one.
	return c.rate, nil
}

func (c *pulseCapture) Start(sink func([]float32)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.s != 0 {
		return nil
	}
	if c.rate == 0 {
		return fmt.Errorf("audio: capture started before Open")
	}
	c.sink = sink
	c.spec = paSampleSpec{Format: paSampleFloat32LE, Rate: uint32(c.rate), Channels: 1}

	name := append([]byte("gophics"), 0)
	stream := append([]byte("capture"), 0)
	c.s = paSimpleNewIn.call(
		0, // server (default)
		uintptr(unsafe.Pointer(&name[0])),
		paStreamRecord,
		0, // device (default source)
		uintptr(unsafe.Pointer(&stream[0])),
		uintptr(unsafe.Pointer(&c.spec)),
		0, // channel map (default)
		0, // buffer attr (default)
		uintptr(unsafe.Pointer(&c.perr)),
	)
	if c.s == 0 {
		return fmt.Errorf("audio: pa_simple_new (record) failed (error %d)", c.perr)
	}
	c.stop = make(chan struct{})
	c.done = make(chan struct{})
	go c.pump()
	return nil
}

// pump blocks in pa_simple_read and delivers each block to the sink.
func (c *pulseCapture) pump() {
	defer close(c.done)
	buf := make([]float32, c.frames)
	raw := unsafe.Slice((*byte)(unsafe.Pointer(&buf[0])), len(buf)*4)
	for {
		select {
		case <-c.stop:
			return
		default:
		}
		var perr int32
		if int32(paSimpleRead.call(c.s, uintptr(unsafe.Pointer(&raw[0])), uintptr(len(raw)), uintptr(unsafe.Pointer(&perr)))) < 0 {
			return // the source went away; Close will tidy up
		}
		if s := c.sink; s != nil {
			s(buf)
		}
	}
}

func (c *pulseCapture) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stop != nil {
		close(c.stop)
		c.stop = nil
	}
	// pa_simple_read may be blocked in the kernel; freeing the stream is what
	// unblocks it, so the pump is joined after the free rather than before.
	if c.s != 0 {
		paSimpleFreeIn.call(c.s)
		c.s = 0
	}
	if c.done != nil {
		<-c.done
		c.done = nil
	}
	c.sink = nil
	return nil
}
